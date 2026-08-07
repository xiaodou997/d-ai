package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	commercial "xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/routing"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/core/surface"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

const (
	legacyRoutingScopeGlobal = "global"
)

// CommercialRepo adapts the current group/limit/route-weight storage to the
// rebuilt commercial repository port. This bridge is intentionally honest:
// only capabilities that already have a real legacy backing store are wired.
// Features that need the rebuilt schema stay explicitly unavailable instead of
// being silently faked.
type CommercialRepo struct {
	q               *dbgen.Queries
	pool            *pgxpool.Pool
	group           *GroupRepo
	limit           *LimitRepo
	weights         *RouteWeightsStore
	runtimeResolver *coreruntime.Resolver
}

// WithRuntimeResolver makes dispatch preview use the same planner and binder
// as live requests. It is set during server wiring after the repository exists.
func (r *CommercialRepo) WithRuntimeResolver(resolver *coreruntime.Resolver) *CommercialRepo {
	r.runtimeResolver = resolver
	return r
}

func NewCommercialRepo(q *dbgen.Queries, pool *pgxpool.Pool) *CommercialRepo {
	return &CommercialRepo{
		q:       q,
		pool:    pool,
		group:   NewGroupRepo(q, pool),
		limit:   NewLimitRepo(q),
		weights: NewRouteWeightsStore(pool),
	}
}

var _ commercial.Repository = (*CommercialRepo)(nil)

func (r *CommercialRepo) CreateGroup(ctx context.Context, tenantID string, in commercial.GroupWrite) (commercial.Group, error) {
	priceBookID, err := akUUID(in.RetailPriceBookID)
	if err != nil {
		return commercial.Group{}, domain.NewValidationError("retail_price_book_id", "invalid retail_price_book_id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.Group{}, err
	}
	defer tx.Rollback(ctx)
	name := commercialNameOrCode(in.Name, in.Code)
	if err := validateVisibleActivePriceBook(ctx, tx, tenantID, priceBookID, "", name); err != nil {
		return commercial.Group{}, err
	}
	item, err := r.q.WithTx(tx).CreateGroup(ctx, dbgen.CreateGroupParams{
		TenantID:                tenantID,
		Name:                    name,
		Description:             in.Description,
		RetailPriceBookID:       priceBookID,
		DefaultUserMultiplier:   floatToNumeric(in.DefaultUserMultiplier),
		UserDefaultVisible:      in.UserDefaultVisible,
		AllowProtocolConversion: in.AllowProtocolConversion,
		SortOrder:               int32(in.SortOrder),
		Status:                  commercialStatusOrDefault(in.Status),
	})
	if err != nil {
		return commercial.Group{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.Group{}, err
	}
	return legacyGroupToCommercial(groupFromRow(item)), nil
}

func (r *CommercialRepo) ListGroups(ctx context.Context, tenantID string) ([]commercial.Group, error) {
	items, err := r.group.ListGroups(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]commercial.Group, 0, len(items))
	for _, item := range items {
		out = append(out, legacyGroupToCommercial(item.Group))
	}
	return out, nil
}

func (r *CommercialRepo) GetGroup(ctx context.Context, scope commercial.TenantGroupScope) (commercial.Group, error) {
	item, err := r.group.GetGroup(ctx, scope.TenantID, scope.GroupID)
	if err != nil {
		return commercial.Group{}, err
	}
	return legacyGroupToCommercial(item), nil
}

func (r *CommercialRepo) UpdateGroup(ctx context.Context, scope commercial.TenantGroupScope, in commercial.GroupWrite) (commercial.Group, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.Group{}, domain.NewValidationError("id", "invalid id")
	}
	priceBookID, err := akUUID(in.RetailPriceBookID)
	if err != nil {
		return commercial.Group{}, domain.NewValidationError("retail_price_book_id", "invalid retail_price_book_id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.Group{}, err
	}
	defer tx.Rollback(ctx)
	current, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return commercial.Group{}, err
	}
	name := commercialNameOrCode(in.Name, in.Code)
	if err := validateVisibleActivePriceBook(ctx, tx, scope.TenantID, priceBookID, current.ID, name); err != nil {
		return commercial.Group{}, err
	}
	nextStatus := commercialStatusOrDefault(in.Status)
	if current.PriceBookID != in.RetailPriceBookID || (current.Status != "active" && nextStatus == "active") {
		candidate := current
		candidate.Name = name
		candidate.PriceBookID = in.RetailPriceBookID
		if err := validateActiveRulesAgainstBook(ctx, tx, candidate, priceBookID); err != nil {
			return commercial.Group{}, err
		}
	}
	item, err := r.q.WithTx(tx).UpdateGroup(ctx, dbgen.UpdateGroupParams{
		ID:                      gid,
		Name:                    name,
		Description:             in.Description,
		RetailPriceBookID:       priceBookID,
		DefaultUserMultiplier:   floatToNumeric(in.DefaultUserMultiplier),
		UserDefaultVisible:      in.UserDefaultVisible,
		AllowProtocolConversion: in.AllowProtocolConversion,
		SortOrder:               int32(in.SortOrder),
		Status:                  nextStatus,
		TenantID:                scope.TenantID,
	})
	if err != nil {
		return commercial.Group{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.Group{}, err
	}
	return legacyGroupToCommercial(groupFromRow(item)), nil
}

func (r *CommercialRepo) UpdateGroupStatus(ctx context.Context, scope commercial.TenantGroupScope, status commercial.Status) (commercial.Group, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.Group{}, domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.Group{}, err
	}
	defer tx.Rollback(ctx)
	group, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return commercial.Group{}, err
	}
	nextStatus := commercialStatusOrDefault(status)
	if nextStatus == "active" {
		priceBookID, parseErr := akUUID(group.PriceBookID)
		if parseErr != nil {
			return commercial.Group{}, parseErr
		}
		if err := validateVisibleActivePriceBook(ctx, tx, scope.TenantID, priceBookID, group.ID, group.Name); err != nil {
			return commercial.Group{}, err
		}
		if err := validateActiveRulesAgainstBook(ctx, tx, group, priceBookID); err != nil {
			return commercial.Group{}, err
		}
	}
	item, err := r.q.WithTx(tx).UpdateGroupStatus(ctx, dbgen.UpdateGroupStatusParams{ID: gid, Status: nextStatus, TenantID: scope.TenantID})
	if err != nil {
		return commercial.Group{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.Group{}, err
	}
	return legacyGroupToCommercial(groupFromRow(item)), nil
}

func (r *CommercialRepo) DeleteGroup(ctx context.Context, scope commercial.TenantGroupScope) error {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	group, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return err
	}
	var deps domain.GroupDependencyCounts
	if err := tx.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM ai_user_groups WHERE group_id = $1),
			(SELECT COUNT(*) FROM ai_api_keys WHERE group_id = $1),
			(SELECT COUNT(DISTINCT plan_id) FROM ai_sub_plan_groups WHERE group_id = $1)
	`, gid).Scan(&deps.UserBindings, &deps.APIKeyBindings, &deps.SubscriptionPlans); err != nil {
		return err
	}
	if deps.Total() > 0 {
		return &domain.GroupInUseError{GroupID: group.ID, GroupName: group.Name, Dependencies: deps}
	}
	for _, query := range []string{
		`DELETE FROM ai_group_client_surfaces WHERE group_id = $1`,
		`DELETE FROM ai_group_model_dispatch_rules WHERE group_id = $1`,
		`DELETE FROM ai_group_targets WHERE group_id = $1`,
	} {
		if _, err := tx.Exec(ctx, query, gid); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ai_groups WHERE id = $1 AND tenant_id = $2`, gid, scope.TenantID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *CommercialRepo) LoadDispatchData(ctx context.Context, tenantID string, groupIDs []string) (commercial.DispatchData, error) {
	out := commercial.DispatchData{
		ClientSurfaces: make(map[string][]commercial.GroupClientSurface, len(groupIDs)),
		Rules:          make(map[string][]commercial.DispatchRule, len(groupIDs)),
		Targets:        make(map[string][]commercial.GroupTarget, len(groupIDs)),
	}
	if len(groupIDs) == 0 {
		return out, nil
	}

	ids := make([]pgtype.UUID, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		id, err := akUUID(groupID)
		if err != nil {
			return commercial.DispatchData{}, domain.NewValidationError("group_id", "invalid group_id")
		}
		ids = append(ids, id)
	}

	batch := &pgx.Batch{}
	batch.Queue(`
		SELECT s.id::text, s.group_id::text, s.surface, s.bridge_enabled, s.status, s.created_at, s.updated_at
		FROM ai_group_client_surfaces s
		JOIN ai_groups g ON g.id = s.group_id
		WHERE s.group_id = ANY($1::uuid[]) AND g.tenant_id = $2
		ORDER BY s.group_id, s.surface ASC
	`, ids, tenantID)
	batch.Queue(`
		SELECT r.id::text, r.group_id::text, r.client_surface, r.match_type, r.match_value,
		       r.target_model_code, r.priority, r.status, r.notes,
		       r.created_at, r.updated_at
		FROM ai_group_model_dispatch_rules r
		JOIN ai_groups g ON g.id = r.group_id
		WHERE r.group_id = ANY($1::uuid[]) AND g.tenant_id = $2
		ORDER BY r.group_id, r.priority ASC, r.created_at ASC, r.id ASC
	`, ids, tenantID)
	batch.Queue(`
		SELECT id::text, group_id::text, target_kind, target_id::text, priority, status, created_at, updated_at
		FROM ai_group_targets gt
		WHERE gt.group_id = ANY($1::uuid[])
		  AND EXISTS (
		    SELECT 1
		    FROM ai_groups g
		    JOIN ai_upstream_resources r ON r.resource_kind = gt.target_kind AND r.id = gt.target_id
		    WHERE g.id = gt.group_id AND g.tenant_id = $2
		      AND (
		        r.tenant_access_mode = 'public'
		        OR EXISTS (
		          SELECT 1 FROM ai_upstream_resource_tenant_policies rg
		          WHERE rg.resource_kind = r.resource_kind AND rg.resource_id = r.id AND rg.tenant_id = g.tenant_id
		            AND rg.access_granted
		        )
		      )
		  )
		ORDER BY gt.group_id, gt.priority ASC, gt.target_kind ASC, gt.target_id ASC
	`, ids, tenantID)

	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()

	surfaceRows, err := results.Query()
	if err != nil {
		return commercial.DispatchData{}, err
	}
	for surfaceRows.Next() {
		item, scanErr := scanCommercialGroupClientSurfaceRow(surfaceRows)
		if scanErr != nil {
			surfaceRows.Close()
			return commercial.DispatchData{}, scanErr
		}
		out.ClientSurfaces[item.GroupID] = append(out.ClientSurfaces[item.GroupID], item)
	}
	if err := surfaceRows.Err(); err != nil {
		surfaceRows.Close()
		return commercial.DispatchData{}, err
	}
	surfaceRows.Close()

	ruleRows, err := results.Query()
	if err != nil {
		return commercial.DispatchData{}, err
	}
	for ruleRows.Next() {
		item, scanErr := scanCommercialDispatchRuleRow(ruleRows)
		if scanErr != nil {
			ruleRows.Close()
			return commercial.DispatchData{}, scanErr
		}
		out.Rules[item.GroupID] = append(out.Rules[item.GroupID], item)
	}
	if err := ruleRows.Err(); err != nil {
		ruleRows.Close()
		return commercial.DispatchData{}, err
	}
	ruleRows.Close()

	targetRows, err := results.Query()
	if err != nil {
		return commercial.DispatchData{}, err
	}
	for targetRows.Next() {
		item, scanErr := scanCommercialGroupTargetRow(targetRows)
		if scanErr != nil {
			targetRows.Close()
			return commercial.DispatchData{}, scanErr
		}
		out.Targets[item.GroupID] = append(out.Targets[item.GroupID], item)
	}
	if err := targetRows.Err(); err != nil {
		targetRows.Close()
		return commercial.DispatchData{}, err
	}
	targetRows.Close()
	if err := results.Close(); err != nil {
		return commercial.DispatchData{}, err
	}
	return out, nil
}

func scanCommercialDispatchRuleRow(scanner interface {
	Scan(dest ...any) error
}) (commercial.DispatchRule, error) {
	var (
		item                     commercial.DispatchRule
		clientSurface, matchType string
		priority                 int32
		status, notes            string
	)
	if err := scanner.Scan(
		&item.ID,
		&item.GroupID,
		&clientSurface,
		&matchType,
		&item.MatchValue,
		&item.TargetModelID,
		&priority,
		&status,
		&notes,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return commercial.DispatchRule{}, err
	}
	item.ClientSurface = surface.ID(clientSurface)
	item.MatchType = commercial.DispatchMatchType(matchType)
	item.Priority = int(priority)
	item.Status = commercial.Status(status)
	item.Notes = notes
	return item, nil
}

func scanCommercialDispatchRuleWithPriceRow(scanner interface {
	Scan(dest ...any) error
}) (commercial.DispatchRule, error) {
	var (
		item                     commercial.DispatchRule
		clientSurface, matchType string
		priority                 int32
		status, notes            string
		priced                   bool
	)
	if err := scanner.Scan(
		&item.ID,
		&item.GroupID,
		&clientSurface,
		&matchType,
		&item.MatchValue,
		&item.TargetModelID,
		&priority,
		&status,
		&notes,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.RequiredCapability,
		&priced,
	); err != nil {
		return commercial.DispatchRule{}, err
	}
	item.ClientSurface = surface.ID(clientSurface)
	item.MatchType = commercial.DispatchMatchType(matchType)
	item.Priority = int(priority)
	item.Status = commercial.Status(status)
	item.Notes = notes
	item.CanEnable = priced
	item.PriceState = commercial.DispatchRulePriceStateUnpriced
	if priced {
		item.PriceState = commercial.DispatchRulePriceStatePriced
	}
	return item, nil
}

func setDispatchRulePriceMetadata(ctx context.Context, tx pgx.Tx, priceBookID string, item *commercial.DispatchRule) error {
	item.RequiredCapability = dispatchRequiredCapability(string(item.ClientSurface))
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM ai_price_book_entries
			WHERE price_book_id = $1::uuid AND model_code = $2 AND capability_type = $3
		)
	`, priceBookID, item.TargetModelID, item.RequiredCapability).Scan(&item.CanEnable); err != nil {
		return err
	}
	item.PriceState = commercial.DispatchRulePriceStateUnpriced
	if item.CanEnable {
		item.PriceState = commercial.DispatchRulePriceStatePriced
	}
	return nil
}

func scanCommercialGroupTargetRow(scanner interface {
	Scan(dest ...any) error
}) (commercial.GroupTarget, error) {
	var (
		item               commercial.GroupTarget
		targetKind, status string
		priority           int32
	)
	if err := scanner.Scan(
		&item.ID,
		&item.GroupID,
		&targetKind,
		&item.TargetID,
		&priority,
		&status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return commercial.GroupTarget{}, err
	}
	item.TargetKind = commercial.TargetKind(targetKind)
	item.Priority = int(priority)
	item.Status = commercial.Status(status)
	return item, nil
}

func (r *CommercialRepo) ReplaceGroupClientSurfaces(ctx context.Context, scope commercial.TenantGroupScope, entries []commercial.GroupClientSurfaceWrite) error {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return domain.NewValidationError("group_id", "invalid group_id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ai_groups WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, gid, scope.TenantID).Scan(&lockedID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ai_group_client_surfaces WHERE group_id = $1`, gid); err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_group_client_surfaces (group_id, surface, bridge_enabled, status)
			VALUES ($1, $2, $3, $4)
		`, gid, entry.Surface, entry.BridgeEnabled, commercialStatusOrDefault(entry.Status)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *CommercialRepo) ListGroupClientSurfaces(ctx context.Context, scope commercial.TenantGroupScope) ([]commercial.GroupClientSurface, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return nil, domain.NewValidationError("group_id", "invalid group_id")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT s.id::text, s.group_id::text, s.surface, s.bridge_enabled, s.status, s.created_at, s.updated_at
		FROM ai_group_client_surfaces s
		JOIN ai_groups g ON g.id = s.group_id AND g.tenant_id = $2
		WHERE s.group_id = $1
		ORDER BY s.surface ASC
	`, gid, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]commercial.GroupClientSurface, 0)
	for rows.Next() {
		item, scanErr := scanCommercialGroupClientSurfaceRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *CommercialRepo) AddGroupTarget(ctx context.Context, scope commercial.TenantGroupScope, in commercial.GroupTargetWrite) (commercial.GroupTarget, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.GroupTarget{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	if in.TargetKind != commercial.TargetKindDirectUpstream && in.TargetKind != commercial.TargetKindOAuthPool {
		return commercial.GroupTarget{}, domain.NewValidationError("target_kind", "unsupported target_kind")
	}
	if strings.TrimSpace(in.TargetID) == "" {
		return commercial.GroupTarget{}, domain.NewValidationError("target_id", "target_id is required")
	}
	tid, err := akUUID(in.TargetID)
	if err != nil {
		return commercial.GroupTarget{}, domain.NewValidationError("target_id", "invalid target_id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.GroupTarget{}, err
	}
	defer tx.Rollback(ctx)
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ai_groups WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, gid, scope.TenantID).Scan(&lockedID); err != nil {
		return commercial.GroupTarget{}, err
	}
	var targetStatus string
	var allowed bool
	if err := tx.QueryRow(ctx, `
		SELECT r.status,
		       r.tenant_access_mode = 'public' OR EXISTS (
		         SELECT 1 FROM ai_upstream_resource_tenant_policies rg
		         WHERE rg.resource_kind = r.resource_kind AND rg.resource_id = r.id AND rg.tenant_id = $3
		           AND rg.access_granted
		       )
		FROM ai_upstream_resources r
		WHERE r.resource_kind = $1 AND r.id = $2
	`, string(in.TargetKind), tid, scope.TenantID).Scan(&targetStatus, &allowed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return commercial.GroupTarget{}, domain.NewValidationError("target_id", "target does not exist")
		}
		return commercial.GroupTarget{}, err
	}
	if targetStatus != string(commercial.StatusActive) {
		return commercial.GroupTarget{}, domain.NewValidationError("target_id", "target must be active")
	}
	if !allowed {
		return commercial.GroupTarget{}, domain.NewValidationError("target_id", "target is not available to tenant")
	}
	item, err := scanCommercialGroupTargetRow(tx.QueryRow(ctx, `
		INSERT INTO ai_group_targets (group_id, target_kind, target_id, priority, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, group_id::text, target_kind, target_id::text, priority, status, created_at, updated_at
	`, gid, string(in.TargetKind), tid, in.Priority, commercialStatusOrDefault(in.Status)))
	if err != nil {
		return commercial.GroupTarget{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.GroupTarget{}, err
	}
	return item, nil
}

func (r *CommercialRepo) ListGroupTargets(ctx context.Context, scope commercial.TenantGroupScope) ([]commercial.GroupTarget, error) {
	items, err := r.group.ListGroupTargets(ctx, scope.TenantID, scope.GroupID)
	if err != nil {
		return nil, err
	}
	out := make([]commercial.GroupTarget, 0)
	for _, item := range items {
		out = append(out, groupTargetBindingToCommercial(item))
	}
	return out, nil
}

func (r *CommercialRepo) ListGroupTargetDetails(ctx context.Context, scope commercial.TenantGroupScope) ([]commercial.GroupTargetDetail, error) {
	items, err := r.group.ListGroupTargetDetails(ctx, scope.TenantID, scope.GroupID)
	if err != nil {
		return nil, err
	}
	out := make([]commercial.GroupTargetDetail, 0, len(items))
	for _, item := range items {
		out = append(out, groupTargetDetailToCommercial(item))
	}
	return out, nil
}

func (r *CommercialRepo) ListGroupTargetsByTarget(ctx context.Context, targetKind commercial.TargetKind, targetID string) ([]commercial.GroupTargetDetail, error) {
	items, err := r.group.ListGroupTargetsByTarget(ctx, string(targetKind), targetID)
	if err != nil {
		return nil, err
	}
	out := make([]commercial.GroupTargetDetail, 0, len(items))
	for _, item := range items {
		out = append(out, groupTargetDetailToCommercial(item))
	}
	return out, nil
}

func (r *CommercialRepo) GetGroupTargetDetail(ctx context.Context, scope commercial.TenantGroupScope, id string) (commercial.GroupTargetDetail, error) {
	item, err := r.group.GetGroupTargetDetail(ctx, scope.TenantID, scope.GroupID, id)
	if err != nil {
		return commercial.GroupTargetDetail{}, err
	}
	return groupTargetDetailToCommercial(item), nil
}

func (r *CommercialRepo) UpdateGroupTarget(ctx context.Context, scope commercial.TenantGroupScope, id string, in commercial.GroupTargetWrite) (commercial.GroupTarget, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.GroupTarget{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	rid, err := akUUID(id)
	if err != nil {
		return commercial.GroupTarget{}, domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.GroupTarget{}, err
	}
	defer tx.Rollback(ctx)
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ai_groups WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, gid, scope.TenantID).Scan(&lockedID); err != nil {
		return commercial.GroupTarget{}, err
	}
	var existingKind, existingTargetID string
	if err := tx.QueryRow(ctx, `
		SELECT target_kind, target_id::text
		FROM ai_group_targets
		WHERE id = $1 AND group_id = $2
	`, rid, gid).Scan(&existingKind, &existingTargetID); err != nil {
		return commercial.GroupTarget{}, err
	}
	if in.TargetKind != "" && string(in.TargetKind) != existingKind {
		return commercial.GroupTarget{}, domain.NewValidationError("target_kind", "group target kind cannot be changed")
	}
	if strings.TrimSpace(in.TargetID) != "" && strings.TrimSpace(in.TargetID) != existingTargetID {
		return commercial.GroupTarget{}, domain.NewValidationError("target_id", "group target id cannot be changed")
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `
		SELECT r.tenant_access_mode = 'public' OR EXISTS (
		  SELECT 1 FROM ai_upstream_resource_tenant_policies rg
		  WHERE rg.resource_kind = r.resource_kind AND rg.resource_id = r.id AND rg.tenant_id = $3
		    AND rg.access_granted
		)
		FROM ai_upstream_resources r
		WHERE r.resource_kind = $1 AND r.id = $2::uuid
	`, existingKind, existingTargetID, scope.TenantID).Scan(&allowed); err != nil {
		return commercial.GroupTarget{}, err
	}
	if !allowed {
		return commercial.GroupTarget{}, domain.NewValidationError("target_id", "target is not available to tenant")
	}
	item, err := scanCommercialGroupTargetRow(tx.QueryRow(ctx, `
		UPDATE ai_group_targets
		SET priority = $1, status = $2, updated_at = now()
		WHERE id = $3 AND group_id = $4
		RETURNING id::text, group_id::text, target_kind, target_id::text, priority, status, created_at, updated_at
	`, in.Priority, commercialStatusOrDefault(in.Status), rid, gid))
	if err != nil {
		return commercial.GroupTarget{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.GroupTarget{}, err
	}
	return item, nil
}

func (r *CommercialRepo) DeleteGroupTarget(ctx context.Context, scope commercial.TenantGroupScope, id string) error {
	rid, err := akUUID(id)
	if err != nil {
		return domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var groupID string
	if err := tx.QueryRow(ctx, `
		SELECT gt.group_id::text FROM ai_group_targets gt
		JOIN ai_groups g ON g.id = gt.group_id AND g.tenant_id = $2
		WHERE gt.id = $1
	`, rid, scope.TenantID).Scan(&groupID); err != nil {
		return err
	}
	var lockedID string
	if groupID != scope.GroupID {
		return domain.ErrNotFound
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ai_groups WHERE id = $1::uuid AND tenant_id = $2 FOR UPDATE`, groupID, scope.TenantID).Scan(&lockedID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM ai_group_targets WHERE id = $1 AND group_id = $2`, rid, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *CommercialRepo) AddDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, in commercial.DispatchRuleWrite) (commercial.DispatchRule, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	defer tx.Rollback(ctx)
	group, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := validateRulePrice(ctx, tx, group, "", string(in.ClientSurface), in.MatchValue, in.TargetModelID); err != nil {
		return commercial.DispatchRule{}, err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO ai_group_model_dispatch_rules (
			group_id,
			client_surface,
			match_type,
			match_value,
			target_model_code,
			priority,
			status,
			notes
		) VALUES ($1, $2, $3, $4, $5, $6, 'active', $7)
		RETURNING id::text, group_id::text, client_surface, match_type, match_value, target_model_code,
		          priority, status, notes, created_at, updated_at
	`, gid, string(in.ClientSurface), string(in.MatchType), in.MatchValue, in.TargetModelID, in.Priority, in.Notes)
	item, err := scanCommercialDispatchRuleRow(row)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := setDispatchRulePriceMetadata(ctx, tx, group.PriceBookID, &item); err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.DispatchRule{}, err
	}
	return item, nil
}

func (r *CommercialRepo) ListDispatchRules(ctx context.Context, scope commercial.TenantGroupScope) ([]commercial.DispatchRule, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return nil, domain.NewValidationError("group_id", "invalid group_id")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT r.id::text, r.group_id::text, r.client_surface, r.match_type, r.match_value, r.target_model_code,
		       r.priority, r.status, r.notes, r.created_at, r.updated_at,
		       CASE
		         WHEN r.client_surface IN ('openai_embeddings', 'gemini_embeddings') THEN 'embedding'
		         WHEN r.client_surface IN ('openai_images', 'gemini_images') THEN 'image'
		         ELSE 'chat'
		       END AS required_capability,
		       EXISTS (
		         SELECT 1 FROM ai_price_book_entries e
		         WHERE e.price_book_id = g.retail_price_book_id
		           AND e.model_code = r.target_model_code
		           AND e.capability_type = CASE
		             WHEN r.client_surface IN ('openai_embeddings', 'gemini_embeddings') THEN 'embedding'
		             WHEN r.client_surface IN ('openai_images', 'gemini_images') THEN 'image'
		             ELSE 'chat'
		           END
		       ) AS priced
		FROM ai_group_model_dispatch_rules r
		JOIN ai_groups g ON g.id = r.group_id
		WHERE r.group_id = $1 AND g.tenant_id = $2
		ORDER BY r.priority ASC, r.created_at ASC
	`, gid, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]commercial.DispatchRule, 0)
	for rows.Next() {
		item, scanErr := scanCommercialDispatchRuleWithPriceRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *CommercialRepo) ListDispatchModels(ctx context.Context, scope commercial.TenantGroupScope, clientSurface surface.ID) ([]commercial.DispatchModel, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return nil, domain.NewValidationError("group_id", "invalid group_id")
	}
	capability := dispatchRequiredCapability(string(clientSurface))
	rows, err := r.pool.Query(ctx, `
		SELECT e.model_code,
		       e.capability_type,
		       COUNT(DISTINCT CASE WHEN gt.status = 'active' AND um.status = 'active' THEN gt.target_id END)::int
		FROM ai_groups g
		JOIN ai_price_book_entries e ON e.price_book_id = g.retail_price_book_id
		LEFT JOIN ai_group_targets gt ON gt.group_id = g.id
		LEFT JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.model_code = e.model_code
		 AND um.capability_type = e.capability_type
		WHERE g.id = $1 AND g.tenant_id = $2 AND e.capability_type = $3
		GROUP BY e.model_code, e.capability_type
		ORDER BY e.model_code ASC
	`, gid, scope.TenantID, capability)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]commercial.DispatchModel, 0)
	for rows.Next() {
		var item commercial.DispatchModel
		if err := rows.Scan(&item.ModelCode, &item.Capability, &item.AvailableTargets); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *CommercialRepo) UpdateDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, id string, in commercial.DispatchRuleWrite) (commercial.DispatchRule, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	rid, err := akUUID(id)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	defer tx.Rollback(ctx)
	group, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM ai_group_model_dispatch_rules WHERE id = $1 AND group_id = $2`, rid, gid).Scan(&currentStatus); err != nil {
		return commercial.DispatchRule{}, err
	}
	if currentStatus == string(commercial.StatusActive) {
		if err := validateRulePrice(ctx, tx, group, id, string(in.ClientSurface), in.MatchValue, in.TargetModelID); err != nil {
			return commercial.DispatchRule{}, err
		}
	}
	row := tx.QueryRow(ctx, `
		UPDATE ai_group_model_dispatch_rules
		SET client_surface = $1,
		    match_type = $2,
		    match_value = $3,
		    target_model_code = $4,
		    priority = $5,
		    notes = $6,
		    updated_at = now()
		WHERE id = $7 AND group_id = $8
		RETURNING id::text, group_id::text, client_surface, match_type, match_value, target_model_code,
		          priority, status, notes, created_at, updated_at
	`, string(in.ClientSurface), string(in.MatchType), in.MatchValue, in.TargetModelID, in.Priority, in.Notes, rid, gid)
	item, err := scanCommercialDispatchRuleRow(row)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := setDispatchRulePriceMetadata(ctx, tx, group.PriceBookID, &item); err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.DispatchRule{}, err
	}
	return item, nil
}

func (r *CommercialRepo) UpdateDispatchRuleStatus(ctx context.Context, scope commercial.TenantGroupScope, id string, status commercial.Status) (commercial.DispatchRule, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	rid, err := akUUID(id)
	if err != nil {
		return commercial.DispatchRule{}, domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	defer tx.Rollback(ctx)
	group, err := lockGroupForTenant(ctx, tx, scope.TenantID, gid)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	var clientSurface, matchValue, targetModel string
	if err := tx.QueryRow(ctx, `SELECT client_surface, match_value, target_model_code FROM ai_group_model_dispatch_rules WHERE id = $1 AND group_id = $2`, rid, gid).Scan(&clientSurface, &matchValue, &targetModel); err != nil {
		return commercial.DispatchRule{}, err
	}
	if status == commercial.StatusActive {
		if err := validateRulePrice(ctx, tx, group, id, clientSurface, matchValue, targetModel); err != nil {
			return commercial.DispatchRule{}, err
		}
	}
	row := tx.QueryRow(ctx, `
		UPDATE ai_group_model_dispatch_rules SET status = $1, updated_at = now()
		WHERE id = $2 AND group_id = $3
		RETURNING id::text, group_id::text, client_surface, match_type, match_value, target_model_code,
		          priority, status, notes, created_at, updated_at
	`, string(status), rid, gid)
	item, err := scanCommercialDispatchRuleRow(row)
	if err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := setDispatchRulePriceMetadata(ctx, tx, group.PriceBookID, &item); err != nil {
		return commercial.DispatchRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commercial.DispatchRule{}, err
	}
	return item, nil
}

func (r *CommercialRepo) DeleteDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, id string) error {
	rid, err := akUUID(id)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var groupID string
	if err := tx.QueryRow(ctx, `
		SELECT r.group_id::text
		FROM ai_group_model_dispatch_rules r
		JOIN ai_groups g ON g.id = r.group_id
		WHERE r.id = $1 AND g.tenant_id = $2
	`, rid, scope.TenantID).Scan(&groupID); err != nil {
		return err
	}
	if groupID != scope.GroupID {
		return domain.ErrNotFound
	}
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ai_groups WHERE id = $1::uuid AND tenant_id = $2 FOR UPDATE`, groupID, scope.TenantID).Scan(&lockedID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM ai_group_model_dispatch_rules WHERE id = $1 AND group_id = $2`, rid, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *CommercialRepo) PreviewDispatch(ctx context.Context, scope commercial.TenantGroupScope, requestedModel string, clientSurface surface.ID) (commercial.DispatchPreview, error) {
	if r.runtimeResolver == nil {
		return commercial.DispatchPreview{}, errors.New("runtime resolver is not configured")
	}
	return r.previewWithRuntime(ctx, scope.TenantID, scope.GroupID, requestedModel, string(clientSurface))
}

func (r *CommercialRepo) previewWithRuntime(ctx context.Context, tenantID, groupID, requestedModel, clientSurface string) (commercial.DispatchPreview, error) {
	surfaceID := surface.ID(clientSurface)
	capability := catalog.CapabilityChat
	if surfaceID == surface.OpenAIEmbeddings || surfaceID == surface.GeminiEmbeddings {
		capability = catalog.CapabilityEmbedding
	} else if surfaceID == surface.OpenAIImages || surfaceID == surface.GeminiImages {
		capability = catalog.CapabilityImageGeneration
	}
	inspection, err := r.runtimeResolver.Inspect(ctx, identity.Subject{TenantID: tenantID, ForcedGroupID: groupID}, coreruntime.Request{
		RequestedModel: requestedModel, ClientSurface: surfaceID, Capability: capability, ForcedGroupID: groupID,
	})
	if err != nil {
		return commercial.DispatchPreview{}, err
	}
	out := commercial.DispatchPreview{
		RequestedModel:     requestedModel,
		ClientSurface:      clientSurface,
		ResolvedModelID:    requestedModel,
		CandidateUpstreams: make([]commercial.DispatchPreviewCandidate, 0, len(inspection.Candidates)),
		RejectedCandidates: make([]commercial.DispatchPreviewRejection, 0, len(inspection.RejectedCandidates)),
	}
	for _, candidate := range inspection.Candidates {
		if out.ResolvedModelID == requestedModel {
			out.ResolvedModelID = candidate.ModelID
		}
		if candidate.MatchedRule != nil && out.MatchedRule == nil {
			rule := *candidate.MatchedRule
			out.MatchedRule = &rule
		}
		binding := candidate.Binding
		item := commercial.DispatchPreviewCandidate{DisplayName: binding.Upstream.Name, ProviderFamily: string(binding.Upstream.ProviderFamily), UpstreamModel: binding.ModelBinding.UpstreamModelName, ProtocolConversion: binding.ModelBinding.RequestSurface != surfaceID, Priority: candidate.Target.Priority}
		if candidate.Target.TargetKind == commercial.TargetKindOAuthPool {
			item.TargetType = "pool"
			item.CredentialPoolID = binding.Upstream.ID
		} else {
			item.TargetType = "account"
			item.AccountID = binding.Upstream.ID
		}
		if protocol, protocolErr := dispatchProtocolFromSurface(binding.ModelBinding.RequestSurface); protocolErr == nil {
			item.SelectedProtocol = string(protocol)
		}
		out.CandidateUpstreams = append(out.CandidateUpstreams, item)
	}
	for _, rejected := range inspection.RejectedCandidates {
		if out.ResolvedModelID == requestedModel {
			out.ResolvedModelID = rejected.ModelID
		}
		if rejected.MatchedRule != nil && out.MatchedRule == nil {
			rule := *rejected.MatchedRule
			out.MatchedRule = &rule
		}
		item := commercial.DispatchPreviewRejection{
			TargetID:        rejected.Target.TargetID,
			ResolvedModelID: rejected.ModelID,
			ReasonCode:      string(rejected.Code),
			ReasonDetail:    rejected.Detail,
			Priority:        rejected.Target.Priority,
		}
		switch rejected.Target.TargetKind {
		case commercial.TargetKindOAuthPool:
			item.TargetType = "pool"
		case commercial.TargetKindDirectUpstream:
			item.TargetType = "account"
		}
		out.RejectedCandidates = append(out.RejectedCandidates, item)
	}
	return out, nil
}

func (r *CommercialRepo) UpsertUserBinding(ctx context.Context, in commercial.UserGroupBindingWrite) (commercial.UserGroupBinding, error) {
	item, err := r.group.UpsertUserGroup(ctx, commercial.UserGroupBindingWrite{
		TenantID:               in.TenantID,
		UserID:                 in.UserID,
		GroupID:                in.GroupID,
		UserMultiplierOverride: in.UserMultiplierOverride,
		CreatedBy:              in.CreatedBy,
		UpdatedBy:              in.UpdatedBy,
	})
	if err != nil {
		return commercial.UserGroupBinding{}, err
	}
	return legacyUserBindingToCommercial(item), nil
}

func (r *CommercialRepo) ListUserBindings(ctx context.Context, tenantID, userID string) ([]commercial.UserGroupBinding, error) {
	items, err := r.group.ListUserGroups(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]commercial.UserGroupBinding, 0, len(items))
	for _, item := range items {
		out = append(out, legacyUserBindingToCommercial(item))
	}
	return out, nil
}

func (r *CommercialRepo) DeleteUserBinding(ctx context.Context, tenantID, userID, groupID string) error {
	return r.group.DeleteUserGroup(ctx, tenantID, userID, groupID)
}

func (r *CommercialRepo) CreateLimitPolicy(ctx context.Context, in commercial.LimitPolicyWrite) (commercial.LimitPolicy, error) {
	if err := validateLegacyLimitBridge(in); err != nil {
		return commercial.LimitPolicy{}, err
	}
	item, err := r.limit.Create(ctx, in)
	if err != nil {
		return commercial.LimitPolicy{}, err
	}
	return item.ToCore(), nil
}

func (r *CommercialRepo) ListLimitPolicies(ctx context.Context, filter commercial.LimitPolicyFilter) ([]commercial.LimitPolicy, error) {
	items, err := r.limit.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]commercial.LimitPolicy, 0, len(items))
	for _, item := range items {
		mapped := item.ToCore()
		if filter.ScopeType != "" && mapped.ScopeType != filter.ScopeType {
			continue
		}
		if filter.ScopeID != "" && mapped.ScopeID != filter.ScopeID {
			continue
		}
		if filter.Status != "" && mapped.Status != filter.Status {
			continue
		}
		out = append(out, mapped)
	}
	return out, nil
}

func (r *CommercialRepo) UpdateLimitPolicy(ctx context.Context, id string, in commercial.LimitPolicyWrite) (commercial.LimitPolicy, error) {
	if err := validateLegacyLimitBridge(in); err != nil {
		return commercial.LimitPolicy{}, err
	}
	item, err := r.limit.Update(ctx, id, in)
	if err != nil {
		return commercial.LimitPolicy{}, err
	}
	return item.ToCore(), nil
}

func (r *CommercialRepo) UpdateLimitPolicyStatus(ctx context.Context, id string, status commercial.Status) (commercial.LimitPolicy, error) {
	item, err := r.limit.UpdateStatus(ctx, id, commercialStatusOrDefault(status))
	if err != nil {
		return commercial.LimitPolicy{}, err
	}
	return item.ToCore(), nil
}

func (r *CommercialRepo) DeleteLimitPolicies(ctx context.Context, filter commercial.LimitPolicyFilter) error {
	if filter.ScopeType == "" || filter.ScopeID == "" {
		return fmt.Errorf("scope_type and scope_id are required")
	}
	return r.limit.DeleteByScope(ctx, string(filter.ScopeType), filter.ScopeID)
}

func (r *CommercialRepo) UpsertRoutingPolicy(ctx context.Context, in commercial.RoutingPolicyWrite) (routing.Policy, error) {
	scopeKey, err := encodeRoutingScope(in.ScopeType, in.ScopeID)
	if err != nil {
		return routing.Policy{}, err
	}
	if err := r.weights.Upsert(ctx, scopeKey, serving.ScoreWeights{
		Cost:    in.Weights.Cost,
		Latency: in.Weights.Latency,
		Load:    in.Weights.Load,
		Health:  in.Weights.Health,
	}); err != nil {
		return routing.Policy{}, err
	}
	return r.getRoutingPolicyByScope(ctx, scopeKey)
}

func (r *CommercialRepo) ListRoutingPolicies(ctx context.Context) ([]routing.Policy, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, scope, weights, updated_at
		FROM ai_route_score_weights
		ORDER BY scope ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]routing.Policy, 0, 8)
	for rows.Next() {
		var (
			id        pgtype.UUID
			scopeKey  string
			weights   []byte
			updatedAt pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &scopeKey, &weights, &updatedAt); err != nil {
			return nil, err
		}
		item, err := routingPolicyFromRow(id, scopeKey, weights, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *CommercialRepo) getRoutingPolicyByScope(ctx context.Context, scopeKey string) (routing.Policy, error) {
	var (
		id        pgtype.UUID
		weights   []byte
		updatedAt pgtype.Timestamptz
	)
	if err := r.pool.QueryRow(ctx, `
		SELECT id, weights, updated_at
		FROM ai_route_score_weights
		WHERE scope = $1
	`, scopeKey).Scan(&id, &weights, &updatedAt); err != nil {
		return routing.Policy{}, err
	}
	return routingPolicyFromRow(id, scopeKey, weights, updatedAt)
}

func legacyGroupToCommercial(item domain.Group) commercial.Group {
	return commercial.Group{
		ID:                      item.ID,
		TenantID:                item.TenantID,
		Code:                    item.Name,
		Name:                    item.Name,
		Description:             item.Description,
		RetailPriceBookID:       item.RetailPriceBookID,
		DefaultUserMultiplier:   item.DefaultUserMultiplier,
		UserDefaultVisible:      item.UserDefaultVisible,
		AllowProtocolConversion: item.AllowProtocolConversion,
		Status:                  commercial.Status(item.Status),
		SortOrder:               int(item.SortOrder),
		CreatedAt:               item.CreatedAt,
		UpdatedAt:               item.UpdatedAt,
	}
}

func groupTargetBindingToCommercial(item domain.GroupTargetBinding) commercial.GroupTarget {
	return commercial.GroupTarget{
		ID:         item.ID,
		GroupID:    item.GroupID,
		TargetKind: commercial.TargetKind(item.TargetKind),
		TargetID:   item.TargetID,
		Priority:   int(item.Priority),
		Status:     commercial.Status(item.Status),
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}
}

func groupTargetDetailToCommercial(item domain.GroupTargetDetail) commercial.GroupTargetDetail {
	return commercial.GroupTargetDetail{
		GroupTarget:       groupTargetBindingToCommercial(item.GroupTargetBinding),
		AccountName:       item.AccountName,
		DefaultProtocol:   item.DefaultProtocol,
		PoolName:          item.PoolName,
		FixedProviderType: item.FixedProviderType,
		Available:         item.Available,
		UnavailableReason: item.UnavailableReason,
	}
}

func dispatchRuleToCommercial(item domain.GroupModelDispatchRule) (commercial.DispatchRule, error) {
	return commercial.DispatchRule{
		ID:            item.ID,
		GroupID:       item.GroupID,
		ClientSurface: surface.ID(item.ClientSurface),
		MatchType:     commercial.DispatchMatchType(item.MatchType),
		MatchValue:    item.MatchValue,
		TargetModelID: item.TargetModelCode,
		Priority:      int(item.Priority),
		Status:        commercial.Status(item.Status),
		Notes:         item.Notes,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}, nil
}

func legacyUserBindingToCommercial(item domain.UserGroup) commercial.UserGroupBinding {
	return commercial.UserGroupBinding{
		ID:                     item.ID,
		TenantID:               item.TenantID,
		UserID:                 item.UserID,
		GroupID:                item.GroupID,
		UserMultiplierOverride: item.UserMultiplierOverride,
		CreatedBy:              item.CreatedBy,
		UpdatedBy:              item.CreatedBy,
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}
}

func routingPolicyFromRow(id pgtype.UUID, scopeKey string, weightsJSON []byte, updatedAt pgtype.Timestamptz) (routing.Policy, error) {
	var weights routing.WeightSet
	if len(weightsJSON) > 0 {
		if err := json.Unmarshal(weightsJSON, &weights); err != nil {
			return routing.Policy{}, fmt.Errorf("unmarshal routing weights for %s: %w", scopeKey, err)
		}
	}
	scopeType, scopeID := decodeRoutingScope(scopeKey)
	return routing.Policy{
		ID:        uuidToString(id),
		ScopeType: scopeType,
		ScopeID:   scopeID,
		Weights:   weights,
		CreatedAt: updatedAt.Time,
		UpdatedAt: updatedAt.Time,
	}, nil
}

func scanCommercialGroupClientSurfaceRow(scanner interface {
	Scan(dest ...any) error
}) (commercial.GroupClientSurface, error) {
	var item commercial.GroupClientSurface
	if err := scanner.Scan(
		&item.ID,
		&item.GroupID,
		&item.Surface,
		&item.BridgeEnabled,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return commercial.GroupClientSurface{}, err
	}
	return item, nil
}

func commercialNameOrCode(name, code string) string {
	return firstNonEmpty(strings.TrimSpace(name), strings.TrimSpace(code))
}

func commercialStatusOrDefault(status commercial.Status) string {
	switch status {
	case commercial.StatusDisabled:
		return string(commercial.StatusDisabled)
	case commercial.StatusActive:
		return string(commercial.StatusActive)
	default:
		return string(commercial.StatusActive)
	}
}

func validateLegacyLimitBridge(in commercial.LimitPolicyWrite) error {
	if strings.TrimSpace(in.ScopeID) == "" {
		return domain.NewValidationError("scope_id", "scope_id is required")
	}
	if in.Capability != "" {
		return domain.NewValidationError("capability", "current postgres commercial adapter only supports subject-scope limit policies")
	}
	if strings.TrimSpace(in.ModelID) != "" {
		return domain.NewValidationError("model_id", "current postgres commercial adapter only supports subject-scope limit policies")
	}
	return nil
}

func providerFamilyToLegacy(raw string) string {
	switch catalog.ProviderFamily(raw) {
	case catalog.ProviderFamilyOpenAICompatible:
		return string(catalog.ProviderFamilyOpenAICompatible)
	case catalog.ProviderFamilyAnthropic:
		return string(catalog.ProviderFamilyAnthropic)
	case catalog.ProviderFamilyGoogle:
		return string(domain.EndpointProtocolGemini)
	default:
		return ""
	}
}

func legacyProviderFamilyToCatalog(raw string) catalog.ProviderFamily {
	switch raw {
	case string(catalog.ProviderFamilyOpenAICompatible):
		return catalog.ProviderFamilyOpenAICompatible
	case string(catalog.ProviderFamilyAnthropic):
		return catalog.ProviderFamilyAnthropic
	case string(catalog.ProviderFamilyGoogle), string(domain.EndpointProtocolGemini):
		return catalog.ProviderFamilyGoogle
	default:
		return ""
	}
}

func encodeRoutingScope(scopeType routing.ScopeType, scopeID string) (string, error) {
	switch scopeType {
	case routing.ScopeGlobal:
		return legacyRoutingScopeGlobal, nil
	case routing.ScopeTenant, routing.ScopeGroup, routing.ScopeUpstream:
		scopeID = strings.TrimSpace(scopeID)
		if scopeID == "" {
			return "", domain.NewValidationError("scope_id", "scope_id is required")
		}
		return string(scopeType) + ":" + scopeID, nil
	default:
		return "", domain.NewValidationError("scope_type", "unsupported scope_type")
	}
}

func decodeRoutingScope(scopeKey string) (routing.ScopeType, string) {
	if scopeKey == legacyRoutingScopeGlobal {
		return routing.ScopeGlobal, legacyRoutingScopeGlobal
	}
	parts := strings.SplitN(scopeKey, ":", 2)
	if len(parts) == 2 {
		switch routing.ScopeType(parts[0]) {
		case routing.ScopeTenant, routing.ScopeGroup, routing.ScopeUpstream:
			return routing.ScopeType(parts[0]), parts[1]
		}
	}
	return routing.ScopeGlobal, scopeKey
}

func intPtrToInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

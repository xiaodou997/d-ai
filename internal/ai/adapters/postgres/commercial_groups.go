package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	commercial "xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/surface"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

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
	item, err := queriesWithTx(tx).CreateGroup(ctx, dbgen.CreateGroupParams{
		TenantID:                tenantID,
		Name:                    name,
		Description:             in.Description,
		RetailPriceBookID:       priceBookID,
		DefaultUserMultiplier:   floatToNumeric(in.DefaultUserMultiplier),
		UserDefaultVisible:      in.UserDefaultVisible,
		AllowProtocolConversion: in.AllowProtocolConversion,
		RoutePolicy:             string(in.RoutePolicy),
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
	if in.ExpectedRoutePolicyVersion > 0 && current.RoutePolicyVersion != in.ExpectedRoutePolicyVersion {
		return commercial.Group{}, &domain.GroupRoutePolicyConflictError{
			GroupID: scope.GroupID, ExpectedVersion: in.ExpectedRoutePolicyVersion,
			ActualVersion: current.RoutePolicyVersion,
		}
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
	item, err := queriesWithTx(tx).UpdateGroup(ctx, dbgen.UpdateGroupParams{
		ID:                      gid,
		Name:                    name,
		Description:             in.Description,
		RetailPriceBookID:       priceBookID,
		DefaultUserMultiplier:   floatToNumeric(in.DefaultUserMultiplier),
		UserDefaultVisible:      in.UserDefaultVisible,
		AllowProtocolConversion: in.AllowProtocolConversion,
		RoutePolicy:             string(in.RoutePolicy),
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

// UpdateGroupRoutePolicy changes only the route-policy columns. Keeping this
// mutation separate from UpdateGroup prevents a route-policy save in the
// tenant UI from racing with unrelated pricing or visibility edits.
func (r *CommercialRepo) UpdateGroupRoutePolicy(ctx context.Context, scope commercial.TenantGroupScope, in commercial.GroupRoutePolicyWrite) (commercial.Group, error) {
	gid, err := akUUID(scope.GroupID)
	if err != nil {
		return commercial.Group{}, domain.NewValidationError("id", "invalid id")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.Group{}, err
	}
	defer tx.Rollback(ctx)
	var actualVersion int64
	if err := tx.QueryRow(ctx, `
		SELECT route_policy_version
		FROM ai_groups
		WHERE id = $1 AND tenant_id = $2
		FOR UPDATE
	`, gid, scope.TenantID).Scan(&actualVersion); err != nil {
		return commercial.Group{}, err
	}
	if actualVersion != in.ExpectedVersion {
		return commercial.Group{}, &domain.GroupRoutePolicyConflictError{
			GroupID:         scope.GroupID,
			ExpectedVersion: in.ExpectedVersion,
			ActualVersion:   actualVersion,
		}
	}
	item, err := queriesWithTx(tx).UpdateGroupRoutePolicy(ctx, dbgen.UpdateGroupRoutePolicyParams{
		ID:                 gid,
		RoutePolicy:        string(in.RoutePolicy),
		TenantID:           scope.TenantID,
		RoutePolicyVersion: in.ExpectedVersion,
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
	item, err := queriesWithTx(tx).UpdateGroupStatus(ctx, dbgen.UpdateGroupStatusParams{ID: gid, Status: nextStatus, TenantID: scope.TenantID})
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
		SELECT id::text, group_id::text, target_kind, target_id::text, status, created_at, updated_at
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
		ORDER BY gt.group_id, gt.target_kind ASC, gt.target_id ASC
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
	)
	if err := scanner.Scan(
		&item.ID,
		&item.GroupID,
		&targetKind,
		&item.TargetID,
		&status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return commercial.GroupTarget{}, err
	}
	item.TargetKind = commercial.TargetKind(targetKind)
	item.Status = commercial.Status(status)
	return item, nil
}

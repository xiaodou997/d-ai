package postgres

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
)

var _ commercial.GroupTransferRepository = (*CommercialRepo)(nil)

type groupTransferQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *CommercialRepo) SnapshotGroupConfigurations(ctx context.Context, tenantID string, groupIDs []string) ([]commercial.GroupConfigurationSnapshot, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	items, err := loadGroupConfigurationSnapshots(ctx, tx, tenantID, groupIDs, nil)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *CommercialRepo) LoadGroupImportEnvironment(ctx context.Context, tenantID string, groupNames, priceBookIDs []string) (commercial.GroupImportEnvironment, error) {
	existing, err := loadGroupConfigurationSnapshots(ctx, r.pool, tenantID, nil, groupNames)
	if err != nil {
		return commercial.GroupImportEnvironment{}, err
	}
	byName := make(map[string]commercial.GroupConfigurationSnapshot, len(existing))
	allPriceBookIDs := append([]string{}, priceBookIDs...)
	for _, item := range existing {
		byName[item.Configuration.Name] = item
		allPriceBookIDs = append(allPriceBookIDs, item.PriceBookID)
	}
	priceBooks, err := loadGroupImportPriceBooks(ctx, r.pool, tenantID, uniqueAdapterStrings(allPriceBookIDs))
	if err != nil {
		return commercial.GroupImportEnvironment{}, err
	}
	return commercial.GroupImportEnvironment{
		ExistingByName: byName,
		PriceBooks:     priceBooks,
	}, nil
}

func (r *CommercialRepo) ApplyGroupImport(ctx context.Context, tenantID string, item commercial.PlannedGroupImport) (commercial.AppliedGroupImport, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercial.AppliedGroupImport{}, err
	}
	defer tx.Rollback(ctx)

	preview := item.Preview
	strategy := item.Source.RouteStrategy
	if strategy == "" {
		strategy = commercial.RouteStrategyAdaptive
	}
	objective := item.Source.RouteObjective
	if objective == "" {
		objective = commercial.RouteObjectiveBalanced
	}
	if strategy != commercial.RouteStrategyAdaptive {
		objective = commercial.RouteObjectiveBalanced
	}
	var existing commercial.GroupConfigurationSnapshot
	if preview.Action == commercial.GroupImportActionUpdate {
		var lockedID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM ai_groups WHERE id = $1::uuid AND tenant_id = $2 FOR UPDATE`, preview.TargetGroupID, tenantID).Scan(&lockedID); err != nil {
			return commercial.AppliedGroupImport{}, err
		}
		items, err := loadGroupConfigurationSnapshots(ctx, tx, tenantID, []string{preview.TargetGroupID}, nil)
		if err != nil {
			return commercial.AppliedGroupImport{}, err
		}
		if len(items) != 1 {
			return commercial.AppliedGroupImport{}, domain.ErrNotFound
		}
		existing = items[0]
	}

	priceBooks, err := loadGroupImportPriceBooks(ctx, tx, tenantID, []string{preview.PriceBookID})
	if err != nil {
		return commercial.AppliedGroupImport{}, err
	}
	priceBook, exists := priceBooks[preview.PriceBookID]
	if !exists || priceBook.Status != commercial.StatusActive {
		return commercial.AppliedGroupImport{}, domain.NewValidationError("price_book_id", "price book is missing or disabled")
	}
	appliedStatus := preview.AppliedStatus
	for _, rule := range item.Source.DispatchRules {
		if rule.Status != commercial.StatusActive {
			continue
		}
		_, compatible := priceBook.SupportsModel(rule.TargetModelID, rule.ClientSurface)
		if !compatible {
			appliedStatus = commercial.StatusDisabled
		}
	}
	if preview.Action != commercial.GroupImportActionUpdate || existing.ActiveTargets == 0 {
		appliedStatus = commercial.StatusDisabled
	}

	groupID := preview.TargetGroupID
	if preview.Action == commercial.GroupImportActionCreate || preview.Action == commercial.GroupImportActionCopy {
		err = tx.QueryRow(ctx, `
			INSERT INTO ai_groups (
				tenant_id, name, description, retail_price_book_id, default_user_multiplier,
				user_default_visible, allow_protocol_conversion, route_strategy, route_objective, sort_order, status
			) VALUES ($1, $2, $3, $4::uuid, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id::text
		`, tenantID, preview.TargetName, item.Source.Description, preview.PriceBookID,
			item.Source.DefaultUserMultiplier, item.Source.UserDefaultVisible,
			item.Source.AllowProtocolConversion, string(strategy), string(objective), item.Source.SortOrder, string(appliedStatus)).Scan(&groupID)
		if err != nil {
			return commercial.AppliedGroupImport{}, err
		}
	} else {
		tag, err := tx.Exec(ctx, `
				UPDATE ai_groups
				SET name = $2,
				    description = $3,
				    retail_price_book_id = $4::uuid,
				    default_user_multiplier = $5,
				    user_default_visible = $6,
				    allow_protocol_conversion = $7,
				    route_strategy = $8,
				    route_objective = $9,
				    route_policy_version = route_policy_version + 1,
				    sort_order = $10,
				    status = $11,
				    updated_at = now()
				WHERE id = $1::uuid AND tenant_id = $12
		`, groupID, preview.TargetName, item.Source.Description, preview.PriceBookID,
			item.Source.DefaultUserMultiplier, item.Source.UserDefaultVisible,
			item.Source.AllowProtocolConversion, string(strategy), string(objective), item.Source.SortOrder, string(appliedStatus), tenantID)
		if err != nil {
			return commercial.AppliedGroupImport{}, err
		}
		if tag.RowsAffected() != 1 {
			return commercial.AppliedGroupImport{}, domain.ErrNotFound
		}
	}

	bridgeBySurface, err := loadGroupSurfaceBridges(ctx, tx, groupID)
	if err != nil {
		return commercial.AppliedGroupImport{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ai_group_client_surfaces WHERE group_id = $1::uuid`, groupID); err != nil {
		return commercial.AppliedGroupImport{}, err
	}
	allowed := make(map[surface.ID]struct{}, len(item.Source.ClientSurfacePolicy.AllowedSurfaces))
	if item.Source.ClientSurfacePolicy.Mode == commercial.GroupClientSurfacePolicyAll {
		for _, id := range surface.Known() {
			allowed[id] = struct{}{}
		}
	} else {
		for _, id := range item.Source.ClientSurfacePolicy.AllowedSurfaces {
			allowed[id] = struct{}{}
		}
	}
	for _, id := range surface.Known() {
		status := commercial.StatusDisabled
		if _, ok := allowed[id]; ok {
			status = commercial.StatusActive
		}
		bridge, ok := bridgeBySurface[id]
		if !ok {
			bridge = true
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_group_client_surfaces (group_id, surface, bridge_enabled, status)
			VALUES ($1::uuid, $2, $3, $4)
		`, groupID, string(id), bridge, string(status)); err != nil {
			return commercial.AppliedGroupImport{}, err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM ai_group_model_dispatch_rules WHERE group_id = $1::uuid`, groupID); err != nil {
		return commercial.AppliedGroupImport{}, err
	}
	for _, rule := range item.Source.DispatchRules {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_group_model_dispatch_rules (
				group_id, client_surface, match_type, match_value, target_model_code,
				priority, status, notes
			) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)
		`, groupID, string(rule.ClientSurface), string(rule.MatchType), rule.MatchValue, rule.TargetModelID,
			rule.Priority, string(rule.Status), rule.Notes); err != nil {
			return commercial.AppliedGroupImport{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return commercial.AppliedGroupImport{}, err
	}
	return commercial.AppliedGroupImport{GroupID: groupID, Status: appliedStatus}, nil
}

func loadGroupConfigurationSnapshots(ctx context.Context, db groupTransferQuerier, tenantID string, groupIDs, groupNames []string) ([]commercial.GroupConfigurationSnapshot, error) {
	filter := "TRUE"
	args := []any{}
	if len(groupIDs) > 0 {
		filter = "g.id::text = ANY($2::text[])"
		args = append(args, tenantID, uniqueAdapterStrings(groupIDs))
	} else if len(groupNames) > 0 {
		filter = "g.name = ANY($2::text[])"
		args = append(args, tenantID, uniqueAdapterStrings(groupNames))
	} else {
		args = append(args, tenantID)
	}
	rows, err := db.Query(ctx, fmt.Sprintf(`
		SELECT g.id::text, g.name, g.description, g.retail_price_book_id::text,
		       g.default_user_multiplier, g.user_default_visible, g.allow_protocol_conversion,
		       g.route_strategy, g.route_objective,
		       g.sort_order, g.status,
		       (SELECT COUNT(*) FROM ai_group_targets gt WHERE gt.group_id = g.id AND gt.status = 'active')
		FROM ai_groups g
		WHERE g.tenant_id = $1 AND %s
		ORDER BY g.name ASC
	`, filter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]commercial.GroupConfigurationSnapshot, 0)
	ids := make([]string, 0)
	for rows.Next() {
		var item commercial.GroupConfigurationSnapshot
		var multiplier pgtype.Numeric
		var status string
		var activeTargets int64
		if err := rows.Scan(
			&item.GroupID, &item.Configuration.Name, &item.Configuration.Description, &item.PriceBookID,
			&multiplier, &item.Configuration.UserDefaultVisible, &item.Configuration.AllowProtocolConversion,
			&item.Configuration.RouteStrategy, &item.Configuration.RouteObjective,
			&item.Configuration.SortOrder, &status, &activeTargets,
		); err != nil {
			return nil, err
		}
		item.Configuration.DefaultUserMultiplier = numericToFloat(multiplier)
		item.Configuration.Status = commercial.Status(status)
		item.Configuration.ClientSurfacePolicy = commercial.GroupTransferClientSurfacePolicy{
			Mode:            commercial.GroupClientSurfacePolicyAll,
			AllowedSurfaces: []surface.ID{},
		}
		item.Configuration.DispatchRules = []commercial.GroupTransferDispatchRule{}
		item.ActiveTargets = int(activeTargets)
		items = append(items, item)
		ids = append(ids, item.GroupID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return items, nil
	}
	byID := make(map[string]*commercial.GroupConfigurationSnapshot, len(items))
	for index := range items {
		byID[items[index].GroupID] = &items[index]
	}
	if err := loadGroupTransferSurfaces(ctx, db, ids, byID); err != nil {
		return nil, err
	}
	if err := loadGroupTransferRules(ctx, db, ids, byID); err != nil {
		return nil, err
	}
	return items, nil
}

func loadGroupTransferSurfaces(ctx context.Context, db groupTransferQuerier, groupIDs []string, byID map[string]*commercial.GroupConfigurationSnapshot) error {
	rows, err := db.Query(ctx, `
		SELECT group_id::text, surface, status
		FROM ai_group_client_surfaces
		WHERE group_id::text = ANY($1::text[])
		ORDER BY group_id, surface
	`, groupIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	all := make(map[string][]surface.ID)
	active := make(map[string][]surface.ID)
	for rows.Next() {
		var groupID, rawSurface, status string
		if err := rows.Scan(&groupID, &rawSurface, &status); err != nil {
			return err
		}
		id := surface.ID(rawSurface)
		all[groupID] = append(all[groupID], id)
		if status == string(commercial.StatusActive) {
			active[groupID] = append(active[groupID], id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for groupID, snapshot := range byID {
		if len(all[groupID]) == 0 || len(active[groupID]) == len(surface.Known()) {
			snapshot.Configuration.ClientSurfacePolicy.Mode = commercial.GroupClientSurfacePolicyAll
			snapshot.Configuration.ClientSurfacePolicy.AllowedSurfaces = []surface.ID{}
			continue
		}
		snapshot.Configuration.ClientSurfacePolicy.Mode = commercial.GroupClientSurfacePolicyRestricted
		snapshot.Configuration.ClientSurfacePolicy.AllowedSurfaces = active[groupID]
	}
	return nil
}

func loadGroupTransferRules(ctx context.Context, db groupTransferQuerier, groupIDs []string, byID map[string]*commercial.GroupConfigurationSnapshot) error {
	rows, err := db.Query(ctx, `
		SELECT group_id::text, client_surface, match_type, match_value, target_model_code,
		       priority, status, notes
		FROM ai_group_model_dispatch_rules
		WHERE group_id::text = ANY($1::text[])
		ORDER BY group_id, priority ASC, created_at ASC, id ASC
	`, groupIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var groupID, clientSurface, matchType, status string
		var priority int32
		var rule commercial.GroupTransferDispatchRule
		if err := rows.Scan(&groupID, &clientSurface, &matchType, &rule.MatchValue, &rule.TargetModelID,
			&priority, &status, &rule.Notes); err != nil {
			return err
		}
		rule.ClientSurface = surface.ID(clientSurface)
		rule.MatchType = commercial.DispatchMatchType(matchType)
		rule.Priority = int(priority)
		rule.Status = commercial.Status(status)
		if snapshot := byID[groupID]; snapshot != nil {
			snapshot.Configuration.DispatchRules = append(snapshot.Configuration.DispatchRules, rule)
		}
	}
	return rows.Err()
}

func loadGroupImportPriceBooks(ctx context.Context, db groupTransferQuerier, tenantID string, priceBookIDs []string) (map[string]commercial.GroupImportPriceBook, error) {
	out := make(map[string]commercial.GroupImportPriceBook)
	if len(priceBookIDs) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `
		SELECT pb.id::text, pb.status, COALESCE(e.model_code, ''), COALESCE(e.capability_type, '')
		FROM ai_price_books pb
		LEFT JOIN ai_price_book_entries e ON e.price_book_id = pb.id
		WHERE pb.id::text = ANY($1::text[])
		  AND (pb.owner_type = 'platform' OR (pb.owner_type = 'tenant' AND pb.owner_tenant_id = $2))
		ORDER BY pb.id, e.model_code, e.capability_type
	`, priceBookIDs, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, modelCode, capability string
		if err := rows.Scan(&id, &status, &modelCode, &capability); err != nil {
			return nil, err
		}
		item, exists := out[id]
		if !exists {
			item = commercial.GroupImportPriceBook{ID: id, Status: commercial.Status(status), Models: make(map[string][]string)}
		}
		if modelCode != "" {
			item.Models[modelCode] = append(item.Models[modelCode], capability)
		}
		out[id] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func loadGroupSurfaceBridges(ctx context.Context, db groupTransferQuerier, groupID string) (map[surface.ID]bool, error) {
	rows, err := db.Query(ctx, `
		SELECT surface, bridge_enabled
		FROM ai_group_client_surfaces
		WHERE group_id = $1::uuid
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[surface.ID]bool)
	for rows.Next() {
		var id string
		var enabled bool
		if err := rows.Scan(&id, &enabled); err != nil {
			return nil, err
		}
		out[surface.ID(id)] = enabled
	}
	return out, rows.Err()
}

func uniqueAdapterStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

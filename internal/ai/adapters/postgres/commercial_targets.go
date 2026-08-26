package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	commercial "xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/domain"
)

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

package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	commercial "xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/surface"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

// GroupRepo is the legacy-backed postgres adapter for commercial group storage.
type GroupRepo struct {
	q    *dbgen.Queries
	pool *pgxpool.Pool
}

func NewGroupRepo(q *dbgen.Queries, pool *pgxpool.Pool) *GroupRepo {
	return &GroupRepo{q: q, pool: pool}
}

func (r *GroupRepo) PriceBookExists(ctx context.Context, id string) (bool, error) {
	uid, err := akUUID(id)
	if err != nil {
		return false, err
	}
	return existsByID(ctx, r.pool, "ai_price_books", uid)
}

func (r *GroupRepo) GroupExists(ctx context.Context, id string) (bool, error) {
	uid, err := akUUID(id)
	if err != nil {
		return false, err
	}
	return existsByID(ctx, r.pool, "ai_groups", uid)
}

func (r *GroupRepo) AccountExists(ctx context.Context, id string) (bool, error) {
	uid, err := akUUID(id)
	if err != nil {
		return false, err
	}
	return existsByID(ctx, r.pool, "ai_upstream_accounts", uid)
}

func (r *GroupRepo) CredentialPoolExists(ctx context.Context, id string) (bool, error) {
	uid, err := akUUID(id)
	if err != nil {
		return false, err
	}
	return existsByID(ctx, r.pool, "ai_credential_pools", uid)
}

func (r *GroupRepo) CountGroupReferences(ctx context.Context, id string) (int, error) {
	uid, err := akUUID(id)
	if err != nil {
		return 0, err
	}
	return countOne(ctx, r.pool, `
		SELECT
			(SELECT COUNT(*) FROM ai_group_targets WHERE group_id = $1) +
			(SELECT COUNT(*) FROM ai_group_model_dispatch_rules WHERE group_id = $1) +
			(SELECT COUNT(*) FROM ai_user_groups WHERE group_id = $1) +
			(SELECT COUNT(*) FROM ai_api_keys WHERE group_id = $1) +
			(SELECT COUNT(*) FROM ai_sub_plan_groups WHERE group_id = $1)
	`, uid)
}

// ---- Group ----

func groupFromRow(r dbgen.AiGroup) domain.Group {
	return domain.Group{
		ID:                      uuidToString(r.ID),
		TenantID:                r.TenantID,
		Name:                    r.Name,
		Description:             r.Description,
		RetailPriceBookID:       uuidToString(r.RetailPriceBookID),
		DefaultUserMultiplier:   numericToFloat(r.DefaultUserMultiplier),
		UserDefaultVisible:      r.UserDefaultVisible,
		AllowProtocolConversion: r.AllowProtocolConversion,
		SortOrder:               r.SortOrder,
		Status:                  r.Status,
		CreatedAt:               r.CreatedAt.Time,
		UpdatedAt:               r.UpdatedAt.Time,
	}
}

func (r *GroupRepo) ListGroups(ctx context.Context, tenantID string) ([]domain.GroupListItem, error) {
	rows, err := r.q.ListGroups(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.GroupListItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.GroupListItem{
			Group: domain.Group{
				ID:                      uuidToString(row.ID),
				TenantID:                row.TenantID,
				Name:                    row.Name,
				Description:             row.Description,
				RetailPriceBookID:       uuidToString(row.RetailPriceBookID),
				DefaultUserMultiplier:   numericToFloat(row.DefaultUserMultiplier),
				UserDefaultVisible:      row.UserDefaultVisible,
				AllowProtocolConversion: row.AllowProtocolConversion,
				SortOrder:               row.SortOrder,
				Status:                  row.Status,
				CreatedAt:               row.CreatedAt.Time,
				UpdatedAt:               row.UpdatedAt.Time,
			},
			RetailPriceBookName: row.RetailPriceBookName,
		})
	}
	return out, nil
}

func (r *GroupRepo) GetGroup(ctx context.Context, tenantID, id string) (domain.Group, error) {
	gid, err := akUUID(id)
	if err != nil {
		return domain.Group{}, err
	}
	row, err := r.q.GetGroup(ctx, dbgen.GetGroupParams{TenantID: tenantID, ID: gid})
	if err != nil {
		return domain.Group{}, err
	}
	return groupFromRow(row), nil
}

func (r *GroupRepo) UpdateGroup(ctx context.Context, tenantID, id string, w commercial.GroupWrite) (domain.Group, error) {
	gid, err := akUUID(id)
	if err != nil {
		return domain.Group{}, err
	}
	row, err := r.q.UpdateGroup(ctx, dbgen.UpdateGroupParams{
		ID:                      gid,
		Name:                    w.Name,
		Description:             w.Description,
		RetailPriceBookID:       nullableUUID(w.RetailPriceBookID),
		DefaultUserMultiplier:   floatToNumeric(w.DefaultUserMultiplier),
		UserDefaultVisible:      w.UserDefaultVisible,
		AllowProtocolConversion: w.AllowProtocolConversion,
		SortOrder:               int32(w.SortOrder),
		Status:                  string(w.Status),
		TenantID:                tenantID,
	})
	if err != nil {
		return domain.Group{}, err
	}
	return groupFromRow(row), nil
}

func (r *GroupRepo) UpdateGroupStatus(ctx context.Context, tenantID, id, status string) (domain.Group, error) {
	gid, err := akUUID(id)
	if err != nil {
		return domain.Group{}, err
	}
	row, err := r.q.UpdateGroupStatus(ctx, dbgen.UpdateGroupStatusParams{ID: gid, Status: status, TenantID: tenantID})
	if err != nil {
		return domain.Group{}, err
	}
	return groupFromRow(row), nil
}

func (r *GroupRepo) DeleteGroup(ctx context.Context, tenantID, id string) error {
	gid, err := akUUID(id)
	if err != nil {
		return err
	}

	refs, err := r.CountGroupReferences(ctx, id)
	if err != nil {
		return err
	}
	if refs > 0 {
		return domain.ErrConflict
	}
	return r.q.DeleteGroup(ctx, dbgen.DeleteGroupParams{TenantID: tenantID, ID: gid})
}

func groupDispatchRuleFromRow(r dbgen.AiGroupModelDispatchRule) domain.GroupModelDispatchRule {
	return domain.GroupModelDispatchRule{
		ID:              uuidToString(r.ID),
		GroupID:         uuidToString(r.GroupID),
		ClientSurface:   r.ClientSurface,
		MatchType:       r.MatchType,
		MatchValue:      r.MatchValue,
		TargetModelCode: r.TargetModelCode,
		Priority:        r.Priority,
		Status:          r.Status,
		Notes:           r.Notes,
		CreatedAt:       r.CreatedAt.Time,
		UpdatedAt:       r.UpdatedAt.Time,
	}
}

func (r *GroupRepo) ListGroupDispatchRules(ctx context.Context, groupID string) ([]domain.GroupModelDispatchRule, error) {
	gid, err := akUUID(groupID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListGroupDispatchRules(ctx, gid)
	if err != nil {
		return nil, err
	}
	out := make([]domain.GroupModelDispatchRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, groupDispatchRuleFromRow(row))
	}
	return out, nil
}

// ---- Group targets (分组 → 上游目标：账号或凭证池) ----

func groupTargetBindingFromRow(id, groupID, targetKind, targetID string, priority int32, status string, createdAt, updatedAt time.Time) domain.GroupTargetBinding {
	return domain.GroupTargetBinding{
		ID:         id,
		GroupID:    groupID,
		TargetKind: targetKind,
		TargetID:   targetID,
		Priority:   priority,
		Status:     status,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
}

func (r *GroupRepo) ListGroupTargets(ctx context.Context, tenantID, groupID string) ([]domain.GroupTargetBinding, error) {
	gid, err := akUUID(groupID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListGroupTargets(ctx, dbgen.ListGroupTargetsParams{TenantID: tenantID, GroupID: gid})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GroupTargetBinding, 0, len(rows))
	for _, row := range rows {
		out = append(out, groupTargetBindingFromRow(uuidToString(row.ID), uuidToString(row.GroupID), row.TargetKind, uuidToString(row.TargetID), row.Priority, row.Status, row.CreatedAt.Time, row.UpdatedAt.Time))
	}
	return out, nil
}

// computeTargetAvailability 复刻运行时 RuntimeBindingAuthorizer 与 AddGroupTarget 的
// 可用性口径：资源必须 active，且 public 或已 access_granted。resource 行缺失（已删）
// 时 targetStatus 为空 → missing。任何 false 都会在请求时被 fail-closed 拒。
func computeTargetAvailability(targetStatus, accessMode string, accessGranted bool) (bool, string) {
	switch {
	case targetStatus == "":
		return false, "missing"
	case targetStatus != "active":
		return false, "inactive"
	case accessMode != "public" && !accessGranted:
		return false, "access_revoked"
	default:
		return true, ""
	}
}

func (r *GroupRepo) ListGroupTargetDetails(ctx context.Context, tenantID, groupID string) ([]domain.GroupTargetDetail, error) {
	gid, err := akUUID(groupID)
	if err != nil {
		return nil, err
	}
	// 手写查询（不复用 sqlc ListGroupTargets）：额外拉资源当前 status/access_mode 与
	// 本租户 access_granted，据此算「绑定是否仍对租户可用」。租户仍由 EXISTS(ai_groups)
	// 兜（与其余 target 读写一致）。
	rows, err := r.pool.Query(ctx, `
		SELECT
			gt.id::text,
			gt.group_id::text,
			gt.target_kind,
			gt.target_id::text,
			gt.priority,
			gt.status,
			gt.created_at,
			gt.updated_at,
			COALESCE(a.tenant_display_name, '')  AS account_name,
			COALESCE(a.default_protocol, '')     AS default_protocol,
			COALESCE(cp.tenant_display_name, '') AS pool_name,
			COALESCE(cp.fixed_provider_type, '') AS fixed_provider_type,
			COALESCE(a.status, cp.status, '')                          AS target_status,
			COALESCE(a.tenant_access_mode, cp.tenant_access_mode, '')  AS access_mode,
			COALESCE(tp.access_granted, false)                         AS access_granted
		FROM ai_group_targets gt
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		LEFT JOIN ai_upstream_resource_tenant_policies tp
		  ON tp.resource_kind = gt.target_kind AND tp.resource_id = gt.target_id AND tp.tenant_id = $1
		WHERE gt.group_id = $2
		  AND EXISTS (SELECT 1 FROM ai_groups g WHERE g.id = gt.group_id AND g.tenant_id = $1)
		ORDER BY gt.priority ASC, account_name ASC, pool_name ASC
	`, tenantID, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.GroupTargetDetail, 0)
	for rows.Next() {
		var (
			id, groupIDText, tKind, tID                           string
			status                                                string
			priority                                              int32
			createdAt, updatedAt                                  time.Time
			accountName, defaultProtocol, poolName, fixedProvider string
			targetStatus, accessMode                              string
			accessGranted                                         bool
		)
		if err := rows.Scan(
			&id, &groupIDText, &tKind, &tID, &priority, &status, &createdAt, &updatedAt,
			&accountName, &defaultProtocol, &poolName, &fixedProvider,
			&targetStatus, &accessMode, &accessGranted,
		); err != nil {
			return nil, err
		}
		available, reason := computeTargetAvailability(targetStatus, accessMode, accessGranted)
		out = append(out, domain.GroupTargetDetail{
			GroupTargetBinding: groupTargetBindingFromRow(id, groupIDText, tKind, tID, priority, status, createdAt, updatedAt),
			AccountName:        accountName,
			DefaultProtocol:    defaultProtocol,
			PoolName:           poolName,
			FixedProviderType:  fixedProvider,
			Available:          available,
			UnavailableReason:  reason,
		})
	}
	return out, rows.Err()
}

// ListGroupTargetsByTarget 反查：按 target_kind + target_id 查出该上游目标被哪些分组关联了。
func (r *GroupRepo) ListGroupTargetsByTarget(ctx context.Context, targetKind, targetID string) ([]domain.GroupTargetDetail, error) {
	tid, err := akUUID(targetID)
	if err != nil {
		return nil, domain.NewValidationError("target_id", "invalid target_id")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			gt.id::text,
			gt.group_id::text,
			gt.target_kind,
			gt.target_id::text,
			gt.priority,
			gt.status,
			gt.created_at,
			gt.updated_at,
			COALESCE(a.tenant_display_name, '')  AS account_name,
			COALESCE(a.default_protocol, '')     AS default_protocol,
			COALESCE(cp.tenant_display_name, '') AS pool_name,
			COALESCE(cp.fixed_provider_type, '') AS fixed_provider_type
		FROM ai_group_targets gt
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		WHERE gt.target_kind = $1 AND gt.target_id = $2
		ORDER BY gt.priority ASC, account_name ASC, pool_name ASC
	`, targetKind, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.GroupTargetDetail, 0)
	for rows.Next() {
		var (
			id, groupID, tKind, tID                               string
			status                                                string
			priority                                              int32
			createdAt, updatedAt                                  time.Time
			accountName, defaultProtocol, poolName, fixedProvider string
		)
		if err := rows.Scan(
			&id, &groupID, &tKind, &tID, &priority, &status, &createdAt, &updatedAt,
			&accountName, &defaultProtocol, &poolName, &fixedProvider,
		); err != nil {
			return nil, err
		}
		item := domain.GroupTargetDetail{
			GroupTargetBinding: groupTargetBindingFromRow(id, groupID, tKind, tID, priority, status, createdAt, updatedAt),
			AccountName:        accountName,
			DefaultProtocol:    defaultProtocol,
			PoolName:           poolName,
			FixedProviderType:  fixedProvider,
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *GroupRepo) GetGroupTargetDetail(ctx context.Context, tenantID, groupID, id string) (domain.GroupTargetDetail, error) {
	gid, err := akUUID(groupID)
	if err != nil {
		return domain.GroupTargetDetail{}, err
	}
	rid, err := akUUID(id)
	if err != nil {
		return domain.GroupTargetDetail{}, err
	}
	var (
		targetID, targetGroupID, targetKind                       string
		status                                                    string
		priority                                                  int32
		createdAt, updatedAt                                      time.Time
		accountName, defaultProtocol, poolName, fixedProviderType string
		targetStatus, accessMode                                  string
		accessGranted                                             bool
	)
	if err := r.pool.QueryRow(ctx, `
		SELECT
			gt.target_id::text,
			gt.group_id::text,
			gt.target_kind,
			gt.status,
			gt.priority,
			gt.created_at,
			gt.updated_at,
			COALESCE(a.tenant_display_name, '')  AS account_name,
			COALESCE(a.default_protocol, '')     AS default_protocol,
			COALESCE(cp.tenant_display_name, '') AS pool_name,
			COALESCE(cp.fixed_provider_type, '') AS fixed_provider_type,
			COALESCE(a.status, cp.status, '')                          AS target_status,
			COALESCE(a.tenant_access_mode, cp.tenant_access_mode, '')  AS access_mode,
			COALESCE(tp.access_granted, false)                         AS access_granted
		FROM ai_group_targets gt
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		LEFT JOIN ai_upstream_resource_tenant_policies tp
		  ON tp.resource_kind = gt.target_kind AND tp.resource_id = gt.target_id AND tp.tenant_id = $3
		JOIN ai_groups g ON g.id = gt.group_id AND g.tenant_id = $3
		WHERE gt.group_id = $1 AND gt.id = $2
	`, gid, rid, tenantID).Scan(
		&targetID, &targetGroupID, &targetKind, &status, &priority, &createdAt, &updatedAt,
		&accountName, &defaultProtocol, &poolName, &fixedProviderType,
		&targetStatus, &accessMode, &accessGranted,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.GroupTargetDetail{}, domain.ErrNotFound
		}
		return domain.GroupTargetDetail{}, err
	}
	available, reason := computeTargetAvailability(targetStatus, accessMode, accessGranted)
	return domain.GroupTargetDetail{
		GroupTargetBinding: groupTargetBindingFromRow(id, targetGroupID, targetKind, targetID, priority, status, createdAt, updatedAt),
		AccountName:        accountName,
		DefaultProtocol:    defaultProtocol,
		PoolName:           poolName,
		FixedProviderType:  fixedProviderType,
		Available:          available,
		UnavailableReason:  reason,
	}, nil
}

func (r *GroupRepo) UpdateGroupTarget(ctx context.Context, tenantID, id string, w commercial.GroupTargetWrite) (domain.GroupTargetBinding, error) {
	rid, err := akUUID(id)
	if err != nil {
		return domain.GroupTargetBinding{}, err
	}
	row, err := r.q.UpdateGroupTarget(ctx, dbgen.UpdateGroupTargetParams{
		ID: rid, Priority: int32(w.Priority), Status: string(w.Status), TenantID: tenantID,
	})
	if err != nil {
		return domain.GroupTargetBinding{}, err
	}
	return groupTargetBindingFromRow(uuidToString(row.ID), uuidToString(row.GroupID), row.TargetKind, uuidToString(row.TargetID), row.Priority, row.Status, row.CreatedAt.Time, row.UpdatedAt.Time), nil
}

func (r *GroupRepo) DeleteGroupTarget(ctx context.Context, tenantID, id string) error {
	rid, err := akUUID(id)
	if err != nil {
		return err
	}
	return r.q.DeleteGroupTarget(ctx, dbgen.DeleteGroupTargetParams{ID: rid, TenantID: tenantID})
}

func (r *GroupRepo) ListVisibleGroupsForTenant(ctx context.Context, tenantID string) ([]domain.VisibleGroup, error) {
	rows, err := r.q.ListVisibleGroupsForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.VisibleGroup, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.VisibleGroup{
			Group: domain.Group{
				ID:                    uuidToString(row.ID),
				TenantID:              row.TenantID,
				Name:                  row.Name,
				Description:           row.Description,
				RetailPriceBookID:     uuidToString(row.RetailPriceBookID),
				DefaultUserMultiplier: numericToFloat(row.DefaultUserMultiplier),
				UserDefaultVisible:    row.UserDefaultVisible,
				SortOrder:             row.SortOrder,
				Status:                row.Status,
				CreatedAt:             row.CreatedAt.Time,
				UpdatedAt:             row.UpdatedAt.Time,
			},
			EffectiveUserMultiplier: numericToFloat(row.EffectiveUserMultiplier),
		})
	}
	return out, nil
}

// ---- User bindings ----

func (r *GroupRepo) UpsertUserGroup(ctx context.Context, w commercial.UserGroupBindingWrite) (domain.UserGroup, error) {
	gid, err := akUUID(w.GroupID)
	if err != nil {
		return domain.UserGroup{}, domain.NewValidationError("group_id", "invalid group_id")
	}
	row, err := r.q.UpsertUserGroup(ctx, dbgen.UpsertUserGroupParams{
		TenantID:               w.TenantID,
		UserID:                 w.UserID,
		GroupID:                gid,
		UserMultiplierOverride: floatPtrToNumeric(w.UserMultiplierOverride),
		CreatedBy:              nullableText(firstNonEmpty(w.UpdatedBy, w.CreatedBy)),
	})
	if err != nil {
		return domain.UserGroup{}, err
	}
	return domain.UserGroup{
		ID:                     uuidToString(row.ID),
		TenantID:               row.TenantID,
		UserID:                 row.UserID,
		GroupID:                uuidToString(row.GroupID),
		UserMultiplierOverride: numericToFloatPtr(row.UserMultiplierOverride),
		CreatedBy:              row.CreatedBy.String,
		CreatedAt:              row.CreatedAt.Time,
		UpdatedAt:              row.UpdatedAt.Time,
	}, nil
}

func commercialSurfaceID(raw string) surface.ID {
	return surface.ID(strings.TrimSpace(raw))
}

func (r *GroupRepo) ListUserGroups(ctx context.Context, tenantID, userID string) ([]domain.UserGroup, error) {
	rows, err := r.q.ListUserGroups(ctx, dbgen.ListUserGroupsParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, err
	}
	out := make([]domain.UserGroup, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.UserGroup{
			ID:                         uuidToString(row.ID),
			TenantID:                   row.TenantID,
			UserID:                     row.UserID,
			GroupID:                    uuidToString(row.GroupID),
			UserMultiplierOverride:     numericToFloatPtr(row.UserMultiplierOverride),
			CreatedBy:                  row.CreatedBy.String,
			CreatedAt:                  row.CreatedAt.Time,
			UpdatedAt:                  row.UpdatedAt.Time,
			GroupName:                  row.GroupName,
			GroupDefaultUserMultiplier: numericToFloat(row.GroupDefaultUserMultiplier),
		})
	}
	return out, nil
}

func (r *GroupRepo) DeleteUserGroup(ctx context.Context, tenantID, userID, groupID string) error {
	gid, err := akUUID(groupID)
	if err != nil {
		return err
	}
	return r.q.DeleteUserGroup(ctx, dbgen.DeleteUserGroupParams{TenantID: tenantID, UserID: userID, GroupID: gid})
}

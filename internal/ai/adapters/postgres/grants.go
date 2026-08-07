package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
)

// GroupAccessReader resolves tenant-owned group visibility for console and
// workspace catalog views. Live request authorization is performed by the
// shared runtime resolver, which also verifies model price and target binding.
//
//	租户可见 = 本租户的 active 分组
//	用户可见 = 分组默认开放 ∪ ai_user_groups 显式例外
//	key 绑定 = 若 Subject.GroupID 非空则只保留该分组
type GroupAccessReader struct {
	pool *pgxpool.Pool
}

func NewGroupAccessReader(pool *pgxpool.Pool) *GroupAccessReader {
	return &GroupAccessReader{pool: pool}
}

func (c *GroupAccessReader) AccessibleGroupIDsForSubject(ctx context.Context, subject *coreidentity.Subject) ([]string, error) {
	if subject == nil {
		return nil, fmt.Errorf("no runtime subject")
	}
	// 0. Explicit group selection still respects tenant ownership and user grants.
	if subject.ForcedGroupID != "" {
		if subject.UserID != "" {
			return c.queryStringList(ctx, `
				SELECT g.id::text
				FROM ai_groups g
				LEFT JOIN ai_user_groups ug
				  ON ug.group_id = g.id AND ug.tenant_id = $2 AND ug.user_id = $3
				WHERE g.id = $1::uuid AND g.tenant_id = $2 AND g.status = 'active'
				  AND (g.user_default_visible OR ug.id IS NOT NULL)
			`, subject.ForcedGroupID, subject.TenantID, subject.UserID)
		}
		return c.queryStringList(ctx, `
			SELECT g.id::text
			FROM ai_groups g
				WHERE g.id = $1::uuid AND g.tenant_id = $2 AND g.status = 'active'
			`, subject.ForcedGroupID, subject.TenantID)
	}
	// 1. 租户拥有的分组。终端用户只继承默认开放的分组，也可通过
	//    ai_user_groups 单独打开例外分组。
	const tenantVisibleQ = `
			SELECT g.id::text
			FROM ai_groups g
			WHERE g.tenant_id = $1 AND g.status = 'active'
			ORDER BY g.name ASC`
	visibleQ := tenantVisibleQ
	args := []any{subject.TenantID}
	if subject.UserID != "" {
		visibleQ = `
				SELECT g.id::text
				FROM ai_groups g
				LEFT JOIN ai_user_groups ug
				  ON ug.group_id = g.id AND ug.tenant_id = $1 AND ug.user_id = $2
				WHERE g.tenant_id = $1
				  AND g.status = 'active'
				  AND (g.user_default_visible OR ug.id IS NOT NULL)
			ORDER BY g.name ASC`
		args = []any{subject.TenantID, subject.UserID}
	}
	visible, err := c.queryStringList(ctx, visibleQ, args...)
	if err != nil {
		return nil, fmt.Errorf("list visible groups: %w", err)
	}

	// 2. API key 或会话显式选定分组时，只保留该分组。
	if subject.GroupID != "" {
		for _, groupID := range visible {
			if groupID == subject.GroupID {
				return []string{groupID}, nil
			}
		}
		return []string{}, nil
	}
	return visible, nil
}

// ---- helpers ----

func (c *GroupAccessReader) queryStringList(ctx context.Context, q string, args ...any) ([]string, error) {
	rows, err := c.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

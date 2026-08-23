package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	userports "xiaodou/dai/internal/user/ports"
)

// AdminEndUserRepository owns the scoped admin end-user list projection.
// Mutations remain separate until their transaction boundary is migrated.
type AdminEndUserRepository struct {
	pool *pgxpool.Pool
}

func NewAdminEndUserRepository(pool *pgxpool.Pool) *AdminEndUserRepository {
	return &AdminEndUserRepository{pool: pool}
}

// UpdateEndUser updates only fields explicitly selected by the caller. The
// tenant and user-type predicates keep the write scoped even if a prior
// ownership check races with a delete or reassignment attempt.
func (r *AdminEndUserRepository) UpdateEndUser(ctx context.Context, input userports.AdminEndUserUpdate) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts
		SET email = CASE WHEN $1 THEN NULLIF($2, '') ELSE email END,
		    phone = CASE WHEN $3 THEN NULLIF($4, '') ELSE phone END,
		    internal_note = CASE WHEN $5 THEN $6 ELSE internal_note END,
		    updated_at = $7
		WHERE user_id = $8 AND tenant_id = $9 AND user_type = 4 AND status <> 'deleted'
	`, input.EmailSet, input.Email, input.PhoneSet, input.Phone, input.InternalNoteSet, input.InternalNote, time.Now().UTC(), input.UserID, input.TenantID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// UpdateEndUserStatus changes an active end-user account state. Deleted
// accounts are intentionally excluded so a stale request cannot resurrect
// one that has already been removed from operational views.
func (r *AdminEndUserRepository) UpdateEndUserStatus(ctx context.Context, userID, status string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts
		SET status = $1, updated_at = $2
		WHERE user_id = $3 AND user_type = 4 AND status <> 'deleted'
	`, status, time.Now().UTC(), userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *AdminEndUserRepository) ListEndUsers(ctx context.Context, filter userports.AdminEndUserListFilter) (userports.AdminEndUserPage, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	size := filter.Size
	if size < 1 || size > 100 {
		size = 20
	}

	where := "WHERE eu.user_type = 4 AND eu.status <> 'deleted'"
	args := []any{}
	idx := 1
	if filter.TenantID != "" {
		where += fmt.Sprintf(" AND eu.tenant_id = $%d", idx)
		args = append(args, filter.TenantID)
		idx++
	}
	if filter.Keyword != "" {
		where += fmt.Sprintf(" AND (eu.username LIKE $%d OR eu.email LIKE $%d OR eu.phone LIKE $%d OR eu.internal_note LIKE $%d)", idx, idx, idx, idx)
		args = append(args, "%"+filter.Keyword+"%")
		idx++
	}
	if filter.TenantName != "" {
		where += fmt.Sprintf(" AND t.tenant_name LIKE $%d", idx)
		args = append(args, "%"+filter.TenantName+"%")
		idx++
	}
	if filter.Username != "" {
		where += fmt.Sprintf(" AND eu.username LIKE $%d", idx)
		args = append(args, "%"+filter.Username+"%")
		idx++
	}
	if filter.Status != "" {
		where += fmt.Sprintf(" AND eu.status = $%d", idx)
		args = append(args, filter.Status)
		idx++
	}

	from := "FROM iam_accounts eu LEFT JOIN iam_tenants t ON eu.tenant_id = t.tenant_id"
	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) "+from+" "+where, args...).Scan(&total); err != nil {
		return userports.AdminEndUserPage{}, err
	}

	offset := (page - 1) * size
	queryArgs := append(append([]any{}, args...), size, offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT eu.user_id, eu.tenant_id, eu.username, eu.email, eu.phone, eu.internal_note, eu.nickname, eu.avatar,
		       eu.status, eu.credential_state, eu.last_login_at, eu.created_at,
		       COALESCE(t.tenant_name, '') AS tenant_name,
		       COALESCE((SELECT b.balance_micro FROM bill_accounts b WHERE b.account_id = eu.user_id), 0) AS credits
		%s
		%s ORDER BY eu.created_at DESC LIMIT $%d OFFSET $%d
	`, from, where, idx, idx+1), queryArgs...)
	if err != nil {
		return userports.AdminEndUserPage{}, err
	}
	defer rows.Close()

	items := make([]userports.AdminEndUserRow, 0)
	for rows.Next() {
		var item userports.AdminEndUserRow
		if err := rows.Scan(
			&item.UserID, &item.TenantID, &item.Username, &item.Email, &item.Phone, &item.InternalNote,
			&item.Nickname, &item.Avatar, &item.Status, &item.CredentialState, &item.LastLoginAt,
			&item.CreatedAt, &item.TenantName, &item.BalanceMicroUSD,
		); err != nil {
			return userports.AdminEndUserPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return userports.AdminEndUserPage{}, err
	}
	return userports.AdminEndUserPage{Records: items, Total: total, Page: page, Size: size}, nil
}

package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	userports "xiaodou/dai/internal/user/ports"
)

// AdminAccountRepository owns paginated management queries for userType=2/3
// accounts. Mutations and activation workflows remain separate until their
// transaction boundary is migrated.
type AdminAccountRepository struct {
	pool *pgxpool.Pool
}

func NewAdminAccountRepository(pool *pgxpool.Pool) *AdminAccountRepository {
	return &AdminAccountRepository{pool: pool}
}

func (r *AdminAccountRepository) ListSystemAdmins(ctx context.Context, keyword string, page, size int) (userports.AdminAccountPage, error) {
	return r.list(ctx, 2, "", keyword, page, size)
}

func (r *AdminAccountRepository) ListTenantUsers(ctx context.Context, tenantID, keyword string, page, size int) (userports.AdminAccountPage, error) {
	return r.list(ctx, 3, tenantID, keyword, page, size)
}

func (r *AdminAccountRepository) list(ctx context.Context, userType int, tenantID, keyword string, page, size int) (userports.AdminAccountPage, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}

	where := "WHERE user_type = $1"
	args := []any{userType}
	idx := 2
	if userType == 3 {
		where += fmt.Sprintf(" AND tenant_id = $%d", idx)
		args = append(args, tenantID)
		idx++
	}
	if keyword != "" {
		operator := "ILIKE"
		if userType == 2 {
			operator = "LIKE"
		}
		where += fmt.Sprintf(" AND (username %s $%d OR email %s $%d)", operator, idx, operator, idx)
		args = append(args, "%"+keyword+"%")
		idx++
	}

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM iam_accounts "+where, args...).Scan(&total); err != nil {
		return userports.AdminAccountPage{}, err
	}

	offset := (page - 1) * size
	queryArgs := append(append([]any{}, args...), size, offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT user_id, username, email, status, credential_state, created_at
		FROM iam_accounts %s
		ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), queryArgs...)
	if err != nil {
		return userports.AdminAccountPage{}, err
	}
	defer rows.Close()

	items := make([]userports.AdminAccountRow, 0)
	for rows.Next() {
		var item userports.AdminAccountRow
		if err := rows.Scan(&item.UserID, &item.Username, &item.Email, &item.Status, &item.CredentialState, &item.CreatedAt); err != nil {
			return userports.AdminAccountPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return userports.AdminAccountPage{}, err
	}
	return userports.AdminAccountPage{Records: items, Total: total, Page: page, Size: size}, nil
}

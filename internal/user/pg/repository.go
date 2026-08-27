package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pagination struct {
	Page   int64 `json:"page"`
	Size   int64 `json:"size"`
	Offset int64 `json:"-"`
}

func NewPagination(page, size int64) Pagination {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	return Pagination{
		Page:   page,
		Size:   size,
		Offset: (page - 1) * size,
	}
}

type PaginatedResult[T any] struct {
	Records []T   `json:"records"`
	Total   int64 `json:"total"`
	Page    int64 `json:"page"`
	Size    int64 `json:"size"`
}

type User struct {
	UserID      string  `json:"userId"`
	TenantID    string  `json:"tenantId"`
	Username    string  `json:"username"`
	Email       *string `json:"email"`
	Nickname    *string `json:"nickname"`
	Avatar      *string `json:"avatar"`
	Status      int32   `json:"status"`
	CreatedTime *int64  `json:"createdTime"`
}

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) GetByUserID(ctx context.Context, userID string) (*User, error) {
	var u User
	var status string
	var createdAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, tenant_id, username, email, nickname, avatar, status, created_at
		FROM iam_accounts WHERE user_id = $1 AND user_type = 4
	`, userID).Scan(&u.UserID, &u.TenantID, &u.Username, &u.Email, &u.Nickname, &u.Avatar, &status, &createdAt)
	if err != nil {
		return nil, err
	}
	u.Status = int32(endUserStatusToInt(status))
	createdMillis := createdAt.UnixMilli()
	u.CreatedTime = &createdMillis
	return &u, nil
}

func (r *UserRepository) GetByUserIDs(ctx context.Context, userIDs []string) ([]*User, error) {
	if len(userIDs) == 0 {
		return []*User{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT user_id, tenant_id, username, email, nickname, avatar, status, created_at
		FROM iam_accounts WHERE user_id = ANY($1) AND user_type = 4
	`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var status string
		var createdAt time.Time
		if err := rows.Scan(&u.UserID, &u.TenantID, &u.Username, &u.Email, &u.Nickname, &u.Avatar, &status, &createdAt); err != nil {
			return nil, err
		}
		u.Status = int32(endUserStatusToInt(status))
		createdMillis := createdAt.UnixMilli()
		u.CreatedTime = &createdMillis
		users = append(users, &u)
	}
	return users, nil
}

func (r *UserRepository) Update(ctx context.Context, userID string, data map[string]any) error {
	if len(data) == 0 {
		return nil
	}

	sql := "UPDATE iam_accounts SET "
	args := []any{}
	i := 1
	for key, value := range data {
		if i > 1 {
			sql += ", "
		}
		sql += fmt.Sprintf("%s = $%d", pgx.Identifier{key}.Sanitize(), i)
		args = append(args, value)
		i++
	}
	sql += fmt.Sprintf(" WHERE user_id = $%d AND user_type = 4", i)
	args = append(args, userID)

	_, err := r.pool.Exec(ctx, sql, args...)
	return err
}

func endUserStatusToInt(status string) int {
	switch status {
	case "disabled":
		return 2
	case "locked":
		return 3
	case "inherited_disabled":
		return 4
	default:
		return 1
	}
}

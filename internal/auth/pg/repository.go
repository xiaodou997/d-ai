package pg

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthRepository struct {
	pool *pgxpool.Pool
}

func NewAuthRepository(pool *pgxpool.Pool) *AuthRepository {
	return &AuthRepository{pool: pool}
}

type SystemUserForLogin struct {
	UserID       string
	Username     string
	PasswordHash string
	UserType     int64
	Status       string
	Email        *string
}

type TenantUserForLogin struct {
	UserID       string
	TenantID     string
	Username     string
	PasswordHash string
	Status       string
	Email        *string
}

type EndUserForLogin struct {
	UserID        string
	TenantID      string
	Username      string
	PasswordHash  string
	Status        string
	FrozenCredits int64
	Email         *string
}

// PortalUserForLogin is the account record resolved by the unified Portal.
// The query intentionally searches all account tables so the browser never
// needs to guess a client type before credentials are known.
type PortalUserForLogin struct {
	UserID       string
	TenantID     string
	Username     string
	PasswordHash string
	UserType     int
	Status       string
}

func (r *AuthRepository) GetSystemUserForLogin(ctx context.Context, username string) (SystemUserForLogin, error) {
	var u SystemUserForLogin
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, username, password_hash, user_type, status, email
		FROM iam_admins WHERE username = $1
	`, username).Scan(&u.UserID, &u.Username, &u.PasswordHash, &u.UserType, &u.Status, &u.Email)
	return u, err
}

func (r *AuthRepository) UpdateSystemUserLoginTime(ctx context.Context, userID string, loginTime time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE iam_admins SET last_login_at = $1 WHERE user_id = $2
	`, loginTime, userID)
	return err
}

func (r *AuthRepository) GetTenantUserForLogin(ctx context.Context, username string) (TenantUserForLogin, error) {
	var u TenantUserForLogin
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, tenant_id, username, password_hash, status, email
		FROM iam_tenant_users WHERE username = $1
	`, username).Scan(&u.UserID, &u.TenantID, &u.Username, &u.PasswordHash, &u.Status, &u.Email)
	return u, err
}

func (r *AuthRepository) UpdateTenantUserLoginTime(ctx context.Context, userID string, loginTime time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE iam_tenant_users SET last_login_at = $1 WHERE user_id = $2
	`, loginTime, userID)
	return err
}

func (r *AuthRepository) GetEndUserForLogin(ctx context.Context, username string) (EndUserForLogin, error) {
	var u EndUserForLogin
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, tenant_id, username, password_hash, status, frozen_credits, email
		FROM iam_users WHERE username = $1
	`, username).Scan(&u.UserID, &u.TenantID, &u.Username, &u.PasswordHash, &u.Status, &u.FrozenCredits, &u.Email)
	return u, err
}

func (r *AuthRepository) GetPortalUserForLogin(ctx context.Context, username string) (PortalUserForLogin, error) {
	var u PortalUserForLogin
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, '', username, password_hash, user_type, status, 1 AS source_order
		FROM iam_admins
		WHERE username = $1
		UNION ALL
		SELECT user_id, tenant_id, username, password_hash, 3, status, 2 AS source_order
		FROM iam_tenant_users
		WHERE username = $1
		UNION ALL
		SELECT user_id, tenant_id, username, password_hash, 4, status, 3 AS source_order
		FROM iam_users
		WHERE username = $1
		ORDER BY source_order
		LIMIT 2
	`, username)
	if err != nil {
		return u, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return u, err
		}
		return u, pgx.ErrNoRows
	}
	if err := rows.Scan(&u.UserID, &u.TenantID, &u.Username, &u.PasswordHash, &u.UserType, &u.Status, new(int)); err != nil {
		return u, err
	}
	if rows.Next() {
		return PortalUserForLogin{}, fmt.Errorf("username is ambiguous across account types")
	}
	return u, rows.Err()
}

func (r *AuthRepository) UpdateEndUserLoginTime(ctx context.Context, userID string, loginTime time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE iam_users SET last_login_at = $1 WHERE user_id = $2
	`, loginTime, userID)
	return err
}

// CheckTenantActive 检查租户是否处于 active 状态
// 返回 (active, error)：active=true 表示租户正常，false 表示租户不存在或已停用/暂停
func (r *AuthRepository) CheckTenantActive(ctx context.Context, tenantID string) (bool, error) {
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT status FROM iam_tenants WHERE tenant_id = $1
	`, tenantID).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return status == "active", nil
}

var serviceScopePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

func ValidateServiceScopeFormat(scopes []string) error {
	for _, scope := range scopes {
		if scope == "" {
			return fmt.Errorf("scope must not be empty")
		}
		if !serviceScopePattern.MatchString(scope) {
			return fmt.Errorf("invalid scope format: %s", scope)
		}
	}
	return nil
}

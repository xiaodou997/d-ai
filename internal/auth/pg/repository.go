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

type ClientPrincipal struct {
	ClientID    string
	DisplayName string
	Status      string
}

type AuthCodeRecord struct {
	Code                string
	ClientID            string
	ClientType          string
	UserID              string
	Username            string
	TenantID            string
	UserType            int
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
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

func (r *AuthRepository) UpdateEndUserLoginTime(ctx context.Context, userID string, loginTime time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE iam_users SET last_login_at = $1 WHERE user_id = $2
	`, loginTime, userID)
	return err
}

func (r *AuthRepository) GetClientPrincipal(ctx context.Context, clientID string) (ClientPrincipal, error) {
	var svc ClientPrincipal
	err := r.pool.QueryRow(ctx, `
		SELECT client_id, display_name, status
		FROM gov_clients
		WHERE client_id = $1
	`, clientID).Scan(&svc.ClientID, &svc.DisplayName, &svc.Status)
	return svc, err
}

func (r *AuthRepository) CreateAuthCode(ctx context.Context, rec AuthCodeRecord) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO auth_oauth_codes (code, client_id, user_id, username, tenant_id, user_type, client_type, redirect_uri, code_challenge, code_challenge_method, expires_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11)
	`, rec.Code, rec.ClientID, rec.UserID, rec.Username, rec.TenantID, rec.UserType, rec.ClientType, rec.RedirectURI, rec.CodeChallenge, rec.CodeChallengeMethod, rec.ExpiresAt)
	return err
}

func (r *AuthRepository) ConsumeAuthCode(ctx context.Context, code, _ /* clientID unused */, redirectURI string) (*AuthCodeRecord, error) {
	var rec AuthCodeRecord
	// code 本身是 64 位随机唯一值，redirect_uri 匹配防止重定向劫持
	// client_id 已绑定在 code 记录中，由 RETURNING 返回，无需在 WHERE 中校验
	// 一次性消费：DELETE 即代表"已用"，省去 used 列与后续清理
	err := r.pool.QueryRow(ctx, `
		DELETE FROM auth_oauth_codes
		WHERE code = $1
		  AND redirect_uri = $2
		  AND expires_at > now()
		RETURNING code, client_id, user_id, username, COALESCE(tenant_id, ''), user_type, client_type, redirect_uri, code_challenge, code_challenge_method, expires_at
	`, code, redirectURI).Scan(
		&rec.Code, &rec.ClientID, &rec.UserID, &rec.Username,
		&rec.TenantID, &rec.UserType, &rec.ClientType, &rec.RedirectURI,
		&rec.CodeChallenge, &rec.CodeChallengeMethod, &rec.ExpiresAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consume auth code: %w", err)
	}
	return &rec, nil
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

// CheckSessionPrincipalActive revalidates the identity stored in an SSO session.
// A session is stale when the account was deleted or disabled, changed tenant,
// or belongs to a tenant that is no longer active.
func (r *AuthRepository) CheckSessionPrincipalActive(ctx context.Context, userID, tenantID string, userType int) (bool, error) {
	var query string
	var args []any
	switch userType {
	case 1, 2:
		query = `
			SELECT EXISTS (
				SELECT 1 FROM iam_admins
				WHERE user_id = $1 AND user_type = $2 AND status = 'active'
			)`
		args = []any{userID, userType}
	case 3:
		query = `
			SELECT EXISTS (
				SELECT 1
				FROM iam_tenant_users u
				JOIN iam_tenants t ON t.tenant_id = u.tenant_id
				WHERE u.user_id = $1
				  AND u.tenant_id = $2
				  AND u.status = 'active'
				  AND t.status = 'active'
			)`
		args = []any{userID, tenantID}
	case 4:
		query = `
			SELECT EXISTS (
				SELECT 1
				FROM iam_users u
				JOIN iam_tenants t ON t.tenant_id = u.tenant_id
				WHERE u.user_id = $1
				  AND u.tenant_id = $2
				  AND u.status = 'active'
				  AND t.status = 'active'
			)`
		args = []any{userID, tenantID}
	default:
		return false, nil
	}

	var active bool
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&active); err != nil {
		return false, fmt.Errorf("check SSO session principal: %w", err)
	}
	return active, nil
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

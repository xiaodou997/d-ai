package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IsUsernameTaken reports whether err is a unique violation on the normalized username index.
func IsUsernameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "ux_iam_accounts_username_normalized"
}

// IsEmailTaken reports whether err is a unique violation on the normalized email index.
func IsEmailTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "ux_iam_accounts_email_normalized"
}

type AuditEvent struct {
	EventType     string
	PrincipalType string
	UserID        string
	JTI           string
	RequestID     string
	Decision      string
	ReasonCode    string
	ReasonMessage string
	Metadata      map[string]any
}

func (r *AuthRepository) RecordAuditEvent(ctx context.Context, event AuditEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal auth audit metadata: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO auth_audit_logs (
			event_type, principal_type, user_id, jti, request_id,
			decision, reason_code, reason_message, metadata
		) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
		          $6, NULLIF($7, ''), NULLIF($8, ''), $9::jsonb)
	`, event.EventType, event.PrincipalType, event.UserID, event.JTI, event.RequestID,
		event.Decision, event.ReasonCode, event.ReasonMessage, string(metadata))
	return err
}

type AuthRepository struct {
	pool *pgxpool.Pool
}

func NewAuthRepository(pool *pgxpool.Pool) *AuthRepository {
	return &AuthRepository{pool: pool}
}

// PortalUserForLogin is the account record resolved by the unified Portal.
type PortalUserForLogin struct {
	UserID            string
	TenantID          string
	Username          string
	PasswordHash      string
	UserType          int
	Status            string
	CredentialVersion int64
}

func (r *AuthRepository) UpdateLoginTime(ctx context.Context, userID string, loginTime time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts SET last_login_at = $1 WHERE user_id = $2
	`, loginTime, userID)
	return err
}

func (r *AuthRepository) GetPortalUserForLogin(ctx context.Context, identifier string) (PortalUserForLogin, error) {
	var u PortalUserForLogin
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, COALESCE(tenant_id, ''), username, password_hash, user_type, status, credential_version
		FROM iam_accounts
		WHERE lower(username) = lower(btrim($1))
		   OR (email IS NOT NULL AND lower(email) = lower(btrim($1)))
	`, identifier).Scan(&u.UserID, &u.TenantID, &u.Username, &u.PasswordHash, &u.UserType, &u.Status, &u.CredentialVersion)
	return u, err
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

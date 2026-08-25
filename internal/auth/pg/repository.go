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
	authports "xiaodou/dai/internal/auth/ports"
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

type AuditEvent = authports.AuditEvent

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

var _ authports.AccountReader = (*AuthRepository)(nil)
var _ authports.AccountWriter = (*AuthRepository)(nil)
var _ authports.AuthAuditLogReader = (*AuthRepository)(nil)
var _ authports.LoginReader = (*AuthRepository)(nil)
var _ authports.AuthAuditRecorder = (*AuthRepository)(nil)

func NewAuthRepository(pool *pgxpool.Pool) *AuthRepository {
	return &AuthRepository{pool: pool}
}

// ListAuthAuditLogs reads the authentication audit projection used by the
// super-admin management endpoint. Pagination normalization and dynamic
// filters stay in the persistence adapter; HTTP only maps the result.
func (r *AuthRepository) ListAuthAuditLogs(ctx context.Context, filter authports.AuthAuditLogFilter) (authports.AuthAuditLogPage, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	size := filter.Size
	if size < 1 || size > 100 {
		size = 20
	}

	where := "WHERE 1=1"
	args := make([]any, 0, 4)
	nextArg := 1
	addFilter := func(column, value string) {
		if value == "" {
			return
		}
		where += fmt.Sprintf(" AND %s = $%d", column, nextArg)
		args = append(args, value)
		nextArg++
	}
	addFilter("event_type", filter.EventType)
	addFilter("principal_type", filter.PrincipalType)
	addFilter("user_id", filter.UserID)
	addFilter("decision", filter.Decision)

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM auth_audit_logs "+where, args...).Scan(&total); err != nil {
		return authports.AuthAuditLogPage{}, err
	}

	queryArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, event_type, principal_type, COALESCE(user_id, ''),
		       decision, COALESCE(reason_code, ''), COALESCE(reason_message, ''), created_at
		FROM auth_audit_logs %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, where, nextArg, nextArg+1), queryArgs...)
	if err != nil {
		return authports.AuthAuditLogPage{}, err
	}
	defer rows.Close()

	result := authports.AuthAuditLogPage{
		Records: make([]authports.AuthAuditLog, 0),
		Total:   total,
		Page:    page,
		Size:    size,
	}
	for rows.Next() {
		var item authports.AuthAuditLog
		if err := rows.Scan(
			&item.ID, &item.EventType, &item.PrincipalType, &item.UserID,
			&item.Decision, &item.ReasonCode, &item.ReasonMessage, &item.CreatedAt,
		); err != nil {
			return authports.AuthAuditLogPage{}, err
		}
		result.Records = append(result.Records, item)
	}
	if err := rows.Err(); err != nil {
		return authports.AuthAuditLogPage{}, err
	}
	return result, nil
}

func (r *AuthRepository) GetCurrentUserSnapshot(ctx context.Context, userID string, userType int) (authports.CurrentUserSnapshot, error) {
	var snapshot authports.CurrentUserSnapshot
	err := r.pool.QueryRow(ctx, `
		SELECT u.user_id, u.username, u.user_type, COALESCE(u.tenant_id, ''),
		       COALESCE(t.tenant_name, ''), u.mfa_enabled, u.status
		FROM iam_accounts u
		LEFT JOIN iam_tenants t ON t.tenant_id = u.tenant_id
		WHERE u.user_id = $1 AND u.user_type = $2
	`, userID, userType).Scan(
		&snapshot.UserID, &snapshot.Username, &snapshot.UserType, &snapshot.TenantID,
		&snapshot.TenantName, &snapshot.MFAEnabled, &snapshot.Status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return authports.CurrentUserSnapshot{}, authports.ErrAccountNotFound
	}
	return snapshot, err
}

func (r *AuthRepository) GetPasswordHash(ctx context.Context, userID string, userType int) (string, error) {
	var hash string
	err := r.pool.QueryRow(ctx, `
		SELECT password_hash FROM iam_accounts
		WHERE user_id = $1 AND user_type = $2 AND status <> 'deleted'
	`, userID, userType).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", authports.ErrAccountNotFound
	}
	return hash, err
}

func (r *AuthRepository) UpdatePassword(ctx context.Context, userID string, userType int, passwordHash string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts
		SET password_hash = $1, credential_version = credential_version + 1, updated_at = $2
		WHERE user_id = $3 AND user_type = $4 AND status <> 'deleted'
	`, passwordHash, time.Now().UTC(), userID, userType)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *AuthRepository) UpdateProfile(ctx context.Context, input authports.ProfileUpdate) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts
		SET username = CASE WHEN $1 THEN $2 ELSE username END,
		    email = CASE WHEN $3 THEN NULLIF($4, '') ELSE email END,
		    credential_version = credential_version + 1,
		    updated_at = $5
		WHERE user_id = $6 AND user_type = $7 AND status <> 'deleted'
	`, input.UsernameSet, input.Username, input.EmailSet, input.Email, time.Now().UTC(), input.UserID, input.UserType)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// PortalUserForLogin is the account record resolved by the unified Portal.
type PortalUserForLogin = authports.LoginAccount

func (r *AuthRepository) UpdateLoginTime(ctx context.Context, userID string, loginTime time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts SET last_login_at = $1 WHERE user_id = $2
	`, loginTime, userID)
	return err
}

func (r *AuthRepository) GetPortalUserForLogin(ctx context.Context, identifier string) (PortalUserForLogin, error) {
	var u PortalUserForLogin
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, COALESCE(tenant_id, ''), username, password_hash, user_type, status, credential_version, credential_state, mfa_enabled
		FROM iam_accounts
		WHERE lower(username) = lower(btrim($1))
		   OR (email IS NOT NULL AND lower(email) = lower(btrim($1)))
	`, identifier).Scan(&u.UserID, &u.TenantID, &u.Username, &u.PasswordHash, &u.UserType, &u.Status, &u.CredentialVersion, &u.CredentialState, &u.MFAEnabled)
	return u, err
}

// LookupTenantForLogin returns only the tenant dimension used by abuse
// controls. It is intentionally not exposed to the HTTP response, so a failed
// lookup cannot be used to enumerate accounts or tenants.
func (r *AuthRepository) LookupTenantForLogin(ctx context.Context, identifier string) string {
	var tenantID string
	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(tenant_id, '') FROM iam_accounts
		WHERE lower(username) = lower(btrim($1))
		   OR (email IS NOT NULL AND lower(email) = lower(btrim($1)))
		LIMIT 1
	`, identifier).Scan(&tenantID)
	return tenantID
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

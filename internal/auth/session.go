package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRefreshTokenReused  = errors.New("refresh token reused")
	ErrSessionInactive     = errors.New("session is inactive")
	ErrAccountInactive     = errors.New("account is inactive")
	ErrTenantInactive      = errors.New("tenant is inactive")
)

// SessionService owns opaque refresh tokens and their server-side session family.
type SessionService struct {
	pool       *pgxpool.Pool
	jwt        *JWTService
	refreshTTL time.Duration
}

func NewSessionService(pool *pgxpool.Pool, jwt *JWTService, refreshTTL time.Duration) *SessionService {
	if refreshTTL <= 0 {
		refreshTTL = 7 * 24 * time.Hour
	}
	return &SessionService{pool: pool, jwt: jwt, refreshTTL: refreshTTL}
}

func (s *SessionService) Create(ctx context.Context, principal Principal) (*TokenPair, error) {
	sessionID := uuid.New()
	raw, hash, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(s.refreshTTL)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var accountStatus, tenantStatus, credentialState string
	var currentCredentialVersion int64
	if err := tx.QueryRow(ctx, `
		SELECT a.status, a.credential_version, a.credential_state, COALESCE(t.status, 'active')
		FROM iam_accounts a
		LEFT JOIN iam_tenants t ON t.tenant_id = a.tenant_id
		WHERE a.user_id = $1
		FOR SHARE OF a
	`, principal.UserID).Scan(&accountStatus, &currentCredentialVersion, &credentialState, &tenantStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccountInactive
		}
		return nil, fmt.Errorf("validate login account: %w", err)
	}
	if accountStatus != "active" || credentialState != "active" || currentCredentialVersion != principal.CredentialVersion {
		return nil, ErrAccountInactive
	}
	if principal.UserType >= 3 && tenantStatus != "active" {
		return nil, ErrTenantInactive
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO auth_sessions (session_id, user_id, credential_version, expires_at, created_at, updated_at, last_refreshed_at)
		VALUES ($1, $2, $3, $4, $5, $5, $5)
	`, sessionID, principal.UserID, principal.CredentialVersion, expiresAt, now); err != nil {
		return nil, fmt.Errorf("create auth session: %w", err)
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO auth_refresh_tokens (token_hash, session_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4)
	`, hash, sessionID, expiresAt, now); err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	access, err := s.jwt.GenerateAccessToken(principal, sessionID.String())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.tokenPair(access, raw, s.refreshTTL), nil
}

func (s *SessionService) Rotate(ctx context.Context, raw string) (*TokenPair, Principal, error) {
	var zero Principal
	presentedHash := hashRefreshToken(raw)
	newRaw, newHash, err := newRefreshToken()
	if err != nil {
		return nil, zero, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, zero, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sessionID uuid.UUID
	var tokenStatus, sessionStatus, accountStatus, tenantStatus, credentialState string
	var tokenExpiresAt, sessionExpiresAt time.Time
	var sessionCredentialVersion, accountCredentialVersion int64
	principal := Principal{}
	err = tx.QueryRow(ctx, `
		SELECT rt.session_id, rt.status, rt.expires_at,
		       s.status, s.expires_at, s.credential_version,
		       a.user_id, a.username, COALESCE(a.tenant_id, ''), a.user_type,
		       a.status, a.credential_version, a.credential_state,
		       COALESCE(t.status, 'active')
		FROM auth_refresh_tokens rt
		JOIN auth_sessions s ON s.session_id = rt.session_id
		JOIN iam_accounts a ON a.user_id = s.user_id
		LEFT JOIN iam_tenants t ON t.tenant_id = a.tenant_id
		WHERE rt.token_hash = $1
		FOR UPDATE OF rt, s, a
	`, presentedHash).Scan(
		&sessionID, &tokenStatus, &tokenExpiresAt,
		&sessionStatus, &sessionExpiresAt, &sessionCredentialVersion,
		&principal.UserID, &principal.Username, &principal.TenantID, &principal.UserType,
		&accountStatus, &accountCredentialVersion, &credentialState, &tenantStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, zero, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, zero, fmt.Errorf("load refresh session: %w", err)
	}
	principal.UserTypeDisplay = userTypeDisplay(principal.UserType)
	principal.CredentialVersion = accountCredentialVersion
	now := time.Now().UTC()

	if sessionStatus != "active" {
		return nil, principal, ErrSessionInactive
	}
	if tokenStatus != "active" {
		if err := revokeSessionTx(ctx, tx, sessionID, "refresh_token_reused", now); err != nil {
			return nil, zero, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, zero, err
		}
		return nil, principal, ErrRefreshTokenReused
	}
	invalidReason, invalidErr := "", error(nil)
	switch {
	case !now.Before(tokenExpiresAt) || !now.Before(sessionExpiresAt):
		invalidReason, invalidErr = "session_expired", ErrInvalidRefreshToken
	case accountStatus != "active":
		invalidReason, invalidErr = "account_inactive", ErrAccountInactive
	case credentialState != "active":
		invalidReason, invalidErr = "credential_inactive", ErrSessionInactive
	case sessionCredentialVersion != accountCredentialVersion:
		invalidReason, invalidErr = "credential_changed", ErrSessionInactive
	case principal.UserType >= 3 && tenantStatus != "active":
		invalidReason, invalidErr = "tenant_inactive", ErrTenantInactive
	}
	if invalidErr != nil {
		if err := revokeSessionTx(ctx, tx, sessionID, invalidReason, now); err != nil {
			return nil, zero, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, zero, err
		}
		return nil, principal, invalidErr
	}

	access, err := s.jwt.GenerateAccessToken(principal, sessionID.String())
	if err != nil {
		return nil, zero, err
	}
	result, err := tx.Exec(ctx, `
		UPDATE auth_refresh_tokens
		SET status = 'consumed', consumed_at = $1, replaced_by_hash = $2
		WHERE token_hash = $3 AND status = 'active'
	`, now, newHash, presentedHash)
	if err != nil {
		return nil, zero, err
	}
	if result.RowsAffected() != 1 {
		return nil, zero, ErrRefreshTokenReused
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO auth_refresh_tokens (token_hash, session_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4)
	`, newHash, sessionID, sessionExpiresAt, now); err != nil {
		return nil, zero, err
	}
	if _, err = tx.Exec(ctx, `
		UPDATE auth_sessions SET last_refreshed_at = $1, updated_at = $1 WHERE session_id = $2
	`, now, sessionID); err != nil {
		return nil, zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, zero, err
	}
	return s.tokenPair(access, newRaw, time.Until(sessionExpiresAt)), principal, nil
}

func (s *SessionService) Revoke(ctx context.Context, sessionID, reason string) error {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return ErrSessionInactive
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := revokeSessionTx(ctx, tx, id, reason, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteExpired removes expired families in bounded batches. Refresh-token rows
// are removed by ON DELETE CASCADE and are no longer useful after absolute expiry.
func (s *SessionService) DeleteExpired(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	result, err := s.pool.Exec(ctx, `
		WITH expired AS (
			SELECT session_id FROM auth_sessions
			WHERE expires_at <= now()
			ORDER BY expires_at
			LIMIT $1
		)
		DELETE FROM auth_sessions s USING expired e WHERE s.session_id = e.session_id
	`, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func revokeSessionTx(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, reason string, now time.Time) error {
	if _, err := tx.Exec(ctx, `
		UPDATE auth_sessions
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, $1),
		    revoke_reason = $2, updated_at = $1
		WHERE session_id = $3
	`, now, reason, sessionID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE auth_refresh_tokens
		SET status = 'consumed', consumed_at = COALESCE(consumed_at, $1)
		WHERE session_id = $2 AND status = 'active'
	`, now, sessionID)
	return err
}

func (s *SessionService) tokenPair(access, refresh string, refreshRemaining time.Duration) *TokenPair {
	if refreshRemaining < 0 {
		refreshRemaining = 0
	}
	return &TokenPair{
		AccessToken: access, RefreshToken: refresh,
		ExpiresIn:        int64(s.jwt.AccessTokenExpiration().Seconds()),
		RefreshExpiresIn: int64(refreshRemaining.Seconds()),
	}
}

func newRefreshToken() (string, []byte, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", nil, fmt.Errorf("generate refresh token: %w", err)
	}
	raw := "dai_rt_" + base64.RawURLEncoding.EncodeToString(random)
	return raw, hashRefreshToken(raw), nil
}

func hashRefreshToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func userTypeDisplay(userType int) string {
	switch userType {
	case 1:
		return "超级管理员"
	case 2:
		return "平台管理员"
	case 3:
		return "租户"
	case 4:
		return "终端用户"
	default:
		return "未知"
	}
}

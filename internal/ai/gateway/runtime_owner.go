package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
)

func verifyRuntimeAPIKeyOwner(ctx context.Context, pool *pgxpool.Pool, row runtimeAPIKeyRecord) error {
	if pool == nil {
		return errors.New("runtime API key owner database is not configured")
	}
	if row.TenantID == "" {
		return errors.New("runtime API key has no tenant owner")
	}

	var active bool
	switch row.OwnerType {
	case "tenant":
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM iam_tenants
				WHERE tenant_id = $1 AND status = 'active'
			)
		`, row.TenantID).Scan(&active); err != nil {
			return fmt.Errorf("verify API key tenant owner: %w", err)
		}
	case "user":
		if !row.UserID.Valid || row.UserID.String == "" {
			return errors.New("user API key has no user owner")
		}
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM iam_accounts a
				JOIN iam_tenants t ON t.tenant_id = a.tenant_id
				WHERE a.user_id = $1
				  AND a.tenant_id = $2
				  AND a.user_type = 4
				  AND a.status = 'active'
				  AND t.status = 'active'
			)
		`, row.UserID.String, row.TenantID).Scan(&active); err != nil {
			return fmt.Errorf("verify API key user owner: %w", err)
		}
	default:
		return fmt.Errorf("unsupported API key owner type %q", row.OwnerType)
	}
	if !active {
		return errors.New("API key owner is inactive")
	}
	return nil
}


func verifyRuntimeJWTTaskSubject(ctx context.Context, pool *pgxpool.Pool, ref asynctask.SubjectRef) (coreidentity.Subject, error) {
	if pool == nil {
		return coreidentity.Subject{}, errors.New("runtime JWT task database is not configured")
	}
	if ref.TenantID == "" || ref.JWTAuthUserID == "" || ref.JWTAuthUserType < 1 || ref.JWTAuthUserType > 4 ||
		ref.JWTSessionID == "" || ref.JWTCredentialVersion <= 0 {
		return coreidentity.Subject{}, errors.New("queued JWT task is missing its authentication reference")
	}

	var valid bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM auth_sessions s
			JOIN iam_accounts a ON a.user_id = s.user_id
			JOIN iam_tenants t ON t.tenant_id = $5
			WHERE s.session_id = $1::uuid
			  AND s.user_id = $2
			  AND s.status = 'active'
			  AND s.expires_at > now()
			  AND s.credential_version = $3
			  AND a.credential_version = $3
			  AND a.user_type = $4
			  AND a.status = 'active'
			  AND a.credential_state = 'active'
			  AND t.status = 'active'
			  AND (
			    ($6 <> '' AND $4 = 4 AND $6 = $2 AND a.tenant_id = $5)
			    OR ($6 = '' AND $4 = 3 AND a.tenant_id = $5)
			    OR ($6 = '' AND $4 IN (1, 2) AND COALESCE(a.tenant_id, '') = '')
			  )
		)
	`, ref.JWTSessionID, ref.JWTAuthUserID, ref.JWTCredentialVersion, ref.JWTAuthUserType, ref.TenantID, ref.UserID).Scan(&valid); err != nil {
		return coreidentity.Subject{}, fmt.Errorf("verify queued JWT task session: %w", err)
	}
	if !valid {
		return coreidentity.Subject{}, errors.New("queued JWT task session is inactive")
	}

	scope := coreidentity.ScopeTenant
	if ref.UserID != "" {
		scope = coreidentity.ScopeUser
	}
	return coreidentity.Subject{
		AuthMethod:           coreidentity.AuthMethodJWT,
		RequestSource:        coreidentity.RequestSourceWebImage,
		Scope:                scope,
		TenantID:             ref.TenantID,
		UserID:               ref.UserID,
		JWTAuthUserID:        ref.JWTAuthUserID,
		JWTAuthUserType:      ref.JWTAuthUserType,
		JWTSessionID:         ref.JWTSessionID,
		JWTCredentialVersion: ref.JWTCredentialVersion,
	}, nil
}

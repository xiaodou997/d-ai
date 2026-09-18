package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
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

package transport

import (
	"context"
	"errors"
	"testing"
	"time"

	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/internal/dbtest"
	tenantpg "xiaodou/dai/internal/tenant/pg"
)

func TestAIIdentityAdapterChecksTenantEndUserScopeThroughRepository(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Date(2026, time.August, 24, 8, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name, created_at, updated_at) VALUES ('identity-scope-a', 'Identity A', $1, $1), ('identity-scope-b', 'Identity B', $1, $1)`, now); err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status, created_at, updated_at)
		VALUES
			('identity-end-active', 'identity-scope-a', 'identity-end-active', 'hash', 4, 'active', $1, $1),
			('identity-end-deleted', 'identity-scope-a', 'identity-end-deleted', 'hash', 4, 'deleted', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed end users: %v", err)
	}

	adapter := &aiIdentityAdapter{tenants: tenantpg.NewTenantRepository(pool)}
	if err := adapter.CheckTenantEndUser(ctx, "identity-scope-a", "identity-end-active"); err != nil {
		t.Fatalf("active scoped end user: %v", err)
	}
	if err := adapter.CheckTenantEndUser(ctx, "identity-scope-b", "identity-end-active"); !errors.Is(err, aitransport.ErrEndUserNotFound) {
		t.Fatalf("cross-tenant result = %v", err)
	}
	if err := adapter.CheckTenantEndUser(ctx, "identity-scope-a", "identity-end-deleted"); !errors.Is(err, aitransport.ErrEndUserNotFound) {
		t.Fatalf("deleted result = %v", err)
	}
}

package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/dbtest"
	tenantpg "xiaodou/dai/internal/tenant/pg"
)

func TestNormalizedOptionalText(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		set, value := normalizedOptionalText(nil)
		if set || value != "" {
			t.Fatalf("expected omitted value, got set=%v value=%q", set, value)
		}
	})

	t.Run("trimmed", func(t *testing.T) {
		input := "  tenant note  "
		set, value := normalizedOptionalText(&input)
		if !set || value != "tenant note" {
			t.Fatalf("expected trimmed value, got set=%v value=%q", set, value)
		}
	})

	t.Run("explicit empty", func(t *testing.T) {
		input := "   "
		set, value := normalizedOptionalText(&input)
		if !set || value != "" {
			t.Fatalf("expected explicit empty value, got set=%v value=%q", set, value)
		}
	})
}

func TestEndUserScopeAlwaysValidatesTargetTypeAndTenant(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('scope-a', 'Scope A'), ('scope-b', 'Scope B');
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status)
		VALUES ('scope-admin', 'scope-admin', 'unused', 2, 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('scope-user', 'scope-a', 'scope-user', 'unused', 4, 'active');
	`); err != nil {
		t.Fatal(err)
	}
	h := &adminHandlers{pool: pool, tenantRepo: tenantpg.NewTenantRepository(pool)}

	if err := h.checkUserBelongsToTenant(ctx, "scope-user", ""); err != nil {
		t.Fatalf("admin target validation: %v", err)
	}
	if err := h.checkUserBelongsToTenant(ctx, "scope-admin", ""); err == nil {
		t.Fatal("admin account was accepted as an end-user target")
	}
	if err := h.checkUserBelongsToTenant(ctx, "scope-user", "scope-b"); err == nil {
		t.Fatal("cross-tenant end-user target was accepted")
	}
}

package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0020AddsCrossDomainReadModels(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 19 WHERE singleton = TRUE;
		DROP VIEW billing_recharge_order_projection;
		DROP VIEW payment_order_party_projection;
		DROP VIEW payment_admin_recharge_order_projection;
	`); err != nil {
		t.Fatalf("prepare schema 19 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0020_20260826_cross_domain_read_models.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0020: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 20 {
		t.Fatalf("migration version = %d, want 20", version)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('read-model-tenant', 'Read Model Tenant');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type)
		VALUES ('read-model-user', 'read-model-tenant', 'read-model-user', 'hash', 4);
		INSERT INTO bill_recharge_orders
			(order_id, order_type, tenant_id, user_id, credit_amount, operator_id)
		VALUES ('ORD_READ_MODEL', 'tenant_to_user', 'read-model-tenant', 'read-model-user', 42, 'operator');
	`); err != nil {
		t.Fatalf("seed read-model fixture: %v", err)
	}

	var tenantName, username string
	if err := pool.QueryRow(ctx, `
		SELECT tenant_name, username
		FROM billing_recharge_order_projection
		WHERE order_id = 'ORD_READ_MODEL'
	`).Scan(&tenantName, &username); err != nil {
		t.Fatalf("query billing read model: %v", err)
	}
	if tenantName != "Read Model Tenant" || username != "read-model-user" {
		t.Fatalf("read model names = %q/%q", tenantName, username)
	}

	var projectionCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT order_id FROM payment_order_party_projection
			UNION ALL
			SELECT order_id FROM payment_admin_recharge_order_projection
		) projections
		WHERE order_id = 'ORD_READ_MODEL'
	`).Scan(&projectionCount); err != nil {
		t.Fatalf("query payment read models: %v", err)
	}
	if projectionCount != 1 {
		t.Fatalf("payment projection count = %d, want 1", projectionCount)
	}
}

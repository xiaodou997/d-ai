package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0023AddsSystemReadModels(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 22 WHERE singleton = TRUE;
		DROP VIEW system_usage_projection;
		DROP VIEW system_balance_projection;
		DROP VIEW system_identity_projection;
		DROP VIEW system_recharge_projection;
	`); err != nil {
		t.Fatalf("prepare schema 22 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0023_20260826_system_read_models.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0023: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 23 {
		t.Fatalf("migration version = %d, want 23", version)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('system-read-tenant', 'System Read Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('system-read-user', 'system-read-tenant', 'system-read-user', 'hash', 4, 'active');
		UPDATE bill_accounts SET balance_micro = 99 WHERE account_id = 'system-read-tenant';
		INSERT INTO bill_recharge_orders
			(order_id, order_type, tenant_id, credit_amount, paid_amount, operator_id)
		VALUES ('system-read-order', 'platform_to_tenant', 'system-read-tenant', 25, 20, 'operator');
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, tenant_id, user_id, model_code, tenant_payable,
			 user_charged, user_payable, billing_status, request_status, request_source)
		VALUES ('system-read-request', 'user', 'system-read-tenant', 'system-read-user',
			 'system-read-model', 3, 7, 7, 'settled', 'success', 'system-test');
	`); err != nil {
		t.Fatalf("seed system read-model fixture: %v", err)
	}

	var activeTenants, users int64
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM system_identity_projection WHERE entity_kind = 'tenant' AND status = 'active'),
		  (SELECT COUNT(*) FROM system_identity_projection WHERE entity_kind = 'user')
	`).Scan(&activeTenants, &users); err != nil {
		t.Fatalf("query system identity read model: %v", err)
	}
	if activeTenants != 1 || users != 1 {
		t.Fatalf("system identity projection = tenants:%d users:%d", activeTenants, users)
	}

	var paid, balance, usage int64
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT SUM(paid_amount) FROM system_recharge_projection),
		  (SELECT SUM(balance_micro) FROM system_balance_projection WHERE account_kind = 1),
		  (SELECT SUM(user_charged) FROM system_usage_projection WHERE billing_status = 'settled')
	`).Scan(&paid, &balance, &usage); err != nil {
		t.Fatalf("query system financial read models: %v", err)
	}
	if paid != 20 || balance != 99 || usage != 7 {
		t.Fatalf("system projections = paid:%d balance:%d usage:%d", paid, balance, usage)
	}
}

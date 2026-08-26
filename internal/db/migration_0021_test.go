package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0021AddsTenantReadModels(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 20 WHERE singleton = TRUE;
		DROP VIEW tenant_usage_projection;
		DROP VIEW tenant_self_overview_projection;
		DROP VIEW tenant_management_projection;
	`); err != nil {
		t.Fatalf("prepare schema 20 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0021_20260826_tenant_read_models.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0021: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 21 {
		t.Fatalf("migration version = %d, want 21", version)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('tenant-read-model', 'Tenant Read Model');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type)
		VALUES ('tenant-read-user', 'tenant-read-model', 'tenant-read-user', 'hash', 4);
		INSERT INTO iam_invitation_codes (code, tenant_id, created_by)
		VALUES ('tenant-read-code', 'tenant-read-model', 'tenant-read-user');
		UPDATE bill_accounts SET balance_micro = 42 WHERE account_id = 'tenant-read-user';
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, tenant_id, user_id, model_code, user_charged,
			 user_payable, billing_status, request_status, request_source)
		VALUES ('tenant-read-request', 'user', 'tenant-read-model', 'tenant-read-user',
			 'read-model', 7, 7, 'settled', 'success', 'portal');
	`); err != nil {
		t.Fatalf("seed tenant read-model fixture: %v", err)
	}

	var endUsers, invites int64
	var balance int64
	if err := pool.QueryRow(ctx, `
		SELECT end_user_count, invite_code_count, user_total_balance_micro
		FROM tenant_self_overview_projection
		WHERE tenant_id = 'tenant-read-model'
	`).Scan(&endUsers, &invites, &balance); err != nil {
		t.Fatalf("query tenant overview read model: %v", err)
	}
	if endUsers != 1 || invites != 1 || balance != 42 {
		t.Fatalf("tenant overview = users:%d invites:%d balance:%d", endUsers, invites, balance)
	}

	var username string
	if err := pool.QueryRow(ctx, `
		SELECT username
		FROM tenant_usage_projection
		WHERE tenant_id = 'tenant-read-model' AND user_id = 'tenant-read-user'
	`).Scan(&username); err != nil {
		t.Fatalf("query tenant usage read model: %v", err)
	}
	if username != "tenant-read-user" {
		t.Fatalf("tenant usage username = %q", username)
	}
}

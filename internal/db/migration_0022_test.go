package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0022AddsAdminEndUserReadModel(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 21 WHERE singleton = TRUE;
		DROP VIEW user_admin_end_user_projection;
	`); err != nil {
		t.Fatalf("prepare schema 21 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0022_20260826_user_read_models.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0022: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 22 {
		t.Fatalf("migration version = %d, want 22", version)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('user-read-tenant', 'User Read Tenant');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, email, user_type, status)
		VALUES ('user-read-end', 'user-read-tenant', 'user-read-end', 'hash', 'user-read@example.com', 4, 'active');
		UPDATE bill_accounts SET balance_micro = 123 WHERE account_id = 'user-read-end';
	`); err != nil {
		t.Fatalf("seed user read-model fixture: %v", err)
	}

	var tenantName, email string
	var balance int64
	if err := pool.QueryRow(ctx, `
		SELECT tenant_name, email, balance_micro
		FROM user_admin_end_user_projection
		WHERE user_id = 'user-read-end'
	`).Scan(&tenantName, &email, &balance); err != nil {
		t.Fatalf("query user read model: %v", err)
	}
	if tenantName != "User Read Tenant" || email != "user-read@example.com" || balance != 123 {
		t.Fatalf("user read model = tenant:%q email:%q balance:%d", tenantName, email, balance)
	}
}

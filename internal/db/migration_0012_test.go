package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0012AddsAccountActivation(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP VIEW user_admin_end_user_projection;
		DROP TABLE auth_activation_tokens;
		ALTER TABLE iam_accounts DROP COLUMN credential_state;
		UPDATE dai_schema_metadata SET version = 11 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 11 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0012_20260820_account_activation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0012: %v", err)
	}

	var version int
	var tokenTable, credentialColumn bool
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.auth_activation_tokens') IS NOT NULL`).Scan(&tokenTable); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'iam_accounts' AND column_name = 'credential_state'
		)
	`).Scan(&credentialColumn); err != nil {
		t.Fatal(err)
	}
	if version != 12 || !tokenTable || !credentialColumn {
		t.Fatalf("migration result = version:%d tokens:%t credential_state:%t", version, tokenTable, credentialColumn)
	}
}

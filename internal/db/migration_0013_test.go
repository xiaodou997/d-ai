package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0013AddsAdminMFAColumns(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE iam_accounts DROP COLUMN mfa_secret_encrypted, DROP COLUMN mfa_enabled, DROP COLUMN mfa_enrolled_at;
		UPDATE dai_schema_metadata SET version = 12 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 12 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0013_20260820_admin_mfa.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0013: %v", err)
	}

	var version int
	var columns int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'iam_accounts'
		  AND column_name IN ('mfa_secret_encrypted', 'mfa_enabled', 'mfa_enrolled_at')
	`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if version != 13 || columns != 3 {
		t.Fatalf("migration result = version:%d mfa_columns:%d", version, columns)
	}
}

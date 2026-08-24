package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0016AddsFileCleanupLeaseColumns(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 15 WHERE singleton = TRUE;
		ALTER TABLE file_assets
			DROP COLUMN IF EXISTS cleanup_owner,
			DROP COLUMN IF EXISTS cleanup_lease_until;
		DROP INDEX IF EXISTS idx_file_assets_cleanup_lease;
	`); err != nil {
		t.Fatalf("prepare schema 15 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0016_20260824_filestore_cleanup_leases.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0016: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	var ownerColumn, leaseColumn, leaseIndex bool
	if err := pool.QueryRow(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'file_assets' AND column_name = 'cleanup_owner'),
			EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'file_assets' AND column_name = 'cleanup_lease_until'),
			EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND tablename = 'file_assets' AND indexname = 'idx_file_assets_cleanup_lease')
	`).Scan(&ownerColumn, &leaseColumn, &leaseIndex); err != nil {
		t.Fatal(err)
	}
	if version != 16 || !ownerColumn || !leaseColumn || !leaseIndex {
		t.Fatalf("migration result = version:%d owner:%t lease:%t index:%t", version, ownerColumn, leaseColumn, leaseIndex)
	}
}

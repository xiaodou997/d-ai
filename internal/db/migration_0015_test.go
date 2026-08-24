package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0015AddsCleanupRunLeaseColumns(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 14 WHERE singleton = TRUE;
		ALTER TABLE sys_data_cleanup_runs
			DROP COLUMN IF EXISTS owner_id,
			DROP COLUMN IF EXISTS heartbeat_at,
			DROP COLUMN IF EXISTS lease_until;
		DROP INDEX IF EXISTS idx_sys_data_cleanup_runs_lease;
	`); err != nil {
		t.Fatalf("prepare schema 14 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0015_20260824_cleanup_run_leases.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0015: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	var ownerColumn bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'sys_data_cleanup_runs'
			  AND column_name = 'owner_id'
		)
	`).Scan(&ownerColumn); err != nil {
		t.Fatal(err)
	}
	var leaseIndex bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND tablename = 'sys_data_cleanup_runs'
			  AND indexname = 'idx_sys_data_cleanup_runs_lease'
		)
	`).Scan(&leaseIndex); err != nil {
		t.Fatal(err)
	}
	if version != 15 || !ownerColumn || !leaseIndex {
		t.Fatalf("migration result = version:%d owner_column:%t lease_index:%t", version, ownerColumn, leaseIndex)
	}
}

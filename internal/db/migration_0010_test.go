package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0010AddsDataCleanupPolicyAndRunHistory(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP INDEX idx_ai_audit_blobs_created_at;
		DROP INDEX uq_sys_data_cleanup_active;
		DROP INDEX idx_sys_data_cleanup_runs_created;
		DROP TABLE sys_data_cleanup_runs;
		DELETE FROM sys_settings WHERE key = 'data_cleanup';
		UPDATE dai_schema_metadata SET version = 9 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 9 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0010_20260819_data_cleanup.sql")
	if err != nil {
		t.Fatalf("read migration 0010: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0010: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	var policyExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sys_settings WHERE key = 'data_cleanup')`).Scan(&policyExists); err != nil {
		t.Fatalf("read cleanup policy: %v", err)
	}
	var tableExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.sys_data_cleanup_runs') IS NOT NULL`).Scan(&tableExists); err != nil {
		t.Fatalf("read cleanup table: %v", err)
	}
	if version != 10 || !policyExists || !tableExists {
		t.Fatalf("migration result = version:%d policy:%t table:%t, want 10/true/true", version, policyExists, tableExists)
	}
}

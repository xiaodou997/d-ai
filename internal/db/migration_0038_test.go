package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0038AddsUpstreamAccountDescription(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_upstream_accounts DROP COLUMN IF EXISTS description;
		UPDATE dai_schema_metadata SET version = 37 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 37 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0038_20260910_upstream_account_description.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0038: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 38 {
		t.Fatalf("migration version = %d, want 38", version)
	}
	var columns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'ai_upstream_accounts'
		  AND column_name = 'description'
	`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 1 {
		t.Fatalf("description columns = %d, want 1", columns)
	}
}

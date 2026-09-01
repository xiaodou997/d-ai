package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0027AddsGroupRoutePolicyVersion(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_groups DROP COLUMN route_policy_version;
		UPDATE dai_schema_metadata SET version = 26 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 26 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0027_20260901_group_route_policy_version.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0027: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 27 {
		t.Fatalf("migration version = %d, want 27", version)
	}

	var nullable, defaultValue string
	if err := pool.QueryRow(ctx, `
		SELECT is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'ai_groups' AND column_name = 'route_policy_version'
	`).Scan(&nullable, &defaultValue); err != nil {
		t.Fatal(err)
	}
	if nullable != "NO" || !strings.Contains(defaultValue, "1") {
		t.Fatalf("route_policy_version column = nullable:%q default:%q", nullable, defaultValue)
	}

	var checkCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_constraint c
		JOIN pg_class table_ref ON table_ref.oid = c.conrelid
		WHERE table_ref.relname = 'ai_groups'
		  AND pg_get_constraintdef(c.oid) LIKE '%route_policy_version > 0%'
	`).Scan(&checkCount); err != nil {
		t.Fatal(err)
	}
	if checkCount != 1 {
		t.Fatalf("route_policy_version check constraints = %d, want 1", checkCount)
	}
}

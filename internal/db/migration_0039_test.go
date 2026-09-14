package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0039RestoresGroupTargetPriority(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2, SchemaSQL: legacySchema40(t)})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	// init.sql is already at version 39; rewind the table and metadata to the
	// v38 shape (no priority column) and seed one binding row to verify the
	// default backfill.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_group_targets DROP COLUMN IF EXISTS priority;
		UPDATE dai_schema_metadata SET version = 38 WHERE singleton = TRUE;
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id)
		VALUES ('aaa1c9e0-0000-4000-8000-000000000039', 'tenant-m0039', 'migration 0039 group', 'eee1c9e0-0000-4000-8000-000000000039');
		INSERT INTO ai_group_targets (group_id, target_kind, target_id)
		VALUES ('aaa1c9e0-0000-4000-8000-000000000039', 'direct_upstream', 'bbb1c9e0-0000-4000-8000-000000000039');
	`); err != nil {
		t.Fatalf("prepare schema 38 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0039_20260910_group_target_priority.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0039: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 39 {
		t.Fatalf("migration version = %d, want 39", version)
	}
	var priority int
	if err := pool.QueryRow(ctx, `
		SELECT priority FROM ai_group_targets
		WHERE group_id = 'aaa1c9e0-0000-4000-8000-000000000039'
	`).Scan(&priority); err != nil {
		t.Fatal(err)
	}
	if priority != 100 {
		t.Fatalf("existing binding priority = %d, want default 100", priority)
	}
	// New bindings without an explicit priority must also start at 100.
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_group_targets (group_id, target_kind, target_id)
		VALUES ('aaa1c9e0-0000-4000-8000-000000000039', 'direct_upstream', 'ccc1c9e0-0000-4000-8000-000000000039')
	`); err != nil {
		t.Fatalf("insert default-priority binding: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT priority FROM ai_group_targets
		WHERE group_id = 'aaa1c9e0-0000-4000-8000-000000000039'
		  AND target_id = 'ccc1c9e0-0000-4000-8000-000000000039'
	`).Scan(&priority); err != nil {
		t.Fatal(err)
	}
	if priority != 100 {
		t.Fatalf("new binding priority = %d, want default 100", priority)
	}
	// Negative priority must be rejected by the CHECK constraint.
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_group_targets (group_id, target_kind, target_id, priority)
		VALUES ('aaa1c9e0-0000-4000-8000-000000000039', 'direct_upstream', 'ddd1c9e0-0000-4000-8000-000000000039', -1)
	`); err == nil {
		t.Fatal("negative priority accepted, want CHECK violation")
	}
}

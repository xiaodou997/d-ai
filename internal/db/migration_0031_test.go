package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0031AllowsAnnouncementDeleteAuditEvents(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE ann_audit_events
			DROP CONSTRAINT ann_audit_events_event_type_check,
			ADD CONSTRAINT ann_audit_events_event_type_check
				CHECK (event_type IN ('created', 'updated', 'published', 'archived', 'draft_deleted'));
		UPDATE dai_schema_metadata SET version = 30 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 30 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0031_20260902_announcement_delete.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0031: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 31 {
		t.Fatalf("migration version = %d, want 31", version)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO ann_audit_events
			(announcement_id, event_type, actor_user_type, actor_user_id)
		VALUES ('migration-0031', 'deleted', 1, 'migration-admin')
	`); err != nil {
		t.Fatalf("insert delete audit event: %v", err)
	}
}

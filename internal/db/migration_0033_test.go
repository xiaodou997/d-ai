package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0033AddsPrivacySafePromptAudit(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DELETE FROM ai_settings WHERE key = 'prompt_audit_config';
		DROP TABLE ai_prompt_audit_events;
		DROP INDEX idx_ai_content_moderation_logs_input_hash;
		ALTER TABLE ai_content_moderation_logs DROP COLUMN input_hash;
		UPDATE dai_schema_metadata SET version = 32 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 32 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0033_20260904_prompt_audit.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0033: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 33 {
		t.Fatalf("migration version = %d, want 33", version)
	}
	var forbiddenColumns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'ai_prompt_audit_events'
		  AND column_name IN ('full_prompt', 'raw_prompt', 'prompt_text', 'scan_text')
	`).Scan(&forbiddenColumns); err != nil {
		t.Fatal(err)
	}
	if forbiddenColumns != 0 {
		t.Fatalf("prompt audit table contains %d forbidden raw-text columns", forbiddenColumns)
	}
	var mode string
	if err := pool.QueryRow(ctx, `SELECT value->>'mode' FROM ai_settings WHERE key='prompt_audit_config'`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "off" {
		t.Fatalf("default prompt audit mode = %q", mode)
	}
}

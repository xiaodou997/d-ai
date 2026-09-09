package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0037AddsRequestRecordingStorageAndDefaults(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DELETE FROM ai_settings WHERE key = 'request_recording';
		ALTER TABLE ai_request_payloads DROP COLUMN request_headers, DROP COLUMN response_headers;
		UPDATE dai_schema_metadata SET version = 36 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 36 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0037_20260909_request_record_headers.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0037: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 37 {
		t.Fatalf("migration version = %d, want 37", version)
	}
	var columns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'ai_request_payloads'
		  AND column_name IN ('request_headers', 'response_headers')
	`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 2 {
		t.Fatalf("request recording columns = %d, want 2", columns)
	}
	var level string
	var sensitiveHeaders int
	if err := pool.QueryRow(ctx, `
		SELECT value->>'level', jsonb_array_length(value->'sensitive_headers')
		FROM ai_settings WHERE key = 'request_recording'
	`).Scan(&level, &sensitiveHeaders); err != nil {
		t.Fatal(err)
	}
	if level != "basic" || sensitiveHeaders < 1 {
		t.Fatalf("request recording defaults = level %q, sensitive headers %d", level, sensitiveHeaders)
	}
}

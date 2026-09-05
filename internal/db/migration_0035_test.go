package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0035AddsStreamSettlementStateColumns(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_usage_logs
			DROP COLUMN provider_terminal_state,
			DROP COLUMN client_delivery_state,
			DROP COLUMN cancellation_origin,
			DROP COLUMN billing_reason,
			DROP COLUMN response_summary_state;
		UPDATE dai_schema_metadata SET version = 34 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 34 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0035_20260905_stream_settlement_states.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0035: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 35 {
		t.Fatalf("migration version = %d, want 35", version)
	}
	var columns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'ai_usage_logs'
		  AND column_name IN ('provider_terminal_state', 'client_delivery_state', 'cancellation_origin', 'billing_reason', 'response_summary_state')
	`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 5 {
		t.Fatalf("stream settlement columns = %d, want 5", columns)
	}
}

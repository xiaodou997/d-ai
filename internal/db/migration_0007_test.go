package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0007AddsClosedOrderCleanupIndex(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP INDEX idx_pay_orders_closed_cleanup;
		UPDATE dai_schema_metadata SET version = 6 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 6 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0007_20260818_cleanup_closed_payment_orders.sql")
	if err != nil {
		t.Fatalf("read migration 0007: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0007: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	var indexName string
	if err := pool.QueryRow(ctx, `SELECT indexname FROM pg_indexes WHERE indexname = 'idx_pay_orders_closed_cleanup'`).Scan(&indexName); err != nil {
		t.Fatalf("read cleanup index: %v", err)
	}
	if version != 7 || indexName != "idx_pay_orders_closed_cleanup" {
		t.Fatalf("migration version/index = %d/%s, want 7/idx_pay_orders_closed_cleanup", version, indexName)
	}
}

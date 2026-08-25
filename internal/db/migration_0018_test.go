package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0018AddsPaymentSweepHealthIndex(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 17 WHERE singleton = TRUE;
		DROP INDEX IF EXISTS idx_pay_orders_sweep_retry_health;
	`); err != nil {
		t.Fatalf("prepare schema 17 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0018_20260825_payment_sweep_health_index.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0018: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	var indexExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND tablename = 'pay_orders'
			  AND indexname = 'idx_pay_orders_sweep_retry_health'
		)
	`).Scan(&indexExists); err != nil {
		t.Fatal(err)
	}
	if version != 18 || !indexExists {
		t.Fatalf("migration result = version:%d index:%t", version, indexExists)
	}
}

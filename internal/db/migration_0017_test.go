package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0017AddsPaymentSweepBackoffColumns(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 16 WHERE singleton = TRUE;
		ALTER TABLE pay_orders
			DROP COLUMN IF EXISTS sweep_attempts,
			DROP COLUMN IF EXISTS sweep_next_attempt_at,
			DROP COLUMN IF EXISTS sweep_last_attempt_at,
			DROP COLUMN IF EXISTS sweep_last_error;
		DROP INDEX IF EXISTS idx_pay_orders_sweep;
		CREATE INDEX idx_pay_orders_sweep
			ON pay_orders (status, expires_at)
			WHERE status IN ('created', 'paying');
	`); err != nil {
		t.Fatalf("prepare schema 16 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0017_20260824_payment_sweep_backoff.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0017: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	var attempts, nextAt, lastAt, lastError, retryIndex bool
	if err := pool.QueryRow(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'pay_orders' AND column_name = 'sweep_attempts'),
			EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'pay_orders' AND column_name = 'sweep_next_attempt_at'),
			EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'pay_orders' AND column_name = 'sweep_last_attempt_at'),
			EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'pay_orders' AND column_name = 'sweep_last_error'),
			EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND tablename = 'pay_orders' AND indexname = 'idx_pay_orders_sweep')
	`).Scan(&attempts, &nextAt, &lastAt, &lastError, &retryIndex); err != nil {
		t.Fatal(err)
	}
	if version != 17 || !attempts || !nextAt || !lastAt || !lastError || !retryIndex {
		t.Fatalf("migration result = version:%d attempts:%t next:%t last:%t error:%t index:%t", version, attempts, nextAt, lastAt, lastError, retryIndex)
	}
}

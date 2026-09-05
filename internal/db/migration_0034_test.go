package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0034AllowsVoidBillingStatus(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `UPDATE dai_schema_metadata SET version = 33 WHERE singleton = TRUE`); err != nil {
		t.Fatalf("prepare schema 33 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0034_20260905_stream_settlement_void.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0034: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 34 {
		t.Fatalf("migration version = %d, want 34", version)
	}
}

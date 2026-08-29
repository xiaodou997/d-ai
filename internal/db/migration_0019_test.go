package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0019RepairsHistoricalBillingStatusIndex(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP INDEX idx_ai_usage_logs_billing_status;
		ALTER TABLE ledger_credit_leases DROP CONSTRAINT ledger_credit_leases_check8;
		UPDATE dai_schema_metadata SET version = 18 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 18 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0019_20260826_repair_billing_status_index.sql")
	if err != nil {
		t.Fatalf("read migration 0019: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0019: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 19 {
		t.Fatalf("schema version = %d, want 19", version)
	}

	var indexDefinition, constraintDefinition string
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_indexdef(index_class.oid)
		FROM pg_class index_class
		JOIN pg_namespace index_namespace ON index_namespace.oid = index_class.relnamespace
		WHERE index_namespace.nspname = current_schema()
		  AND index_class.relname = 'idx_ai_usage_logs_billing_status'
	`).Scan(&indexDefinition); err != nil {
		t.Fatalf("read repaired index: %v", err)
	}
	if indexDefinition == "" {
		t.Fatal("migration created an empty index definition")
	}
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(constraint_row.oid, true)
		FROM pg_constraint constraint_row
		JOIN pg_class table_class ON table_class.oid = constraint_row.conrelid
		JOIN pg_namespace table_namespace ON table_namespace.oid = table_class.relnamespace
		WHERE table_namespace.nspname = current_schema()
		  AND table_class.relname = 'ledger_credit_leases'
		  AND constraint_row.conname = 'ledger_credit_leases_check8'
	`).Scan(&constraintDefinition); err != nil {
		t.Fatalf("read repaired lease constraint: %v", err)
	}
	if !strings.Contains(constraintDefinition, "settlement_state") || !strings.Contains(constraintDefinition, "escrow_state") {
		t.Fatalf("repaired lease constraint = %q", constraintDefinition)
	}
}

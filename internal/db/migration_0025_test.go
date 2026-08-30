package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0025AddsAccountAndTenantReadModels(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 24 WHERE singleton = TRUE;
		DROP VIEW tenant_income_projection;
		DROP VIEW system_account_stats_projection;
	`); err != nil {
		t.Fatalf("prepare schema 24 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0025_20260830_account_tenant_read_models.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0025: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 25 {
		t.Fatalf("migration version = %d, want 25", version)
	}

	var accountStats, tenantIncome bool
	if err := pool.QueryRow(ctx, `
		SELECT to_regclass('system_account_stats_projection') IS NOT NULL,
		       to_regclass('tenant_income_projection') IS NOT NULL
	`).Scan(&accountStats, &tenantIncome); err != nil {
		t.Fatal(err)
	}
	if !accountStats || !tenantIncome {
		t.Fatalf("read models present = account:%v income:%v", accountStats, tenantIncome)
	}
}

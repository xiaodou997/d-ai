package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0003ConsolidatesSignedBalanceAndDropsLegacyModel(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	// The canonical baseline is v18. Remove the later refund table that holds a
	// foreign key to bill_accounts, then use the checked-in 0003 rollback as a
	// faithful v3 -> v2 fixture instead of duplicating the old schema by hand.
	if _, err := pool.Exec(ctx, `
		DROP TABLE bill_refund_reversal_effects;
		UPDATE dai_schema_metadata SET version = 3 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare rollback fixture: %v", err)
	}
	rollback, err := os.ReadFile("rollback/0003_rollback.sql")
	if err != nil {
		t.Fatalf("read 0003 rollback: %v", err)
	}
	if _, err := pool.Exec(ctx, string(rollback)); err != nil {
		t.Fatalf("restore schema 2 fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status, current_overdraft)
		VALUES
			('tenant-m3-positive', 'Migration 3 Positive', 'active', 20),
			('tenant-m3-debt', 'Migration 3 Debt', 'active', 90);
		INSERT INTO iam_accounts
			(user_id, tenant_id, username, password_hash, user_type, status, current_overdraft)
		VALUES ('user-m3-positive', 'tenant-m3-positive', 'u_m3_positive', 'x', 4, 'active', 0);

		INSERT INTO bill_credit_packages
			(package_id, package_type, tenant_id, total_credits, remaining_credits, status, source)
		VALUES
			('pkg-m3-positive', 'tenant', 'tenant-m3-positive', 100, 70, 'available', 'ADMIN_RECHARGE'),
			('pkg-m3-debt', 'tenant', 'tenant-m3-debt', 80, 80, 'available', 'ADMIN_RECHARGE'),
			('pkg-m3-expired', 'tenant', 'tenant-m3-positive', 50, 50, 'expired', 'ADMIN_RECHARGE');
		INSERT INTO bill_credit_packages
			(package_id, package_type, tenant_id, user_id, total_credits, remaining_credits, status, source)
		VALUES ('pkg-m3-user', 'user', 'tenant-m3-positive', 'user-m3-positive', 60, 60, 'available', 'TENANT_RECHARGE');
	`); err != nil {
		t.Fatalf("seed schema 2 balance fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0003_20260817_signed_balance_ledger.sql")
	if err != nil {
		t.Fatalf("read migration 0003: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0003: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 3 {
		t.Fatalf("schema version = %d, want 3", version)
	}

	assertMigratedBalance(t, ctx, pool, "tenant-m3-positive", 50, 100, 50)
	assertMigratedBalance(t, ctx, pool, "tenant-m3-debt", -10, 80, 80)
	assertMigratedBalance(t, ctx, pool, "user-m3-positive", 60, 60, 0)

	var migratedPackages int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_credit_lots WHERE lot_id = 'pkg-m3-expired'`).Scan(&migratedPackages); err != nil {
		t.Fatalf("check expired package migration: %v", err)
	}
	if migratedPackages != 0 {
		t.Fatal("expired package was migrated into an active credit lot")
	}

	var oldPackages, oldOverdrafts, chargeOutbox bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.bill_credit_packages') IS NOT NULL`).Scan(&oldPackages); err != nil {
		t.Fatalf("check old package table: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.bill_overdraft_adjustments') IS NOT NULL`).Scan(&oldOverdrafts); err != nil {
		t.Fatalf("check old overdraft table: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.bill_charge_outbox') IS NOT NULL`).Scan(&chargeOutbox); err != nil {
		t.Fatalf("check charge outbox: %v", err)
	}
	if oldPackages || oldOverdrafts || !chargeOutbox {
		t.Fatalf("legacy/new tables = packages:%t overdrafts:%t outbox:%t", oldPackages, oldOverdrafts, chargeOutbox)
	}

	var oldOverdraftColumn bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'iam_tenants'
			  AND column_name = 'current_overdraft'
		)
	`).Scan(&oldOverdraftColumn); err != nil {
		t.Fatalf("check removed overdraft column: %v", err)
	}
	if oldOverdraftColumn {
		t.Fatal("legacy current_overdraft column still exists after migration")
	}
}

func assertMigratedBalance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, accountID string, wantBalance, wantGranted, wantConsumed int64) {
	t.Helper()
	var balance, granted, consumed int64
	if err := pool.QueryRow(ctx, `
		SELECT b.balance_micro,
		       COALESCE(SUM(l.granted_micro), 0)::bigint,
		       COALESCE(SUM(l.consumed_micro), 0)::bigint
		FROM bill_accounts b
		LEFT JOIN bill_credit_lots l ON l.account_id = b.account_id
		WHERE b.account_id = $1
		GROUP BY b.balance_micro
	`, accountID).Scan(&balance, &granted, &consumed); err != nil {
		t.Fatalf("read migrated account %s: %v", accountID, err)
	}
	if balance != wantBalance || granted != wantGranted || consumed != wantConsumed {
		t.Fatalf("account %s = balance:%d granted:%d consumed:%d, want %d/%d/%d", accountID, balance, granted, consumed, wantBalance, wantGranted, wantConsumed)
	}
}

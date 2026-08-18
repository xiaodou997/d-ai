package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0004RepairsLegacyUserTopupTenantIncome(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE bill_recharge_orders
			DROP CONSTRAINT bill_recharge_orders_payment_link_check,
			DROP CONSTRAINT bill_recharge_orders_payment_order_fk;
		DROP INDEX idx_bill_recharge_orders_payment_order;
		ALTER TABLE bill_recharge_orders
			DROP COLUMN payment_order_id,
			DROP COLUMN reversed_amount_micro,
			DROP COLUMN lost_amount_micro;
		ALTER TABLE pay_orders DROP COLUMN fulfillment_status;
		DROP INDEX uq_bill_recharge_orders_user_topup_income_payment_ref;
		UPDATE dai_schema_metadata SET version = 3 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 3 fixture: %v", err)
	}

	seedLegacyUserTopupGap(t, ctx, pool, "tenant_positive", "user_positive", 25_000_000, 10_000_000)
	seedLegacyUserTopupGap(t, ctx, pool, "tenant_debt", "user_debt", -5_000_000, 10_000_000)

	migration, err := os.ReadFile("changes/0004_20260817_repair_user_topup_tenant_income.sql")
	if err != nil {
		t.Fatalf("read migration 0004: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0004: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 4 {
		t.Fatalf("schema version = %d, want 4", version)
	}

	assertRepairedTenant(t, ctx, pool, "tenant_positive", 35_000_000, 35_000_000)
	assertRepairedTenant(t, ctx, pool, "tenant_debt", 5_000_000, 5_000_000)

	var orders int
	var credited int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(credit_amount), 0)
		FROM bill_recharge_orders
		WHERE order_type = 'user_topup_income' AND operator_id = 'system:data-repair:0004'
	`).Scan(&orders, &credited); err != nil {
		t.Fatalf("read repaired income orders: %v", err)
	}
	if orders != 2 || credited != 20_000_000 {
		t.Fatalf("repaired orders/amount = %d/%d, want 2/20000000", orders, credited)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_recharge_orders
		(order_id, order_type, tenant_id, credit_amount, paid_amount, payment_ref, operator_id)
		VALUES ('duplicate_income', 'user_topup_income', 'tenant_positive', 10000000, 1000,
		        'txn_tenant_positive', 'test')
	`); err == nil {
		t.Fatal("duplicate user_topup_income payment_ref was accepted")
	}
}

func seedLegacyUserTopupGap(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID string,
	userID string,
	balanceMicro int64,
	incomeMicro int64,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, $1, 'active')
	`, tenantID); err != nil {
		t.Fatalf("seed legacy tenant %s: %v", tenantID, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($2, $1, $2, 'x', 4, 'active')
	`, tenantID, userID); err != nil {
		t.Fatalf("seed legacy user %s: %v", userID, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bill_accounts SET balance_micro = $2 WHERE account_id = $1;
	`, tenantID, balanceMicro); err != nil {
		t.Fatalf("seed legacy tenant balance %s: %v", tenantID, err)
	}

	if balanceMicro > 0 {
		if _, err := pool.Exec(ctx, `
			INSERT INTO bill_credit_lots
			(lot_id, account_id, granted_micro, consumed_micro, source)
			VALUES ('existing_' || $1, $1, $2, 0, 'ADMIN_RECHARGE')
		`, tenantID, balanceMicro); err != nil {
			t.Fatalf("seed existing balance lot for %s: %v", tenantID, err)
		}
	}

	paidAt := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO pay_orders
		(order_id, out_trade_no, scene, tenant_id, user_id, topup_mode,
		 payment_amount_minor, gross_amount_micro_usd, credited_amount_micro_usd,
		 tenant_income_micro_usd, transaction_id, status, paid_at, expires_at)
		VALUES ('pay_' || $1, 'out_' || $1, 'user_topup', $1, $2, 'custom',
		        $3 / 10000, $3, $3, $3, 'txn_' || $1, 'paid',
		        $4::timestamptz, $4::timestamptz + interval '1 hour')
	`, tenantID, userID, incomeMicro, paidAt); err != nil {
		t.Fatalf("seed legacy user top-up for %s: %v", tenantID, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO pay_cash_ledger
		(txn_id, tenant_id, txn_type, amount_micro_usd, balance_after_micro_usd,
		 ref_type, ref_id, operator_id, idempotency_key, created_at)
		VALUES ('cash_' || $1, $1, 'topup_income', $2, $2,
		        'pay_order', 'pay_' || $1, 'system:wechatpay', 'wxpay:out_' || $1, $3)
	`, tenantID, incomeMicro, paidAt); err != nil {
		t.Fatalf("seed legacy cash ledger for %s: %v", tenantID, err)
	}
}

func assertRepairedTenant(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID string,
	wantBalance int64,
	wantLots int64,
) {
	t.Helper()
	var balance, lots int64
	if err := pool.QueryRow(ctx, `
		SELECT b.balance_micro,
		       COALESCE(SUM(l.granted_micro - l.consumed_micro), 0)::bigint
		FROM bill_accounts b
		LEFT JOIN bill_credit_lots l
		  ON l.account_id = b.account_id
		 AND l.expired_at IS NULL AND l.revoked_at IS NULL
		WHERE b.account_id = $1
		GROUP BY b.balance_micro
	`, tenantID).Scan(&balance, &lots); err != nil {
		t.Fatalf("read repaired tenant %s: %v", tenantID, err)
	}
	if balance != wantBalance || lots != wantLots {
		t.Fatalf("tenant %s balance/lots = %d/%d, want %d/%d", tenantID, balance, lots, wantBalance, wantLots)
	}
}

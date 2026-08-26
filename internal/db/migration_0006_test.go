package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0006AddsCompletedRefundAccounting(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP VIEW payment_admin_recharge_order_projection;
		DROP VIEW payment_order_party_projection;
		DROP VIEW billing_recharge_order_projection;
		DROP TABLE bill_refund_reversal_effects;
		DROP TABLE pay_refunds;
		ALTER TABLE pay_orders DROP COLUMN refund_status;
		ALTER TABLE bill_credit_lots DROP COLUMN expired_unused_micro;
		ALTER TABLE pay_cash_ledger DROP CONSTRAINT pay_cash_ledger_txn_type_check;
		ALTER TABLE pay_cash_ledger ADD CONSTRAINT pay_cash_ledger_txn_type_check
			CHECK (txn_type IN ('topup_income', 'consumption', 'withdraw', 'adjust'));
		UPDATE dai_schema_metadata SET version = 5 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 5 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0006_20260818_refund_recharge_reversal.sql")
	if err != nil {
		t.Fatalf("read migration 0006: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0006: %v", err)
	}

	var version int
	var refundStatus string
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_m6', 'Migration 6 Tenant', 'active')
	`); err != nil {
		t.Fatalf("seed migrated tenant: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO pay_orders (
			order_id, out_trade_no, scene, tenant_id, payment_amount_minor,
			gross_amount_micro_usd, credited_amount_micro_usd, status,
			fulfillment_status, paid_at, expires_at
		) VALUES (
			'PAY_M6', 'MCH_M6', 'tenant_topup', 'tenant_m6', 100,
			1000000, 1000000, 'paid', 'credited', now(), now() + interval '1 hour'
		)
		RETURNING refund_status
	`).Scan(&refundStatus); err != nil {
		t.Fatalf("seed migrated payment order: %v", err)
	}
	if version != 6 || refundStatus != "none" {
		t.Fatalf("migration version/refund status = %d/%s, want 6/none", version, refundStatus)
	}
	if _, err := pool.Exec(ctx, `UPDATE pay_orders SET refund_status = 'refunded' WHERE order_id = 'PAY_M6'`); err == nil {
		t.Fatal("refunded payment accepted without reversed fulfillment")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO pay_refunds (
			refund_id, payment_order_id, refund_method, refund_reference,
			refund_amount_minor, refunded_at, reason, operator_id
		) VALUES ('RFD_M6', 'PAY_M6', 'wechat', 'MCH_REFUND_M6', 100, now(), 'test', 'admin')
	`); err == nil {
		t.Fatal("wechat refund accepted without channel refund id")
	}
}

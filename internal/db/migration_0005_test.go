package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0005LinksRechargeOrdersAndSeparatesFulfillmentStatus(t *testing.T) {
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
		ALTER TABLE bill_recharge_orders
			DROP CONSTRAINT bill_recharge_orders_payment_link_check,
			DROP CONSTRAINT bill_recharge_orders_payment_order_fk;
		DROP INDEX idx_bill_recharge_orders_payment_order;
		ALTER TABLE bill_recharge_orders
			DROP COLUMN payment_order_id,
			DROP COLUMN reversed_amount_micro,
			DROP COLUMN lost_amount_micro;
		ALTER TABLE pay_orders DROP COLUMN fulfillment_status;
		UPDATE dai_schema_metadata SET version = 4 WHERE singleton = TRUE;

		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_m5', 'Migration Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_m5', 'tenant_m5', 'migration-user', 'x', 4, 'active');

		INSERT INTO pay_orders (
			order_id, out_trade_no, scene, tenant_id, user_id, payment_amount_minor,
			gross_amount_micro_usd, credited_amount_micro_usd, tenant_income_micro_usd,
			transaction_id, status, paid_at, expires_at, balance_order_id
		) VALUES (
			'PAY_M5', 'MCH_M5', 'user_topup', 'tenant_m5', 'user_m5', 1000,
			10000000, 10000000, 9000000, 'WX_M5', 'paid', now(), now() + interval '1 hour', 'ORD_M5_USER'
		);
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, user_id, credit_amount, paid_amount,
			payment_ref, operator_id
		) VALUES
			('ORD_M5_USER', 'online_user_topup', 'tenant_m5', 'user_m5', 10000000, 1000, 'WX_M5', 'system:wechatpay'),
			('ORD_M5_INCOME', 'user_topup_income', 'tenant_m5', NULL, 9000000, 1000, 'WX_M5', 'system:wechatpay');
	`); err != nil {
		t.Fatalf("prepare schema 4 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0005_20260818_unify_recharge_order_management.sql")
	if err != nil {
		t.Fatalf("read migration 0005: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0005: %v", err)
	}

	var version int
	var paymentLinks int
	var fulfillmentStatus string
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_recharge_orders WHERE payment_order_id = 'PAY_M5'`).Scan(&paymentLinks); err != nil {
		t.Fatalf("read payment links: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT fulfillment_status FROM pay_orders WHERE order_id = 'PAY_M5'`).Scan(&fulfillmentStatus); err != nil {
		t.Fatalf("read fulfillment status: %v", err)
	}
	if version != 5 || paymentLinks != 2 || fulfillmentStatus != "credited" {
		t.Fatalf("migration result version/links/status = %d/%d/%s, want 5/2/credited", version, paymentLinks, fulfillmentStatus)
	}
	if _, err := pool.Exec(ctx, `UPDATE pay_orders SET fulfillment_status = 'pending' WHERE order_id = 'PAY_M5'`); err == nil {
		t.Fatal("paid order accepted pending fulfillment status")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bill_recharge_orders
		SET reversed_amount_micro = credit_amount, lost_amount_micro = 1
		WHERE order_id = 'ORD_M5_USER'
	`); err == nil {
		t.Fatal("recharge order accepted reversal amounts above credited amount")
	}
}

package service_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/dbtest"
)

func TestReverseOnlineUserCreditKeepsPaymentPaidAndTenantIncome(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_reverse', 'Reverse Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_reverse', 'tenant_reverse', 'reverse-user', 'x', 4, 'active');

		INSERT INTO pay_orders (
			order_id, out_trade_no, scene, tenant_id, user_id, payment_amount_minor,
			gross_amount_micro_usd, credited_amount_micro_usd, tenant_income_micro_usd,
			transaction_id, status, fulfillment_status, paid_at, expires_at, balance_order_id
		) VALUES (
			'PAY_REVERSE', 'MCH_REVERSE', 'user_topup', 'tenant_reverse', 'user_reverse', 100,
			1200000, 1200000, 900000, 'WX_REVERSE', 'paid', 'credited', now(), now() + interval '1 hour', 'ORD_REVERSE_USER'
		);
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, user_id, credit_amount, paid_amount,
			payment_ref, payment_order_id, operator_id
		) VALUES
			('ORD_REVERSE_USER', 'online_user_topup', 'tenant_reverse', 'user_reverse', 1200000, 100, 'WX_REVERSE', 'PAY_REVERSE', 'system:wechatpay'),
			('ORD_REVERSE_INCOME', 'user_topup_income', 'tenant_reverse', NULL, 900000, 100, 'WX_REVERSE', 'PAY_REVERSE', 'system:wechatpay');
		INSERT INTO bill_credit_lots (
			lot_id, account_id, granted_micro, consumed_micro, source, recharge_order_id
		) VALUES
			('LOT_REVERSE_USER', 'user_reverse', 1000000, 400000, 'ONLINE_TOPUP', 'ORD_REVERSE_USER'),
			('LOT_REVERSE_INCOME', 'tenant_reverse', 900000, 0, 'USER_TOPUP_INCOME', 'ORD_REVERSE_INCOME');
		UPDATE bill_accounts SET balance_micro = 600000 WHERE account_id = 'user_reverse';
		UPDATE bill_accounts SET balance_micro = 900000 WHERE account_id = 'tenant_reverse';
	`); err != nil {
		t.Fatalf("seed online recharge: %v", err)
	}

	result, err := service.NewDeductionService(pool, zap.NewNop()).ReverseOrder("ORD_REVERSE_USER", "risk review", "admin-1")
	if err != nil {
		t.Fatalf("reverse online credit: %v", err)
	}
	if !result.IsPartial || result.OriginalCredits != 1200000 || result.ReversedCredits != 600000 || result.LostCredits != 600000 || result.FulfillmentStatus != "partially_reversed" {
		t.Fatalf("unexpected reverse result: %+v", result)
	}

	var paymentStatus, fulfillmentStatus, primaryStatus, incomeStatus string
	var userBalance, tenantBalance, reversedAmount, lostAmount int64
	if err := pool.QueryRow(ctx, `SELECT status, fulfillment_status FROM pay_orders WHERE order_id = 'PAY_REVERSE'`).Scan(&paymentStatus, &fulfillmentStatus); err != nil {
		t.Fatalf("read payment state: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT status, reversed_amount_micro, lost_amount_micro
		FROM bill_recharge_orders WHERE order_id = 'ORD_REVERSE_USER'
	`).Scan(&primaryStatus, &reversedAmount, &lostAmount); err != nil {
		t.Fatalf("read primary recharge state: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM bill_recharge_orders WHERE order_id = 'ORD_REVERSE_INCOME'`).Scan(&incomeStatus); err != nil {
		t.Fatalf("read income recharge state: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_reverse'`).Scan(&userBalance); err != nil {
		t.Fatalf("read user balance: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'tenant_reverse'`).Scan(&tenantBalance); err != nil {
		t.Fatalf("read tenant balance: %v", err)
	}
	if paymentStatus != "paid" || fulfillmentStatus != "partially_reversed" || primaryStatus != "reversed" || incomeStatus != "active" || userBalance != 0 || tenantBalance != 900000 || reversedAmount != 600000 || lostAmount != 600000 {
		t.Fatalf("states payment/fulfillment/primary/income/user/tenant/reversed/lost = %s/%s/%s/%s/%d/%d/%d/%d", paymentStatus, fulfillmentStatus, primaryStatus, incomeStatus, userBalance, tenantBalance, reversedAmount, lostAmount)
	}
}

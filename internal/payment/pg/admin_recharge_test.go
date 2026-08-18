package pg_test

import (
	"context"
	"testing"

	"xiaodou/dai/internal/dbtest"
	paymentpg "xiaodou/dai/internal/payment/pg"
)

func TestAdminRechargeOrdersUnifiesPaymentAndBalanceGrants(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("payment test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_admin_recharge', 'Unified Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_admin_recharge', 'tenant_admin_recharge', 'unified-user', 'x', 4, 'active');

		INSERT INTO pay_orders (
			order_id, out_trade_no, scene, tenant_id, user_id, payment_amount_minor,
			gross_amount_micro_usd, credited_amount_micro_usd, tenant_income_micro_usd,
			transaction_id, status, fulfillment_status, paid_at, expires_at, balance_order_id
		) VALUES (
			'PAY_ADMIN_RECHARGE', 'MCH_ADMIN_RECHARGE', 'user_topup',
			'tenant_admin_recharge', 'user_admin_recharge', 100,
			1000000, 1200000, 900000, 'WX_ADMIN_RECHARGE',
			'paid', 'credited', now(), now() + interval '1 hour', 'ORD_ADMIN_RECHARGE_USER'
		);
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, user_id, credit_amount, paid_amount,
			payment_ref, payment_order_id, operator_id, note
		) VALUES
			('ORD_ADMIN_RECHARGE_USER', 'online_user_topup', 'tenant_admin_recharge',
			 'user_admin_recharge', 1200000, 100, 'WX_ADMIN_RECHARGE', 'PAY_ADMIN_RECHARGE',
			 'system:wechatpay', 'online primary'),
			('ORD_ADMIN_RECHARGE_INCOME', 'user_topup_income', 'tenant_admin_recharge',
			 NULL, 900000, 100, 'WX_ADMIN_RECHARGE', 'PAY_ADMIN_RECHARGE',
			 'system:wechatpay', 'tenant income'),
			('ORD_ADMIN_RECHARGE_MANUAL', 'platform_to_tenant', 'tenant_admin_recharge',
			 NULL, 500000, 0, NULL, NULL, 'admin-1', 'manual grant');
		UPDATE bill_recharge_orders
		SET status = 'reversed', reversed_amount_micro = 300000, lost_amount_micro = 200000,
		    reversed_at = now(), reversed_by = 'admin-1', reversal_reason = 'manual correction'
		WHERE order_id = 'ORD_ADMIN_RECHARGE_MANUAL';
	`); err != nil {
		t.Fatalf("seed unified recharge orders: %v", err)
	}

	items, total, err := paymentpg.ListAdminRechargeOrders(ctx, pool, paymentpg.ListAdminRechargeOrdersParams{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("list unified recharge orders: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("unified orders = %d/%d, want 2/2", len(items), total)
	}

	var onlineFound, manualFound bool
	for _, item := range items {
		switch item.OrderID {
		case "PAY_ADMIN_RECHARGE":
			onlineFound = item.Method == "online" && item.BalanceOrderID == "ORD_ADMIN_RECHARGE_USER" &&
				item.OutTradeNo == "MCH_ADMIN_RECHARGE" && item.TransactionID == "WX_ADMIN_RECHARGE"
		case "ORD_ADMIN_RECHARGE_MANUAL":
			manualFound = item.Method == "manual" && item.PaymentStatus == "not_required" && item.FulfillmentStatus == "partially_reversed"
		}
	}
	if !onlineFound || !manualFound {
		t.Fatalf("missing expected unified projections: online=%v manual=%v items=%+v", onlineFound, manualFound, items)
	}

	detail, err := paymentpg.GetAdminRechargeOrder(ctx, pool, "PAY_ADMIN_RECHARGE")
	if err != nil {
		t.Fatalf("get unified recharge detail: %v", err)
	}
	if len(detail.Credits) != 2 || !detail.Credits[0].Primary {
		t.Fatalf("linked credits = %+v, want primary user grant followed by tenant income", detail.Credits)
	}
}

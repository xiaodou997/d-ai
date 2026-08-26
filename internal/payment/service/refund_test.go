package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/dbtest"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/payment"
	paymentpg "xiaodou/dai/internal/payment/pg"
)

func TestRecordCompletedRefundReversesUserAndTenantIncomeExactlyOnce(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("payment test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_refund', 'Refund Tenant', 'active')
	`); err != nil {
		t.Fatalf("seed refund tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_refund', 'tenant_refund', 'refund-user', 'x', 4, 'active')
	`); err != nil {
		t.Fatalf("seed refund user: %v", err)
	}
	paidAt := time.Now().UTC().Add(-time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO pay_orders (
			order_id, out_trade_no, scene, tenant_id, user_id, payment_amount_minor,
			gross_amount_micro_usd, credited_amount_micro_usd, tenant_income_micro_usd,
			transaction_id, status, fulfillment_status, paid_at, expires_at, balance_order_id
		) VALUES (
			'PAY_REFUND', 'MCH_REFUND', 'user_topup', 'tenant_refund', 'user_refund', 100,
			1200000, 1200000, 900000, 'WX_PAY_REFUND', 'paid', 'credited', $1,
			$2, 'ORD_REFUND_USER'
		)
	`, paidAt, paidAt.Add(time.Hour)); err != nil {
		t.Fatalf("seed completed payment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, user_id, credit_amount, paid_amount,
			payment_ref, payment_order_id, operator_id
		) VALUES
			('ORD_REFUND_USER', 'online_user_topup', 'tenant_refund', 'user_refund',
			 1200000, 100, 'WX_PAY_REFUND', 'PAY_REFUND', 'system:wechatpay'),
			('ORD_REFUND_INCOME', 'user_topup_income', 'tenant_refund', NULL,
			 900000, 100, 'WX_PAY_REFUND', 'PAY_REFUND', 'system:wechatpay')
	`); err != nil {
		t.Fatalf("seed linked recharge orders: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_credit_lots (
			lot_id, account_id, granted_micro, consumed_micro, source, recharge_order_id
		) VALUES
			('LOT_REFUND_USER', 'user_refund', 1000000, 400000, 'ONLINE_TOPUP', 'ORD_REFUND_USER'),
			('LOT_REFUND_INCOME', 'tenant_refund', 900000, 300000, 'USER_TOPUP_INCOME', 'ORD_REFUND_INCOME')
	`); err != nil {
		t.Fatalf("seed refund credit lots: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bill_accounts
		SET balance_micro = 600000
		WHERE account_id IN ('user_refund', 'tenant_refund')
	`); err != nil {
		t.Fatalf("seed refund account balances: %v", err)
	}

	svc := &PaymentService{pool: pool, logger: zap.NewNop()}
	params := RecordCompletedRefundParams{
		PaymentOrderID: "PAY_REFUND", Method: payment.RefundMethodWechat,
		RefundReference: "MCH_REFUND_001", ChannelRefundID: "WX_REFUND_001",
		RefundedAt: paidAt.Add(30 * time.Minute), Reason: "customer refund", Note: "verified",
		OperatorID: "admin-1",
	}
	refund, err := svc.RecordCompletedRefund(ctx, params)
	if err != nil {
		t.Fatalf("record completed refund: %v", err)
	}
	if refund.RefundAmountMinor != 100 || refund.Status != "completed" {
		t.Fatalf("unexpected refund: %+v", refund)
	}

	var paymentStatus, fulfillmentStatus, refundStatus string
	if err := pool.QueryRow(ctx, `SELECT status, fulfillment_status, refund_status FROM pay_orders WHERE order_id = 'PAY_REFUND'`).Scan(&paymentStatus, &fulfillmentStatus, &refundStatus); err != nil {
		t.Fatalf("read payment state: %v", err)
	}
	var userBalance, tenantBalance int64
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_refund'`).Scan(&userBalance); err != nil {
		t.Fatalf("read user balance: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'tenant_refund'`).Scan(&tenantBalance); err != nil {
		t.Fatalf("read tenant balance: %v", err)
	}
	if paymentStatus != "paid" || fulfillmentStatus != "reversed" || refundStatus != "refunded" || userBalance != -600000 || tenantBalance != -300000 {
		t.Fatalf("payment/fulfillment/refund/user/tenant = %s/%s/%s/%d/%d", paymentStatus, fulfillmentStatus, refundStatus, userBalance, tenantBalance)
	}

	var effects, reversedOrders, cashEntries int
	var cashAmount, cashBalance int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_refund_reversal_effects WHERE refund_id = $1`, refund.RefundID).Scan(&effects); err != nil {
		t.Fatalf("count reversal effects: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_recharge_orders WHERE payment_order_id = 'PAY_REFUND' AND status = 'reversed'`).Scan(&reversedOrders); err != nil {
		t.Fatalf("count reversed recharge orders: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(amount_micro_usd), 0), COALESCE(MIN(balance_after_micro_usd), 0)
		FROM pay_cash_ledger WHERE ref_type = 'refund' AND ref_id = $1
	`, refund.RefundID).Scan(&cashEntries, &cashAmount, &cashBalance); err != nil {
		t.Fatalf("read refund cash ledger: %v", err)
	}
	if effects != 2 || reversedOrders != 2 || cashEntries != 1 || cashAmount != -900000 || cashBalance != -300000 {
		t.Fatalf("effects/orders/cash/amount/balance = %d/%d/%d/%d/%d", effects, reversedOrders, cashEntries, cashAmount, cashBalance)
	}
	var repairAction, repairKey string
	var beforeState, afterState []byte
	if err := pool.QueryRow(ctx, `
		SELECT action, idempotency_key, before_state, after_state FROM bill_repair_audits
		WHERE target_type = 'pay_orders' AND target_id = 'PAY_REFUND'
	`).Scan(&repairAction, &repairKey, &beforeState, &afterState); err != nil {
		t.Fatalf("read payment repair evidence: %v", err)
	}
	if repairAction != "payment_refund" || repairKey != "payment-refund:PAY_REFUND" {
		t.Fatalf("payment repair evidence = %s/%s", repairAction, repairKey)
	}
	var before, after map[string]any
	if err := json.Unmarshal(beforeState, &before); err != nil {
		t.Fatalf("decode payment repair before state: %v", err)
	}
	if err := json.Unmarshal(afterState, &after); err != nil {
		t.Fatalf("decode payment repair after state: %v", err)
	}
	if before["refund_status"] != "none" || after["refund_status"] != "refunded" {
		t.Fatalf("payment repair before/after state = %#v -> %#v", before, after)
	}
	detail, err := paymentpg.GetAdminRechargeOrder(ctx, pool, "PAY_REFUND")
	if err != nil {
		t.Fatalf("get refunded admin recharge detail: %v", err)
	}
	if detail.RefundStatus != payment.RefundStatusRefunded || detail.Refund == nil || detail.Refund.RefundID != refund.RefundID || len(detail.Credits) != 2 {
		t.Fatalf("unexpected refunded admin detail: %+v", detail)
	}
	if detail.Credits[0].RefundAccountDebitMicroUSD != 1200000 || detail.Credits[1].RefundAccountDebitMicroUSD != 900000 {
		t.Fatalf("unexpected detail reversal effects: %+v", detail.Credits)
	}

	if _, err := svc.RecordCompletedRefund(ctx, params); err != nil {
		t.Fatalf("idempotent refund retry: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_refund'`).Scan(&userBalance); err != nil || userBalance != -600000 {
		t.Fatalf("idempotent retry changed user balance: %d, err=%v", userBalance, err)
	}
	params.RefundReference = "MCH_REFUND_DIFFERENT"
	if _, err := svc.RecordCompletedRefund(ctx, params); !errors.Is(err, domain.ErrPaymentAlreadyRefunded) {
		t.Fatalf("different duplicate refund error = %v, want already refunded", err)
	}
}

func TestRecordCompletedRefundDoesNotDoubleDebitExpiredCredit(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("payment test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_expired_refund', 'Expired Refund Tenant', 'active')
	`); err != nil {
		t.Fatalf("seed expired refund tenant: %v", err)
	}
	paidAt := time.Now().UTC().Add(-48 * time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO pay_orders (
			order_id, out_trade_no, scene, tenant_id, payment_amount_minor,
			gross_amount_micro_usd, credited_amount_micro_usd, transaction_id,
			status, fulfillment_status, paid_at, expires_at, balance_order_id
		) VALUES (
			'PAY_EXPIRED_REFUND', 'MCH_EXPIRED_REFUND', 'tenant_topup',
			'tenant_expired_refund', 100, 1000000, 1000000, 'WX_EXPIRED_REFUND',
			'paid', 'credited', $1, $2, 'ORD_EXPIRED_REFUND'
		)
	`, paidAt, paidAt.Add(time.Hour)); err != nil {
		t.Fatalf("seed expired payment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, credit_amount, paid_amount,
			payment_ref, payment_order_id, operator_id
		) VALUES (
			'ORD_EXPIRED_REFUND', 'online_tenant_topup', 'tenant_expired_refund',
			1000000, 100, 'WX_EXPIRED_REFUND', 'PAY_EXPIRED_REFUND', 'system:wechatpay'
		)
	`); err != nil {
		t.Fatalf("seed expired recharge order: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_credit_lots (
			lot_id, account_id, granted_micro, consumed_micro, expired_at,
			expired_unused_micro, source, recharge_order_id
		) VALUES (
			'LOT_EXPIRED_REFUND', 'tenant_expired_refund', 1000000, 200000, now(),
			800000, 'ONLINE_TOPUP', 'ORD_EXPIRED_REFUND'
		)
	`); err != nil {
		t.Fatalf("seed expired credit lot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bill_accounts SET balance_micro = 0 WHERE account_id = 'tenant_expired_refund'
	`); err != nil {
		t.Fatalf("seed expired refund account balance: %v", err)
	}

	svc := &PaymentService{pool: pool, logger: zap.NewNop()}
	refund, err := svc.RecordCompletedRefund(ctx, RecordCompletedRefundParams{
		PaymentOrderID: "PAY_EXPIRED_REFUND", Method: payment.RefundMethodOffline,
		RefundReference: "OFFLINE_EXPIRED_001", RefundedAt: paidAt.Add(24 * time.Hour),
		Reason: "offline full refund", OperatorID: "admin-1",
	})
	if err != nil {
		t.Fatalf("refund expired credit: %v", err)
	}
	var balance, debit, nonAvailable, expired int64
	if err := pool.QueryRow(ctx, `
		SELECT e.balance_after_micro, e.account_debit_micro,
		       e.non_available_debit_micro, e.expired_amount_micro
		FROM bill_refund_reversal_effects e WHERE e.refund_id = $1
	`, refund.RefundID).Scan(&balance, &debit, &nonAvailable, &expired); err != nil {
		t.Fatalf("read expired reversal effect: %v", err)
	}
	if balance != -200000 || debit != 200000 || nonAvailable != 200000 || expired != 800000 {
		t.Fatalf("expired reversal balance/debit/nonavailable/expired = %d/%d/%d/%d", balance, debit, nonAvailable, expired)
	}
}

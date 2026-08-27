package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/ledger"
	"xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/dbtest"
	shared "xiaodou/dai/internal/domain"
)

func TestDeductionCommandsHonorCanceledContext(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	deduction := service.NewDeductionService(pool, zap.NewNop())

	if err := deduction.RefundUsage(canceled, "missing", "canceled", "test"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled usage refund error = %v, want context.Canceled", err)
	}
	if _, err := deduction.ReverseOrder(canceled, "missing", "canceled", "test"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled recharge reversal error = %v, want context.Canceled", err)
	}
	result := deduction.BatchRefundUsage(canceled, []string{"one", "two"}, "canceled", "test")
	if len(result.Succeeded) != 0 || len(result.Failed) != 2 {
		t.Fatalf("canceled batch result = %+v, want two failures and no successes", result)
	}
	for _, failure := range result.Failed {
		if failure.Reason != context.Canceled.Error() {
			t.Fatalf("canceled batch failure = %+v, want context canceled", failure)
		}
	}
}

func TestRefundUsageCreditsAccountsOnceAndAuditsUsage(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_refund', 'Refund Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_refund', 'tenant_refund', 'refund-user', 'x', 4, 'active');
		INSERT INTO ai_usage_logs (
			request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
			model_code, requested_model, capability_type, billable_unit_type,
			tenant_payable, user_payable, user_charged, billing_status, request_status,
			client_protocol, billing_source, settled_at
		) VALUES (
			'req-refund', 'user', 'jwt', 'web_chat', 'tenant_refund', 'user_refund',
			'model', 'model', 'chat', 'token', 100, 200, 200, 'settled', 'success',
			'openai_chat', 'payg', now()
		)
	`); err != nil {
		t.Fatalf("seed refund usage: %v", err)
	}

	grantRefundBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: "tenant_refund", TenantID: "tenant_refund"}, 1000)
	grantRefundBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: "user_refund", TenantID: "tenant_refund"}, 2000)

	deduction := service.NewDeductionService(pool, zap.NewNop())
	if err := deduction.RefundUsage(ctx, "req-refund", "operator correction", "admin-refund"); err != nil {
		t.Fatalf("refund usage: %v", err)
	}

	var tenantBalance, userBalance int64
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'tenant_refund'`).Scan(&tenantBalance); err != nil {
		t.Fatalf("read tenant balance: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_refund'`).Scan(&userBalance); err != nil {
		t.Fatalf("read user balance: %v", err)
	}
	if tenantBalance != 1100 || userBalance != 2200 {
		t.Fatalf("refunded balances = %d/%d, want 1100/2200", tenantBalance, userBalance)
	}

	var refundStatus, reason, operator string
	var refundedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT refund_status, refund_reason, refund_operator_id, refunded_at
		FROM ai_usage_logs WHERE request_id = 'req-refund'
	`).Scan(&refundStatus, &reason, &operator, &refundedAt); err != nil {
		t.Fatalf("read usage refund audit: %v", err)
	}
	if refundStatus != "refunded" || reason != "operator correction" || operator != "admin-refund" || refundedAt == nil {
		t.Fatalf("usage refund audit = %s/%s/%s/%v", refundStatus, reason, operator, refundedAt)
	}
	var repairAction, repairKey string
	var beforeState, afterState []byte
	var before, after map[string]any
	if err := pool.QueryRow(ctx, `
		SELECT action, idempotency_key, before_state, after_state FROM bill_repair_audits
		WHERE target_type = 'ai_usage_logs' AND target_id = 'req-refund'
	`).Scan(&repairAction, &repairKey, &beforeState, &afterState); err != nil {
		t.Fatalf("read usage repair evidence: %v", err)
	}
	if repairAction != "usage_refund" || repairKey != "usage-refund:req-refund" {
		t.Fatalf("usage repair evidence = %s/%s", repairAction, repairKey)
	}
	if err := json.Unmarshal(beforeState, &before); err != nil {
		t.Fatalf("decode usage repair before state: %v", err)
	}
	if err := json.Unmarshal(afterState, &after); err != nil {
		t.Fatalf("decode usage repair after state: %v", err)
	}
	if before["refund_status"] != "none" || after["refund_status"] != "refunded" {
		t.Fatalf("usage repair before/after state = %#v -> %#v", before, after)
	}

	if err := deduction.RefundUsage(ctx, "req-refund", "duplicate", "admin-refund"); err == nil {
		t.Fatal("duplicate usage refund succeeded")
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_refund'`).Scan(&userBalance); err != nil {
		t.Fatalf("read user balance after duplicate: %v", err)
	}
	if userBalance != 2200 {
		t.Fatalf("duplicate refund changed user balance to %d", userBalance)
	}
}

func grantRefundBalance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ref ledger.Ref, amount int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin grant: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := ledger.Grant(ctx, tx, ref, amount, nil, billing.PackageSourceAdminRecharge, ""); err != nil {
		t.Fatalf("grant refund balance: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit grant: %v", err)
	}
}

func TestReverseOnlineUserCreditRequiresRefundWorkflow(t *testing.T) {
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

	if _, err := service.NewDeductionService(pool, zap.NewNop()).ReverseOrder(ctx, "ORD_REVERSE_USER", "risk review", "admin-1"); err == nil {
		t.Fatal("online recharge was reversed without completed refund workflow")
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
	if paymentStatus != "paid" || fulfillmentStatus != "credited" || primaryStatus != "active" || incomeStatus != "active" || userBalance != 600000 || tenantBalance != 900000 || reversedAmount != 0 || lostAmount != 0 {
		t.Fatalf("states payment/fulfillment/primary/income/user/tenant/reversed/lost = %s/%s/%s/%s/%d/%d/%d/%d", paymentStatus, fulfillmentStatus, primaryStatus, incomeStatus, userBalance, tenantBalance, reversedAmount, lostAmount)
	}
}

func TestReverseTenantOrderEnforcesScopeInsideReversalTransaction(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES
			('tenant_scope_a', 'Scope Tenant A', 'active'),
			('tenant_scope_b', 'Scope Tenant B', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_scope_a', 'tenant_scope_a', 'scope-user-a', 'x', 4, 'active');
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, user_id, credit_amount, paid_amount, operator_id
		) VALUES
			('ORD_SCOPE_USER', 'tenant_to_user', 'tenant_scope_a', 'user_scope_a', 1000, 0, 'tenant-a'),
			('ORD_SCOPE_TENANT', 'platform_to_tenant', 'tenant_scope_a', NULL, 1000, 0, 'admin');
	`); err != nil {
		t.Fatalf("seed tenant-scoped recharge orders: %v", err)
	}
	grantTenantRechargeBalance(t, ctx, pool, "user_scope_a", "tenant_scope_a", "ORD_SCOPE_USER", 1000)

	deduction := service.NewDeductionService(pool, zap.NewNop())
	if _, err := deduction.ReverseTenantOrder(ctx, "ORD_SCOPE_USER", "tenant_scope_b", "wrong tenant", "tenant-b"); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("cross-tenant reversal error = %v, want ErrForbidden", err)
	}
	if _, err := deduction.ReverseTenantOrder(ctx, "ORD_SCOPE_TENANT", "tenant_scope_a", "wrong order type", "tenant-a"); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("platform order reversal error = %v, want ErrForbidden", err)
	}

	var status string
	var balance int64
	if err := pool.QueryRow(ctx, `SELECT status FROM bill_recharge_orders WHERE order_id = 'ORD_SCOPE_USER'`).Scan(&status); err != nil {
		t.Fatalf("read order after forbidden reversal: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_scope_a'`).Scan(&balance); err != nil {
		t.Fatalf("read balance after forbidden reversal: %v", err)
	}
	if status != billing.OrderStatusActive || balance != 1000 {
		t.Fatalf("forbidden reversal changed order/balance to %s/%d", status, balance)
	}

	result, err := deduction.ReverseTenantOrder(ctx, "ORD_SCOPE_USER", "tenant_scope_a", "approved", "tenant-a")
	if err != nil {
		t.Fatalf("same-tenant reversal: %v", err)
	}
	if result.ReversedCredits != 1000 || result.LostCredits != 0 || result.IsPartial {
		t.Fatalf("reversal result = %+v", result)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM bill_recharge_orders WHERE order_id = 'ORD_SCOPE_USER'`).Scan(&status); err != nil {
		t.Fatalf("read reversed order: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_scope_a'`).Scan(&balance); err != nil {
		t.Fatalf("read reversed balance: %v", err)
	}
	if status != billing.OrderStatusReversed || balance != 0 {
		t.Fatalf("same-tenant reversal state = %s/%d", status, balance)
	}
	var repairAction, repairKey string
	var beforeState, afterState []byte
	var before, after map[string]any
	if err := pool.QueryRow(ctx, `
		SELECT action, idempotency_key, before_state, after_state FROM bill_repair_audits
		WHERE target_type = 'bill_recharge_orders' AND target_id = 'ORD_SCOPE_USER'
	`).Scan(&repairAction, &repairKey, &beforeState, &afterState); err != nil {
		t.Fatalf("read recharge repair evidence: %v", err)
	}
	if repairAction != "recharge_reversal" || repairKey != "recharge-reversal:ORD_SCOPE_USER" {
		t.Fatalf("recharge repair evidence = %s/%s", repairAction, repairKey)
	}
	if err := json.Unmarshal(beforeState, &before); err != nil {
		t.Fatalf("decode recharge repair before state: %v", err)
	}
	if err := json.Unmarshal(afterState, &after); err != nil {
		t.Fatalf("decode recharge repair after state: %v", err)
	}
	if before["status"] != "active" || after["status"] != "reversed" {
		t.Fatalf("recharge repair before/after state = %#v -> %#v", before, after)
	}
}

func grantTenantRechargeBalance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID, tenantID, orderID string, amount int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tenant recharge grant: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := ledger.Grant(ctx, tx, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, amount, nil, billing.PackageSourceTenantRecharge, orderID); err != nil {
		t.Fatalf("grant tenant recharge balance: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tenant recharge grant: %v", err)
	}
}

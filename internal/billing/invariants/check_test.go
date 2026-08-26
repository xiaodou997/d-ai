package invariants_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/subscription"
	billingdomain "xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/invariants"
	"xiaodou/dai/internal/billing/ledger"
	"xiaodou/dai/internal/billing/outbox"
	billingservice "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/dbtest"
)

func TestMoneyInvariantSuiteCoversCrossModuleLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 8})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const tenantID = "invariant-tenant"
	const userID = "invariant-user"
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, 'Invariant Tenant', 'active')
	`, tenantID); err != nil {
		t.Fatalf("seed identity/orders: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, 'invariant-user', 'hash', 4, 'active')
	`, userID, tenantID); err != nil {
		t.Fatalf("seed invariant user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_recharge_orders
			(order_id, order_type, tenant_id, credit_amount, paid_amount, operator_id)
		VALUES ('INV_TENANT_ORDER', 'platform_to_tenant', $1, 1000, 1000, 'invariant-test')
	`, tenantID); err != nil {
		t.Fatalf("seed invariant recharge orders: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_recharge_orders
			(order_id, order_type, tenant_id, user_id, credit_amount, paid_amount, operator_id)
		VALUES ('INV_USER_ORDER', 'tenant_to_user', $1, $2, 800, 800, 'invariant-test')
	`, tenantID, userID); err != nil {
		t.Fatalf("seed invariant user recharge order: %v", err)
	}

	grant := func(ref ledger.Ref, amount int64, expiresAt *time.Time, source, orderID string) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin grant: %v", err)
		}
		defer tx.Rollback(ctx)
		if _, err := ledger.Grant(ctx, tx, ref, amount, expiresAt, source, orderID); err != nil {
			t.Fatalf("grant %s: %v", ref.ID, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit grant %s: %v", ref.ID, err)
		}
	}
	checkHealthy := func(stage string) {
		t.Helper()
		report, err := invariants.Check(ctx, pool)
		if err != nil {
			t.Fatalf("%s invariant query: %v", stage, err)
		}
		if err := report.Err(); err != nil {
			t.Fatalf("%s invariants: %v (report=%+v)", stage, err, report)
		}
	}

	grant(ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, 1000, nil, billingdomain.PackageSourceAdminRecharge, "INV_TENANT_ORDER")
	grant(ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 800, nil, billingdomain.PackageSourceTenantRecharge, "INV_USER_ORDER")
	checkHealthy("after grants")

	// Expiry is a real balance mutation, not a read-time filter.
	past := time.Now().UTC().Add(-time.Hour)
	grant(ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, 100, &past, billingdomain.PackageSourceAdminRecharge, "")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if expired, err := ledger.ExpireDueLots(ctx, tx, time.Now().UTC(), 100); err != nil || expired != 1 {
		t.Fatalf("expire due lots = %d, err=%v", expired, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	checkHealthy("after expiry")

	// A post-paid charge may drive the signed balance negative, while a
	// pre-paid recharge reversal can only reclaim the lot's unspent remainder.
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Charge(ctx, tx, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 300); err != nil {
		t.Fatalf("charge user: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	checkHealthy("after charge")

	if _, err := billingservice.NewDeductionService(pool, zap.NewNop()).ReverseOrder("INV_USER_ORDER", "invariant reversal", "invariant-test"); err != nil {
		t.Fatalf("reverse recharge: %v", err)
	}
	checkHealthy("after recharge reversal")

	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
			 model_code, billable_unit_type, tenant_payable, user_payable, user_charged,
			 billing_status, request_status, client_protocol, billing_source)
		VALUES ('INV_REFUND_USAGE', 'user', 'jwt', 'invariant-test', $1, $2,
			 'invariant-model', 'token', 100, 200, 200,
			 'settled', 'success', 'openai_chat', 'payg')
	`, tenantID, userID); err != nil {
		t.Fatalf("seed refundable usage: %v", err)
	}
	if err := billingservice.NewDeductionService(pool, zap.NewNop()).RefundUsage("INV_REFUND_USAGE", "invariant refund", "invariant-test"); err != nil {
		t.Fatalf("refund usage: %v", err)
	}
	checkHealthy("after usage refund")

	const orderNo = "INV_SUB_ORDER"
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_sub_orders
			(order_no, tenant_id, user_id, plan_id, plan_name_snapshot, price_micro_usd,
			 duration_days_snapshot, total_limit_micro_snapshot,
			 group_quota_debit_multipliers_snapshot, purchase_policy_version,
			 purchase_policy_snapshot, status)
		VALUES ($1, $2, $3, gen_random_uuid(), 'Invariant Plan', 150,
			 7, 1000, '{"invariant": 1}'::jsonb, 1, '{}'::jsonb, 'created')
	`, orderNo, tenantID, userID); err != nil {
		t.Fatalf("seed subscription order: %v", err)
	}
	if _, err := subscription.NewBillingPurchaser(pool, "dai").DebitStrict(ctx, subscription.DebitRequest{
		IdempotencyKey: "ai-sub-" + orderNo,
		TenantID:       tenantID,
		UserID:         userID,
		UserMicro:      150,
	}); err != nil {
		t.Fatalf("subscription debit: %v", err)
	}
	checkHealthy("after subscription debit")

	// The request usage row and its outbox charge are committed together; the
	// checker must accept both pending and settled forms of that pair.
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
			 model_code, billable_unit_type, tenant_payable, user_payable, user_charged,
			 billing_status, request_status, client_protocol, billing_source)
		VALUES ('INV_OUTBOX_USAGE', 'user', 'jwt', 'invariant-test', $1, $2,
			 'invariant-model', 'token', 50, 25, 25,
			 'pending', 'success', 'openai_chat', 'payg')
	`, tenantID, userID); err != nil {
		t.Fatalf("seed outbox usage: %v", err)
	}
	if err := outbox.Enqueue(ctx, tx, outbox.Charge{
		RequestID: "INV_OUTBOX_USAGE", TenantID: tenantID, UserID: userID,
		TenantMicro: 50, UserMicro: 25, Description: "invariant outbox",
	}); err != nil {
		t.Fatalf("enqueue outbox charge: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	checkHealthy("with pending outbox")

	if count, err := outbox.NewConsumer(pool, zap.NewNop()).DrainOnce(ctx); err != nil || count != 1 {
		t.Fatalf("drain outbox = %d, err=%v", count, err)
	}
	checkHealthy("after outbox settlement")

	// The checker must fail loudly on a broken balance/lot relationship, then
	// return healthy again once the test restores the row.
	if _, err := pool.Exec(ctx, `UPDATE bill_accounts SET balance_micro = balance_micro + 1 WHERE account_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	report, err := invariants.Check(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy() {
		t.Fatalf("checker missed deliberate balance corruption: %+v", report)
	}
	if _, err := pool.Exec(ctx, `UPDATE bill_accounts SET balance_micro = balance_micro - 1 WHERE account_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	checkHealthy("after corruption repair")
}

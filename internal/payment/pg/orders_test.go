package pg_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"xiaodou/dai/internal/dbtest"
	"xiaodou/dai/internal/money"
	"xiaodou/dai/internal/payment"
	paymentpg "xiaodou/dai/internal/payment/pg"
)

func TestListOrdersIncludesTenantAndUserNames(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("payment test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "tenant_" + suffix
	userID := "user_" + suffix
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status) VALUES ($1, '测试租户', 'active')
	`, tenantID); err != nil {
		t.Fatalf("seed payment tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, '测试用户', 'x', 4, 'active')
	`, userID, tenantID); err != nil {
		t.Fatalf("seed payment user: %v", err)
	}

	order := &payment.Order{
		OrderID: "PAY_" + suffix, OutTradeNo: "OUT_" + suffix,
		Scene: payment.SceneUserTopup, TenantID: tenantID, UserID: userID,
		TopupMode: "custom", PaymentCurrency: money.CurrencyUSD, PaymentAmountMinor: 100,
		LedgerCurrency: money.CurrencyUSD, GrossAmountMicroUSD: 1_000_000,
		CreditedAmountMicroUSD: 1_000_000, TenantIncomeMicroUSD: 1_000_000,
		Channel: "wechat_native", Status: payment.OrderStatusCreated, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := paymentpg.InsertOrder(ctx, pool, order); err != nil {
		t.Fatalf("insert payment order: %v", err)
	}

	orders, total, err := paymentpg.ListOrders(ctx, pool, paymentpg.ListOrdersParams{TenantID: tenantID, Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("list payment orders: %v", err)
	}
	if total != 1 || len(orders) != 1 {
		t.Fatalf("orders = %d/%d, want 1/1", len(orders), total)
	}
	if orders[0].TenantName != "测试租户" || orders[0].Username != "测试用户" {
		t.Fatalf("payment owner names = %q/%q", orders[0].TenantName, orders[0].Username)
	}
}

func TestUpdateStatusIfCurrentDoesNotOverwriteConcurrentPaidTransition(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("payment test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	orderID := "PAY_SWEEP_" + suffix
	if err := paymentpg.InsertOrder(ctx, pool, &payment.Order{
		OrderID: orderID, OutTradeNo: "OUT_SWEEP_" + suffix, Scene: payment.SceneTenantTopup,
		TenantID: "tenant-sweep-transition", TopupMode: "custom", PaymentCurrency: money.CurrencyUSD,
		PaymentAmountMinor: 100, LedgerCurrency: money.CurrencyUSD, GrossAmountMicroUSD: 1_000_000,
		CreditedAmountMicroUSD: 1_000_000, Channel: "wechat_native", Status: payment.OrderStatusCreated,
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed sweep transition order: %v", err)
	}

	updated, err := paymentpg.UpdateStatusIfCurrent(ctx, pool, orderID, payment.OrderStatusCreated, payment.OrderStatusClosed, "")
	if err != nil || !updated {
		t.Fatalf("first sweep transition = updated:%v err:%v", updated, err)
	}
	updated, err = paymentpg.UpdateStatusIfCurrent(ctx, pool, orderID, payment.OrderStatusCreated, payment.OrderStatusExpired, "late close failure")
	if err != nil || updated {
		t.Fatalf("stale sweep transition = updated:%v err:%v", updated, err)
	}
	var status, failNote string
	if err := pool.QueryRow(ctx, `SELECT status, COALESCE(fail_note, '') FROM pay_orders WHERE order_id = $1`, orderID).Scan(&status, &failNote); err != nil {
		t.Fatalf("read sweep transition order: %v", err)
	}
	if status != payment.OrderStatusClosed || failNote != "" {
		t.Fatalf("stale sweep transition changed order to %s/%q", status, failNote)
	}
}

func TestSweepRetryStatePersistsBackoffAndFencesStaleResults(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("payment test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	orderID := "PAY_SWEEP_BACKOFF_" + suffix
	if err := paymentpg.InsertOrder(ctx, pool, &payment.Order{
		OrderID: orderID, OutTradeNo: "OUT_SWEEP_BACKOFF_" + suffix, Scene: payment.SceneTenantTopup,
		TenantID: "tenant-sweep-backoff", TopupMode: "custom", PaymentCurrency: money.CurrencyUSD,
		PaymentAmountMinor: 100, LedgerCurrency: money.CurrencyUSD, GrossAmountMicroUSD: 1_000_000,
		CreditedAmountMicroUSD: 1_000_000, Channel: "wechat_native", Status: payment.OrderStatusCreated,
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed sweep backoff order: %v", err)
	}

	now := time.Now().UTC()
	next := now.Add(10 * time.Minute)
	updated, err := paymentpg.RecordSweepFailureIfCurrent(ctx, pool, orderID, payment.OrderStatusCreated, payment.OrderStatusExpired, next, "wechat unavailable")
	if err != nil || !updated {
		t.Fatalf("record sweep failure = updated:%v err:%v", updated, err)
	}
	order, err := paymentpg.GetOrderByID(ctx, pool, orderID)
	if err != nil {
		t.Fatalf("read sweep backoff order: %v", err)
	}
	if order.Status != payment.OrderStatusExpired || order.SweepAttempts != 1 || order.SweepNextAttemptAt == nil || order.SweepLastAttemptAt == nil || order.SweepLastError != "wechat unavailable" {
		t.Fatalf("sweep retry state = status:%s attempts:%d next:%v last:%v error:%q", order.Status, order.SweepAttempts, order.SweepNextAttemptAt, order.SweepLastAttemptAt, order.SweepLastError)
	}

	candidates, err := paymentpg.ListSweepCandidates(ctx, pool, now, 10)
	if err != nil {
		t.Fatalf("list deferred sweep candidates: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("deferred candidates = %d, want 0", len(candidates))
	}
	candidates, err = paymentpg.ListSweepCandidates(ctx, pool, next.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("list due sweep candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].SweepAttempts != 1 {
		t.Fatalf("due candidates = %+v, want one attempt 1", candidates)
	}

	updated, err = paymentpg.RecordSweepFailureIfCurrent(ctx, pool, orderID, payment.OrderStatusCreated, payment.OrderStatusExpired, next.Add(time.Hour), "stale result")
	if err != nil || updated {
		t.Fatalf("stale sweep failure = updated:%v err:%v", updated, err)
	}
	updated, err = paymentpg.UpdateStatusIfCurrent(ctx, pool, orderID, payment.OrderStatusExpired, payment.OrderStatusClosed, "")
	if err != nil || !updated {
		t.Fatalf("close retry order = updated:%v err:%v", updated, err)
	}
	order, err = paymentpg.GetOrderByID(ctx, pool, orderID)
	if err != nil {
		t.Fatalf("read closed sweep order: %v", err)
	}
	if order.SweepAttempts != 0 || order.SweepNextAttemptAt != nil || order.SweepLastAttemptAt != nil || order.SweepLastError != "" {
		t.Fatalf("closed order retained retry state = %+v", order)
	}
}

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"xiaodou/dai/internal/dbtest"
	"xiaodou/dai/internal/money"
	"xiaodou/dai/internal/payment"
	paymentpg "xiaodou/dai/internal/payment/pg"
	"xiaodou/dai/internal/payment/wechat"
)

func TestSweepFailureRetryDelayUsesExponentialBackoff(t *testing.T) {
	want := []time.Duration{
		time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		32 * time.Minute,
		time.Hour,
		time.Hour,
	}
	for index, expected := range want {
		attempt := index + 1
		if got := sweepFailureRetryDelay(attempt); got != expected {
			t.Fatalf("attempt %d delay = %s, want %s", attempt, got, expected)
		}
	}
}

type sweepGatewayStub struct {
	query    *wechat.QueryResult
	queryErr error
}

func (s *sweepGatewayStub) Prepay(context.Context, string, int64, time.Time, string) (string, error) {
	return "", nil
}

func (s *sweepGatewayStub) Query(context.Context, string) (*wechat.QueryResult, error) {
	return s.query, s.queryErr
}

func (s *sweepGatewayStub) Close(context.Context, string) error { return nil }

func (s *sweepGatewayStub) ParseNotify(context.Context, *http.Request) (*wechat.QueryResult, error) {
	return nil, nil
}

func (s *sweepGatewayStub) SimulateSuccess(string, int64) {}

func TestSweepInFlightPersistsProviderFailure(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("payment test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	orderID := "PAY_SERVICE_SWEEP_" + suffix
	if err := paymentpg.InsertOrder(ctx, pool, &payment.Order{
		OrderID: orderID, OutTradeNo: "OUT_SERVICE_SWEEP_" + suffix, Scene: payment.SceneTenantTopup,
		TenantID: "tenant-service-sweep", TopupMode: "custom", PaymentCurrency: money.CurrencyUSD,
		PaymentAmountMinor: 100, LedgerCurrency: money.CurrencyUSD, GrossAmountMicroUSD: 1_000_000,
		CreditedAmountMicroUSD: 1_000_000, Channel: "wechat_native", Status: payment.OrderStatusCreated,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC().Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("seed in-flight order: %v", err)
	}

	svc := &PaymentService{
		pool:    pool,
		gateway: &sweepGatewayStub{queryErr: errors.New("wechat unavailable")},
	}
	order, err := paymentpg.GetOrderByID(ctx, pool, orderID)
	if err != nil {
		t.Fatalf("read in-flight order: %v", err)
	}
	if err := svc.sweepInFlightOrder(ctx, order); err == nil {
		t.Fatal("sweep provider failure returned nil")
	}
	order, err = paymentpg.GetOrderByID(ctx, pool, orderID)
	if err != nil {
		t.Fatalf("read persisted sweep failure: %v", err)
	}
	if order.Status != payment.OrderStatusCreated || order.SweepAttempts != 1 || order.SweepNextAttemptAt == nil || order.SweepLastError == "" {
		t.Fatalf("persisted sweep failure = %+v", order)
	}
	if err := svc.publishSweepRetryHealth(ctx); err != nil {
		t.Fatalf("publish sweep retry health: %v", err)
	}
	if got := testutil.ToFloat64(paymentSweepRetryOrders); got != 1 {
		t.Fatalf("retry order metric = %v, want 1", got)
	}
	if got := testutil.ToFloat64(paymentSweepDueRetryOrders); got != 0 {
		t.Fatalf("due retry metric = %v, want 0 before backoff", got)
	}
	if got := testutil.ToFloat64(paymentSweepOldestRetrySeconds); got <= 0 {
		t.Fatalf("oldest retry metric = %v, want positive", got)
	}
}

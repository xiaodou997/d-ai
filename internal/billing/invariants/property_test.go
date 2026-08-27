package invariants_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	billingdomain "xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/invariants"
	"xiaodou/dai/internal/billing/ledger"
	billingservice "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/dbtest"
	shared "xiaodou/dai/internal/domain"
)

// TestRandomConcurrentBillingProperty exercises the invariant after a fixed,
// reproducible random schedule. The schedule deliberately mixes operations
// that take locks in different domains: account charges/grants, due-lot
// expiry, recharge reversal and usage refunds. A fixed seed makes a production
// failure replayable while still exploring a different interleaving from the
// example lifecycle test.
func TestRandomConcurrentBillingProperty(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 32})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		seed     = int64(20260826)
		tenantID = "property-tenant"
		userID   = "property-user"
	)
	if err := seedPropertyFixture(ctx, pool, tenantID, userID); err != nil {
		t.Fatal(err)
	}

	operations := make([]propertyOperation, 0, 80)
	for i := 0; i < 8; i++ {
		orderID := fmt.Sprintf("PROP_GRANT_%02d", i)
		operations = append(operations, propertyOperation{
			name: "grant/" + orderID,
			run: func(ctx context.Context) error {
				return inTx(ctx, pool, func(tx pgx.Tx) error {
					_, err := ledger.Grant(ctx, tx,
						ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID},
						7_000, nil, billingdomain.PackageSourceTenantRecharge, orderID)
					return err
				})
			},
		})
	}
	for i := 0; i < 8; i++ {
		orderID := fmt.Sprintf("PROP_REVOKE_%02d", i)
		operations = append(operations, propertyOperation{
			name: "revoke/" + orderID,
			run: func(ctx context.Context) error {
				_, err := billingservice.NewDeductionService(pool, zap.NewNop()).ReverseOrder(ctx, orderID, "property schedule", "property-test")
				return err
			},
		})
	}
	for i := 0; i < 8; i++ {
		operations = append(operations, propertyOperation{
			name: fmt.Sprintf("expire/%02d", i),
			run: func(ctx context.Context) error {
				return inTx(ctx, pool, func(tx pgx.Tx) error {
					_, err := ledger.ExpireDueLots(ctx, tx, time.Now().UTC(), 1)
					return err
				})
			},
		})
	}
	for i := 0; i < 24; i++ {
		target := ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}
		if i%3 == 0 {
			target = ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}
		}
		amount := int64(100 + (i*791)%600)
		operations = append(operations, propertyOperation{
			name: fmt.Sprintf("charge/%02d", i),
			run: func(ctx context.Context) error {
				return inTx(ctx, pool, func(tx pgx.Tx) error {
					return ledger.Charge(ctx, tx, target, amount)
				})
			},
		})
	}
	for i := 0; i < 8; i++ {
		requestID := fmt.Sprintf("PROP_REFUND_%02d", i)
		operations = append(operations, propertyOperation{
			name: "refund/" + requestID,
			run: func(ctx context.Context) error {
				return billingservice.NewDeductionService(pool, zap.NewNop()).RefundUsage(ctx, requestID, "property refund", "property-test")
			},
		})
	}

	random := rand.New(rand.NewSource(seed))
	random.Shuffle(len(operations), func(i, j int) { operations[i], operations[j] = operations[j], operations[i] })
	t.Logf("random billing operation seed=%d operations=%d", seed, len(operations))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan propertyResult, len(operations))
	var wg sync.WaitGroup
	for _, operation := range operations {
		operation := operation
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-start:
			case <-ctx.Done():
				results <- propertyResult{name: operation.name, err: ctx.Err()}
				return
			}
			results <- propertyResult{name: operation.name, err: operation.run(ctx)}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Errorf("random operation %s failed: %v", result.name, result.err)
		}
	}
	if t.Failed() {
		return
	}

	// Some expiry workers can legitimately observe a row held by another
	// worker because SKIP LOCKED is used. Finish the same operation serially so
	// the property is checked after the whole due-lot set has been reconciled.
	for range 16 {
		var expired int
		if err := inTx(context.Background(), pool, func(tx pgx.Tx) error {
			var err error
			expired, err = ledger.ExpireDueLots(context.Background(), tx, time.Now().UTC(), 100)
			return err
		}); err != nil {
			t.Fatalf("finish due-lot reconciliation: %v", err)
		}
		if expired == 0 {
			break
		}
	}

	assertPropertyHealthy(t, context.Background(), pool, "random concurrent schedule")
	var expiredLots int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM bill_credit_lots
		WHERE account_id = $1 AND expires_at IS NOT NULL AND expired_at IS NOT NULL
	`, tenantID).Scan(&expiredLots); err != nil {
		t.Fatalf("count expired property lots: %v", err)
	}
	if expiredLots != 8 {
		t.Fatalf("expired property lots = %d, want 8", expiredLots)
	}
}

// TestConcurrentFinancialCommandsAreIdempotent checks the two operator
// commands whose retries can otherwise create money: a usage refund and a
// recharge reversal. Every caller races on the same idempotency anchor, while
// the final invariant pass proves that only one balance mutation won.
func TestConcurrentFinancialCommandsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 32})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const tenantID = "idempotency-tenant"
	const userID = "idempotency-user"
	if err := seedIdempotencyFixture(ctx, pool, tenantID, userID); err != nil {
		t.Fatal(err)
	}

	type commandResult struct {
		kind string
		err  error
	}
	const attempts = 16
	start := make(chan struct{})
	results := make(chan commandResult, attempts*2)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			results <- commandResult{kind: "refund", err: billingservice.NewDeductionService(pool, zap.NewNop()).RefundUsage(ctx, "IDEMP_USAGE", "idempotent refund", "property-test")}
		}()
		go func() {
			defer wg.Done()
			<-start
			_, err := billingservice.NewDeductionService(pool, zap.NewNop()).ReverseOrder(ctx, "IDEMP_ORDER", "idempotent reversal", "property-test")
			results <- commandResult{kind: "reversal", err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var refunds, reversals int
	for result := range results {
		switch result.kind {
		case "refund":
			if result.err == nil {
				refunds++
				continue
			}
			if !strings.Contains(result.err.Error(), "already refunded") {
				t.Errorf("unexpected concurrent refund error: %v", result.err)
			}
		case "reversal":
			if result.err == nil {
				reversals++
				continue
			}
			if !errors.Is(result.err, shared.ErrRechargeAlreadyReversed) {
				t.Errorf("unexpected concurrent reversal error: %v", result.err)
			}
		}
	}
	if refunds != 1 || reversals != 1 {
		t.Fatalf("idempotent winners = refunds:%d reversals:%d, want one each", refunds, reversals)
	}

	var userBalance int64
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = $1`, userID).Scan(&userBalance); err != nil {
		t.Fatalf("read idempotent balance: %v", err)
	}
	if userBalance != 200 {
		t.Fatalf("idempotent balance = %d, want 200 refund after reversal", userBalance)
	}
	assertPropertyHealthy(t, ctx, pool, "concurrent idempotent commands")
}

type propertyOperation struct {
	name string
	run  func(context.Context) error
}

type propertyResult struct {
	name string
	err  error
}

func seedPropertyFixture(ctx context.Context, pool *pgxpool.Pool, tenantID, userID string) error {
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, 'Property Tenant', 'active')
	`, tenantID); err != nil {
		return fmt.Errorf("seed property tenant: %w", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, 'property-user', 'hash', 4, 'active')
	`, userID, tenantID); err != nil {
		return fmt.Errorf("seed property user: %w", err)
	}
	if err := inTx(ctx, pool, func(tx pgx.Tx) error {
		for _, fixture := range []struct {
			prefix    string
			orderType string
			account   ledger.Ref
			amount    int64
			expiresAt *time.Time
			source    string
			count     int
		}{
			{prefix: "PROP_REVOKE", orderType: billingdomain.OrderTypeTenantToUser, account: ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, amount: 30_000, source: billingdomain.PackageSourceTenantRecharge, count: 8},
			{prefix: "PROP_EXPIRE", orderType: billingdomain.OrderTypePlatformToTenant, account: ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, amount: 20_000, expiresAt: timePtr(time.Now().UTC().Add(-time.Hour)), source: billingdomain.PackageSourceAdminRecharge, count: 8},
		} {
			for i := 0; i < fixture.count; i++ {
				orderID := fmt.Sprintf("%s_%02d", fixture.prefix, i)
				if _, err := tx.Exec(ctx, `
					INSERT INTO bill_recharge_orders
						(order_id, order_type, tenant_id, user_id, credit_amount, paid_amount, operator_id)
					VALUES ($1, $2, $3, NULLIF($4, ''), $5, $5, 'property-test')
				`, orderID, fixture.orderType, tenantID, func() string {
					if fixture.orderType == billingdomain.OrderTypeTenantToUser {
						return userID
					}
					return ""
				}(), fixture.amount); err != nil {
					return fmt.Errorf("seed property order %s: %w", orderID, err)
				}
				if _, err := ledger.Grant(ctx, tx, fixture.account, fixture.amount, fixture.expiresAt, fixture.source, orderID); err != nil {
					return fmt.Errorf("seed property lot %s: %w", orderID, err)
				}
			}
		}
		_, err := ledger.Grant(ctx, tx, ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, 2_000_000, nil, billingdomain.PackageSourceAdminRecharge, "")
		return err
	}); err != nil {
		return fmt.Errorf("seed property ledger: %w", err)
	}
	for i := 0; i < 8; i++ {
		orderID := fmt.Sprintf("PROP_GRANT_%02d", i)
		if _, err := pool.Exec(ctx, `
			INSERT INTO bill_recharge_orders
				(order_id, order_type, tenant_id, user_id, credit_amount, paid_amount, operator_id)
			VALUES ($1, 'tenant_to_user', $2, $3, 7000, 7000, 'property-test')
		`, orderID, tenantID, userID); err != nil {
			return fmt.Errorf("seed grant order %s: %w", orderID, err)
		}
	}
	if err := inTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := ledger.Grant(ctx, tx, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 2_000_000, nil, billingdomain.PackageSourceTenantRecharge, "")
		return err
	}); err != nil {
		return fmt.Errorf("seed property user ledger: %w", err)
	}
	for i := 0; i < 8; i++ {
		requestID := fmt.Sprintf("PROP_REFUND_%02d", i)
		if _, err := pool.Exec(ctx, `
			INSERT INTO ai_usage_logs
				(request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
				 model_code, billable_unit_type, tenant_payable, user_payable, user_charged,
				 billing_status, request_status, client_protocol, billing_source)
			VALUES ($1, 'user', 'jwt', 'property-test', $2, $3,
				 'property-model', 'token', 100, 200, 200,
				 'settled', 'success', 'openai_chat', 'payg')
		`, requestID, tenantID, userID); err != nil {
			return fmt.Errorf("seed property usage %s: %w", requestID, err)
		}
	}
	return nil
}

func seedIdempotencyFixture(ctx context.Context, pool *pgxpool.Pool, tenantID, userID string) error {
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, 'Idempotency Tenant', 'active')
	`, tenantID); err != nil {
		return fmt.Errorf("seed idempotency tenant: %w", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, 'idempotency-user', 'hash', 4, 'active')
	`, userID, tenantID); err != nil {
		return fmt.Errorf("seed idempotency user: %w", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_recharge_orders
			(order_id, order_type, tenant_id, user_id, credit_amount, paid_amount, operator_id)
		VALUES ('IDEMP_ORDER', 'tenant_to_user', $1, $2, 1000, 1000, 'property-test')
	`, tenantID, userID); err != nil {
		return fmt.Errorf("seed idempotency order: %w", err)
	}
	if err := inTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := ledger.Grant(ctx, tx, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 1000, nil, billingdomain.PackageSourceTenantRecharge, "IDEMP_ORDER")
		return err
	}); err != nil {
		return fmt.Errorf("seed idempotency lot: %w", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
			 model_code, billable_unit_type, tenant_payable, user_payable, user_charged,
			 billing_status, request_status, client_protocol, billing_source)
		VALUES ('IDEMP_USAGE', 'user', 'jwt', 'property-test', $1, $2,
			 'property-model', 'token', 0, 200, 200,
			 'settled', 'success', 'openai_chat', 'payg')
	`, tenantID, userID); err != nil {
		return fmt.Errorf("seed idempotency usage: %w", err)
	}
	return nil
}

func assertPropertyHealthy(t *testing.T, ctx context.Context, pool *pgxpool.Pool, stage string) {
	t.Helper()
	report, err := invariants.Check(ctx, pool)
	if err != nil {
		t.Fatalf("%s invariant query: %v", stage, err)
	}
	if report.InvariantsChecked != 7 {
		t.Fatalf("%s checked %d invariants, want 7", stage, report.InvariantsChecked)
	}
	if err := report.Err(); err != nil {
		t.Fatalf("%s invariants: %v (report=%+v)", stage, err, report)
	}
}

func inTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func timePtr(value time.Time) *time.Time { return &value }

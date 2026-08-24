package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/dbtest"
	shared "xiaodou/dai/internal/domain"
	tenantpg "xiaodou/dai/internal/tenant/pg"
)

func TestRechargeServiceOwnsManualGrantTransactionAndTargetLock(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_manual_grant', 'Manual Grant Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_manual_grant', 'tenant_manual_grant', 'manual-grant-user', 'x', 4, 'active');
	`); err != nil {
		t.Fatalf("seed manual grant target: %v", err)
	}

	recharge := service.NewRechargeService(pool, tenantpg.NewTenantRepository(pool))
	grant, err := recharge.GrantManual(ctx, service.GrantParams{
		OrderType: billing.OrderTypeTenantToUser, TenantID: "tenant_manual_grant", UserID: "user_manual_grant",
		AmountMicroUSD: 1000, OperatorID: "tenant-operator", Source: billing.PackageSourceTenantRecharge,
	})
	if err != nil {
		t.Fatalf("grant manual user recharge: %v", err)
	}
	if grant.OrderID == "" || grant.BalanceLotID == "" || grant.LotAmountMicroUSD != 1000 {
		t.Fatalf("manual grant result = %+v", grant)
	}

	var orderCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_recharge_orders WHERE order_type = 'tenant_to_user'`).Scan(&orderCount); err != nil {
		t.Fatalf("count manual recharge orders: %v", err)
	}
	var balance int64
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_manual_grant'`).Scan(&balance); err != nil {
		t.Fatalf("read manual grant balance: %v", err)
	}
	if orderCount != 1 || balance != 1000 {
		t.Fatalf("manual grant order/balance = %d/%d", orderCount, balance)
	}
	tenantGrant, err := recharge.GrantManual(ctx, service.GrantParams{
		OrderType: billing.OrderTypePlatformToTenant, TenantID: "tenant_manual_grant",
		AmountMicroUSD: 2000, OperatorID: "platform-operator", Source: billing.PackageSourceAdminRecharge,
	})
	if err != nil {
		t.Fatalf("grant manual tenant recharge: %v", err)
	}
	if tenantGrant.OrderID == "" || tenantGrant.BalanceLotID == "" || tenantGrant.LotAmountMicroUSD != 2000 {
		t.Fatalf("manual tenant grant result = %+v", tenantGrant)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'tenant_manual_grant'`).Scan(&balance); err != nil {
		t.Fatalf("read manual tenant balance: %v", err)
	}
	if balance != 2000 {
		t.Fatalf("manual tenant grant balance = %d", balance)
	}

	if _, err := pool.Exec(ctx, `UPDATE iam_accounts SET status = 'disabled' WHERE user_id = 'user_manual_grant'`); err != nil {
		t.Fatalf("disable manual grant target: %v", err)
	}
	if _, err := recharge.GrantManual(ctx, service.GrantParams{
		OrderType: billing.OrderTypeTenantToUser, TenantID: "tenant_manual_grant", UserID: "user_manual_grant",
		AmountMicroUSD: 500, OperatorID: "tenant-operator", Source: billing.PackageSourceTenantRecharge,
	}); !errors.Is(err, shared.ErrBadRequest) {
		t.Fatalf("disabled target error = %v, want bad request", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_recharge_orders WHERE order_type = 'tenant_to_user'`).Scan(&orderCount); err != nil {
		t.Fatalf("count orders after disabled target: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = 'user_manual_grant'`).Scan(&balance); err != nil {
		t.Fatalf("read balance after disabled target: %v", err)
	}
	if orderCount != 1 || balance != 1000 {
		t.Fatalf("disabled target changed order/balance = %d/%d", orderCount, balance)
	}
}

func TestGrantBalanceRejectsPaymentAndManualBoundaryMixups(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_grant_validation', 'Grant Validation Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_grant_validation', 'tenant_grant_validation', 'grant-validation-user', 'x', 4, 'active');
	`); err != nil {
		t.Fatalf("seed grant validation target: %v", err)
	}

	cases := []service.GrantParams{
		{
			OrderType: billing.OrderTypeTenantToUser, TenantID: "tenant_grant_validation", UserID: "user_grant_validation",
			AmountMicroUSD: 100, PaymentOrderID: "PAY_MIXED", OperatorID: "operator", Source: billing.PackageSourceTenantRecharge,
		},
		{
			OrderType: billing.OrderTypeOnlineUserTopup, TenantID: "tenant_grant_validation", UserID: "user_grant_validation",
			AmountMicroUSD: 100, OperatorID: "operator", PaymentRef: "WX_MIXED", Source: billing.PackageSourceOnlineTopup,
		},
		{
			OrderType: billing.OrderTypeOnlineUserTopup, TenantID: "tenant_grant_validation", UserID: "user_grant_validation",
			AmountMicroUSD: 100, PaymentOrderID: "PAY_MIXED", PaymentRef: "WX_MIXED", OperatorID: "operator", Source: billing.PackageSourceTenantRecharge,
		},
	}

	for i, params := range cases {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin validation transaction %d: %v", i, err)
		}
		if _, err := service.GrantBalance(ctx, tx, params); !errors.Is(err, shared.ErrBadRequest) {
			t.Fatalf("case %d error = %v, want bad request", i, err)
		}
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Fatalf("rollback validation transaction %d: %v", i, err)
		}
	}

	var orderCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_recharge_orders`).Scan(&orderCount); err != nil {
		t.Fatalf("count invalid grant orders: %v", err)
	}
	if orderCount != 0 {
		t.Fatalf("invalid grant calls inserted %d orders", orderCount)
	}
}

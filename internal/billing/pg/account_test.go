package pg

import (
	"context"
	"errors"
	"testing"
	"time"

	"xiaodou/dai/internal/billing"
	billingports "xiaodou/dai/internal/billing/ports"
	"xiaodou/dai/internal/dbtest"
)

func TestAccountRepositoryUsesCanonicalBalanceAndQueryProjections(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Date(2026, time.August, 25, 8, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, created_at, updated_at)
		VALUES ('account-query-tenant', 'Account Query Tenant', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status, created_at, updated_at)
		VALUES ('account-query-user', 'account-query-tenant', 'account-query-user', 'hash', 4, 'active', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bill_accounts SET balance_micro = 2000000, updated_at = $1 WHERE account_id = 'account-query-tenant'`, now); err != nil {
		t.Fatalf("seed account balance: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_credit_lots (lot_id, account_id, granted_micro, consumed_micro, source, created_at, updated_at)
		VALUES ('account-query-lot', 'account-query-tenant', 3000000, 1000000, 'ADMIN_RECHARGE', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed credit lot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_invitation_codes (code, tenant_id, created_by, created_at, updated_at)
		VALUES ('ACCT2345', 'account-query-tenant', 'account-query-user', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed invitation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_recharge_orders (order_id, order_type, tenant_id, credit_amount, paid_amount, operator_id, note, created_at)
		VALUES ('account-query-order', 'platform_to_tenant', 'account-query-tenant', 2500000, 2500000, 'admin-1', 'seed', $1)
	`, now); err != nil {
		t.Fatalf("seed recharge: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_logs (request_id, key_owner_type, tenant_id, model_code, user_id, user_payable, user_charged, billing_status, request_status, created_at)
		VALUES ('account-query-request', 'tenant', 'account-query-tenant', 'model-1', 'account-query-user', 125000, 125000, 'settled', 'success', $1)
	`, now); err != nil {
		t.Fatalf("seed usage: %v", err)
	}

	repo := NewAccountRepository(pool)
	balance, err := repo.GetTenantBalance(ctx, "account-query-tenant", true)
	if err != nil {
		t.Fatalf("GetTenantBalance: %v", err)
	}
	if balance.AvailableUSD != 2 || balance.UsedUSD != 1 || balance.PermanentUSD != 2 || len(balance.BalanceLots) != 1 {
		t.Fatalf("balance projection = %#v", balance)
	}
	if balance.BalanceLots[0].RemainingUSD != 2 || balance.ServiceState != "active" {
		t.Fatalf("balance lot/state = %#v", balance)
	}

	rows, total, err := repo.ListRechargeRecords(ctx, billingports.RechargeRecordsQuery{
		TenantID: "account-query-tenant", OrderTypes: billing.TenantRechargeOrderTypes, Page: 1, Size: 10,
	})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].OrderID != "account-query-order" || rows[0].AmountUSD != 2.5 {
		t.Fatalf("recharge projection = %#v total=%d err=%v", rows, total, err)
	}

	stats, err := repo.GetAccountStats(ctx, "account-query-tenant")
	if err != nil {
		t.Fatalf("GetAccountStats: %v", err)
	}
	if stats.EndUserCount != 1 || stats.InviteCodeCount != 1 || stats.UserDeductionUSD != 0.125 {
		t.Fatalf("account stats = %#v", stats)
	}
}

func TestAccountRepositoryHonorsCanceledQueryContext(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 1})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = NewAccountRepository(pool).GetTenantBalance(canceled, "missing", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetTenantBalance canceled error = %v", err)
	}
}

package pg

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/dbtest"
)

func TestTenantAnalyticsReadModelsPreserveProjectionSemantics(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name)
		VALUES ('tenant-analytics', 'Analytics Tenant');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type)
		VALUES ('user-analytics', 'tenant-analytics', 'analytics-user', 'hash', 4);
		INSERT INTO iam_invitation_codes (code, tenant_id, created_by)
		VALUES ('analytics-code', 'tenant-analytics', 'user-analytics');
		UPDATE bill_accounts SET balance_micro = 25 WHERE account_id = 'user-analytics';
		INSERT INTO bill_settlements(created_at,request_id,tenant_id,user_id,dimensions,user_due,user_charged,state,reason,posted_at)
 VALUES(now(),'analytics-request','tenant-analytics','user-analytics','{"model_code":"analytics-model","request_source":"portal"}',7,7,'posted','reported_usage',now());
		INSERT INTO pay_cash_ledger
			(txn_id, tenant_id, txn_type, amount_micro_usd, balance_after_micro_usd, idempotency_key)
		VALUES ('analytics-cash', 'tenant-analytics', 'topup_income', 13, 13, 'analytics-cash');
	`); err != nil {
		t.Fatalf("seed analytics fixture: %v", err)
	}

	repo := NewTenantRepo(pool)
	stats, err := repo.GetTenantOverviewStats(ctx, "tenant-analytics", nil, nil)
	if err != nil {
		t.Fatalf("GetTenantOverviewStats: %v", err)
	}
	if stats.EndUserCount != 1 || stats.InviteCodeCount != 1 || stats.UserTotalBalanceUSD != 0.000025 ||
		stats.UserConsumptionCount != 1 || stats.ActiveUserCount != 1 || stats.SettlementIncomeMicroUSD != 13 {
		t.Fatalf("tenant analytics stats = %#v", stats)
	}

	ranking, err := repo.GetUserConsumptionRanking(ctx, "tenant-analytics", nil, nil, 10)
	if err != nil {
		t.Fatalf("GetUserConsumptionRanking: %v", err)
	}
	if len(ranking) != 1 || ranking[0].UserID != "user-analytics" || ranking[0].Username != "analytics-user" || ranking[0].AmountUSD != 0.000007 {
		t.Fatalf("user ranking = %#v", ranking)
	}

	clients, err := repo.GetClientConsumption(ctx, "tenant-analytics", nil, nil)
	if err != nil {
		t.Fatalf("GetClientConsumption: %v", err)
	}
	if len(clients) != 1 || clients[0].ClientID != "portal" || clients[0].ClientName != "portal" || clients[0].AmountUSD != 0.000007 {
		t.Fatalf("client consumption = %#v", clients)
	}
}

func TestLowBalanceUserCountIncludesAllActiveTenantUsers(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('balance-tenant', 'Balance'), ('other-tenant', 'Other');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type)
		SELECT 'low-' || n, 'balance-tenant', 'low-' || n, 'hash', 4 FROM generate_series(1, 205) n;
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status) VALUES
		('at-threshold', 'balance-tenant', 'at-threshold', 'hash', 4, 'active'),
		('above-threshold', 'balance-tenant', 'above-threshold', 'hash', 4, 'active'),
		('negative', 'balance-tenant', 'negative', 'hash', 4, 'active'),
		('disabled', 'balance-tenant', 'disabled', 'hash', 4, 'disabled'),
		('deleted', 'balance-tenant', 'deleted', 'hash', 4, 'deleted'),
		('locked', 'balance-tenant', 'locked', 'hash', 4, 'locked'),
		('inherited', 'balance-tenant', 'inherited', 'hash', 4, 'inherited_disabled'),
		('owner', 'balance-tenant', 'owner', 'hash', 3, 'active'),
		('other-user', 'other-tenant', 'other-user', 'hash', 4, 'active');
		UPDATE bill_accounts SET balance_micro = 1000000 WHERE account_id = 'at-threshold';
		UPDATE bill_accounts SET balance_micro = 1000001 WHERE account_id = 'above-threshold';
		UPDATE bill_accounts SET balance_micro = -1 WHERE account_id = 'negative';
	`); err != nil {
		t.Fatalf("seed balance fixture: %v", err)
	}

	// A future reporting period must not filter current account balances.
	from, to := time.Now().AddDate(1, 0, 0), time.Now().AddDate(1, 0, 1)
	stats, err := NewTenantRepo(pool).GetTenantOverviewStats(ctx, "balance-tenant", &from, &to)
	if err != nil {
		t.Fatal(err)
	}
	if stats.LowBalanceUserCount != 207 {
		t.Fatalf("low balance users = %d, want 207", stats.LowBalanceUserCount)
	}
}

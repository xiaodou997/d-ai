package pg

import (
	"context"
	"testing"

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
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, tenant_id, user_id, model_code, user_charged,
			 user_payable, billing_status, request_status, request_source)
		VALUES ('analytics-request', 'user', 'tenant-analytics', 'user-analytics',
			 'analytics-model', 7, 7, 'settled', 'success', 'portal');
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

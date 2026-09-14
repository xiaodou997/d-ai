package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/billing/ledger"
)

type fixedUsageBiller struct {
	result domain.BillingResult
}

func (b fixedUsageBiller) Calculate(context.Context, *serving.Request) (domain.BillingResult, error) {
	return b.result, nil
}

func TestPreUpstreamBillingFailureDoesNotAttributeOrChargePlannedRoute(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	billing := domain.BillingResult{
		CatalogBaseMicro: 500, TenantPayableMicro: 700,
		RetailBaseMicro: 600, UserPayableMicro: 900, UserChargedMicro: 900,
		APIKeyQuotaCostMicro: 1_100, ServiceTier: domain.ServiceTierStandard,
		BillingBreakdownJSON: []byte(`{"planned":true}`),
		BillableUnits:        1, BillableUnitType: "request",
		GroupNameSnapshot: "planned-group", GroupDefaultUserMultiplier: 1,
		EffectiveUserMultiplier: 1,
	}
	req := usageCompletionRequest("pre-upstream-failure", "pre-upstream-tenant", "pre-upstream-user")
	req.Attempts = nil
	req.RequestStatus = domain.RequestFailed
	req.HTTPStatus = 503
	req.ErrorCode = "billing_dependency_unavailable"
	req.FailedStep = "billing_guard"
	if err := NewUsageLogger(pool, fixedUsageBiller{result: billing}).Log(ctx, req); err != nil {
		t.Fatalf("log pre-upstream failure: %v", err)
	}
	var attempts, promptTokens, completionTokens int32
	var provider, endpoint, finalRoute *string
	var upstreamStatus *int32
	var tenantPayable, userCharged int64
	var billingStatus string
	if err := pool.QueryRow(ctx, `
		SELECT attempts_count, provider_code, endpoint_id::text, final_route_id::text,
		       upstream_status, prompt_tokens, completion_tokens,
		       tenant_payable, user_charged, billing_status
		FROM ai_consumption_projection WHERE request_id=$1
	`, req.RequestID).Scan(&attempts, &provider, &endpoint, &finalRoute,
		&upstreamStatus, &promptTokens, &completionTokens,
		&tenantPayable, &userCharged, &billingStatus); err != nil {
		t.Fatalf("read pre-upstream usage: %v", err)
	}
	if attempts != 0 || provider != nil || endpoint != nil || finalRoute != nil ||
		upstreamStatus != nil || promptTokens != 0 || completionTokens != 0 ||
		tenantPayable != 0 || userCharged != 0 || billingStatus != "void" {
		t.Fatalf("pre-upstream attribution/charge = attempts:%d provider:%v endpoint:%v route:%v upstream:%v tokens:(%d,%d) amount:(%d,%d) status:%s",
			attempts, provider, endpoint, finalRoute, upstreamStatus, promptTokens,
			completionTokens, tenantPayable, userCharged, billingStatus)
	}
}

func usageCompletionRequest(requestID, tenantID, userID string) *serving.Request {
	return &serving.Request{
		RequestedModel: "test-model",
		ModelCode:      "test-model",
		CapabilityType: domain.CapabilityChat,
		ClientProtocol: domain.ProtocolOpenAIChat,
		Subject: &coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodJWT,
			RequestSource: coreidentity.RequestSourceWebChat,
			Scope:         coreidentity.ScopeUser,
			TenantID:      tenantID,
			UserID:        userID,
		},
		Candidate: &domain.RouteCandidate{
			RouteID:                    "11111111-1111-1111-1111-111111111111",
			AccountID:                  "33333333-3333-3333-3333-333333333333",
			EndpointID:                 "44444444-4444-4444-4444-444444444444",
			ProviderCode:               "test-upstream",
			ModelCode:                  "test-model",
			CapabilityType:             domain.CapabilityChat,
			Protocol:                   domain.ProtocolOpenAIChat,
			GroupID:                    "22222222-2222-2222-2222-222222222222",
			GroupName:                  "test-group",
			GroupDefaultUserMultiplier: 1,
			TenantMultiplier:           1,
			ResolvedProviderFamily:     "openai",
		},
		UsageEvidence: domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10, "output_tokens": 20}},
		TokenUsage: domain.TokenUsage{
			PromptTokens:     10,
			CompletionTokens: 20,
		},
		Attempts:      []serving.AttemptRecord{{RouteID: "11111111-1111-1111-1111-111111111111"}},
		BillingSource: subscription.BillingSourcePayg,
		RequestStatus: domain.RequestSuccess,
		HTTPStatus:    200,
		RequestID:     requestID,
		StartedAt:     time.Now().Add(-time.Second),
	}
}

// seedDirectBillingAccounts creates the identity rows only. The bill_accounts
// rows appear on their own: provisioning is a schema trigger, so forgetting it
// here is not possible.
func seedDirectBillingAccounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, userID, label string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, $2, 'active')
	`, tenantID, "usage-test-"+label); err != nil {
		t.Fatalf("seed tenant account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, $3, 'x', 4, 'active')
	`, userID, tenantID, "u_"+label+"-"+tenantID[len(tenantID)-8:]); err != nil {
		t.Fatalf("seed user account: %v", err)
	}
	for _, id := range []string{tenantID, userID} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bill_accounts WHERE account_id = $1)`, id).Scan(&exists); err != nil {
			t.Fatalf("check provisioned billing account: %v", err)
		}
		if !exists {
			t.Fatalf("billing account was not provisioned for %s", id)
		}
	}
}

func grantTestBalance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ref ledger.Ref, micro int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin grant: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := ledger.Grant(ctx, tx, ref, micro, nil, "ADMIN_RECHARGE", ""); err != nil {
		t.Fatalf("grant balance to %s: %v", ref.ID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit grant: %v", err)
	}
}

func accountBalance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, accountID string) int64 {
	t.Helper()
	var balance int64
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = $1`, accountID).Scan(&balance); err != nil {
		t.Fatalf("read balance for %s: %v", accountID, err)
	}
	return balance
}

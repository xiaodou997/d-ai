package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/billing/ledger"
	"xiaodou/dai/internal/billing/outbox"
)

type fixedUsageBiller struct {
	result domain.BillingResult
}

func (b fixedUsageBiller) Calculate(context.Context, *serving.Request) (domain.BillingResult, error) {
	return b.result, nil
}

func TestUsageCompletionIsIdempotentAndAtomic(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "usage-tenant-" + suffix
	userID := "usage-user-" + suffix
	rollbackTenantID := "usage-rollback-tenant-" + suffix
	rollbackUserID := "usage-rollback-user-" + suffix
	failedSubTenantID := "usage-failed-sub-tenant-" + suffix
	failedSubUserID := "usage-failed-sub-user-" + suffix
	requestID := "usage-request-" + suffix
	rollbackRequestID := "usage-rollback-request-" + suffix
	failedSubRequestID := "usage-failed-sub-request-" + suffix
	t.Cleanup(func() {
		tenantIDs := []string{tenantID, rollbackTenantID, failedSubTenantID}
		requestIDs := []string{requestID, rollbackRequestID, failedSubRequestID}
		_, _ = pool.Exec(ctx, `DELETE FROM ai_usage_rollups_hourly WHERE tenant_id = ANY($1::text[])`, tenantIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_usage_logs WHERE request_id = ANY($1::text[])`, requestIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_charge_outbox WHERE tenant_id = ANY($1::text[])`, tenantIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_credit_lots WHERE account_id IN (SELECT account_id FROM bill_accounts WHERE tenant_id = ANY($1::text[]))`, tenantIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM bill_accounts WHERE tenant_id = ANY($1::text[])`, tenantIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_accounts WHERE tenant_id = ANY($1::text[])`, tenantIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id = ANY($1::text[])`, tenantIDs)
	})

	billing := domain.BillingResult{
		CatalogBaseMicro:           500,
		TenantPayableMicro:         700,
		RetailBaseMicro:            600,
		UserPayableMicro:           900,
		UserChargedMicro:           900,
		APIKeyQuotaCostMicro:       1_100,
		ServiceTier:                domain.ServiceTierStandard,
		BillingBreakdownJSON:       []byte(`{}`),
		BillableUnits:              30,
		BillableUnitType:           "token",
		GroupNameSnapshot:          "test-group",
		GroupDefaultUserMultiplier: 1,
		EffectiveUserMultiplier:    1,
	}
	logger := NewUsageLogger(pool, fixedUsageBiller{result: billing})
	seedDirectBillingAccounts(t, ctx, pool, tenantID, userID, "usage")
	seedDirectBillingAccounts(t, ctx, pool, rollbackTenantID, rollbackUserID, "rollback")
	seedDirectBillingAccounts(t, ctx, pool, failedSubTenantID, failedSubUserID, "failed-sub")
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, 700)
	grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 900)

	req := usageCompletionRequest(requestID, tenantID, userID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_async_tasks (
		  task_type, tenant_id, user_id, model_code, input_payload, status, request_id
		) VALUES ('chat.completions', $1, $2, 'test-model', '{}'::jsonb, 'cancelled', $3)
	`, tenantID, userID, requestID); err != nil {
		t.Fatalf("seed cancelled async task: %v", err)
	}
	if err := logger.Log(ctx, req); err != nil {
		t.Fatalf("first completion: %v", err)
	}
	if err := logger.Log(ctx, req); err != nil {
		t.Fatalf("duplicate completion: %v", err)
	}

	assertUsageCompletionState(t, ctx, pool, requestID, tenantID, userID, 1, 700, 900)

	// Log ran twice; the balance must show exactly one charge. request_id is
	// unique on both ai_usage_logs and bill_charge_outbox, so the replay adds
	// neither a usage row nor a second debit.
	if got := accountBalance(t, ctx, pool, tenantID); got != 0 {
		t.Fatalf("tenant balance after one 700 charge on a 700 grant = %d, want 0", got)
	}
	if got := accountBalance(t, ctx, pool, userID); got != 0 {
		t.Fatalf("user balance after one 900 charge on a 900 grant = %d, want 0", got)
	}

	var callerCharge int64
	if err := pool.QueryRow(ctx, `SELECT caller_charge FROM ai_async_tasks WHERE request_id = $1`, requestID).Scan(&callerCharge); err != nil {
		t.Fatalf("read cancelled async task charge: %v", err)
	}
	if callerCharge != 900 {
		t.Fatalf("cancelled async task caller charge = %d, want 900", callerCharge)
	}
	upstreamRows, err := NewUsageRepo(dbgen.New(pool), pool).UpstreamSummary(ctx, domain.UsageSummaryFilter{TenantID: tenantID})
	if err != nil {
		t.Fatalf("read upstream reference-cost summary: %v", err)
	}
	if len(upstreamRows) != 1 || upstreamRows[0].TargetKind != "direct_upstream" ||
		upstreamRows[0].TargetID != "33333333-3333-3333-3333-333333333333" ||
		upstreamRows[0].CatalogBaseMicro != 500 || upstreamRows[0].TenantPayableMicro != 700 {
		t.Fatalf("upstream reference-cost summary = %#v", upstreamRows)
	}
	waitForUsageRollup(t, ctx, pool, tenantID)

	rollbackReq := usageCompletionRequest(rollbackRequestID, rollbackTenantID, rollbackUserID)
	rollbackReq.Subject.AuthMethod = coreidentity.AuthMethodAPIKey
	rollbackReq.Subject.RequestSource = coreidentity.RequestSourceAPIKey
	rollbackReq.Subject.APIKeyID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if err := logger.Log(ctx, rollbackReq); err == nil {
		t.Fatal("missing API key must fail financial completion")
	}
	assertUsageCompletionState(t, ctx, pool, rollbackRequestID, rollbackTenantID, rollbackUserID, 0, 0, 0)

	failedSubBilling := billing
	failedSubBilling.CatalogBaseMicro = 0
	failedSubBilling.TenantPayableMicro = 0
	failedSubBilling.UserPayableMicro = 0
	// Charged must fall with payable. Calculate never yields charged > payable,
	// and ai_usage_logs_charge_semantics now rejects that shape outright.
	failedSubBilling.UserChargedMicro = 0
	failedSubBilling.APIKeyQuotaCostMicro = 0
	failedSubBilling.RetailBaseMicro = 0
	failedSubBilling.BillableUnits = 0
	failedSubLogger := NewUsageLogger(pool, fixedUsageBiller{result: failedSubBilling})
	failedSubReq := usageCompletionRequest(failedSubRequestID, failedSubTenantID, failedSubUserID)
	failedSubReq.TokenUsage = domain.TokenUsage{}
	failedSubReq.BillingSource = subscription.BillingSourceSubscription
	failedSubReq.SubscriptionID = "33333333-3333-3333-3333-333333333333"
	failedSubReq.RequestStatus = domain.RequestFailed
	failedSubReq.HTTPStatus = 502
	failedSubReq.ErrorCode = "upstream_error"
	if err := failedSubLogger.Log(ctx, failedSubReq); err != nil {
		t.Fatalf("record zero-usage subscription failure: %v", err)
	}
	assertUsageCompletionState(t, ctx, pool, failedSubRequestID, failedSubTenantID, failedSubUserID, 1, 0, 0)
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
		FROM ai_usage_logs WHERE request_id=$1
	`, req.RequestID).Scan(&attempts, &provider, &endpoint, &finalRoute,
		&upstreamStatus, &promptTokens, &completionTokens,
		&tenantPayable, &userCharged, &billingStatus); err != nil {
		t.Fatalf("read pre-upstream usage: %v", err)
	}
	if attempts != 0 || provider != nil || endpoint != nil || finalRoute != nil ||
		upstreamStatus != nil || promptTokens != 0 || completionTokens != 0 ||
		tenantPayable != 0 || userCharged != 0 || billingStatus != "free" {
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

// drainOutbox settles every queued charge so a test can assert on final
// balances. Production runs the same consumer on a timer.
func drainOutbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for range 10 {
		applied, err := outbox.NewConsumer(pool, nil).DrainOnce(ctx)
		if err != nil {
			t.Fatalf("drain billing outbox: %v", err)
		}
		if applied == 0 {
			return
		}
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

func assertUsageCompletionState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	requestID, tenantID, userID string,
	wantLogs, wantTenantMicro, wantUserMicro int64,
) {
	t.Helper()
	var logs int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_usage_logs WHERE request_id = $1`, requestID).Scan(&logs); err != nil {
		t.Fatalf("count usage logs: %v", err)
	}
	if logs != wantLogs {
		t.Fatalf("usage logs = %d, want %d", logs, wantLogs)
	}
	var billingStatus string
	err := pool.QueryRow(ctx, `
		SELECT billing_status
		FROM ai_usage_logs WHERE request_id = $1
	`, requestID).Scan(&billingStatus)
	if err != nil {
		if wantLogs == 0 {
			return
		}
		t.Fatalf("read usage billing state: %v", err)
	}
	if wantTenantMicro == 0 && wantUserMicro == 0 {
		if billingStatus != "free" {
			t.Fatalf("zero-amount usage billing = status:%s", billingStatus)
		}
		return
	}

	// Before the outbox drains, the usage row must say "pending" rather than
	// claim a charge that has not been applied — and the queued charge must
	// already exist, because it was committed with the usage row.
	if billingStatus != "pending" {
		t.Fatalf("pre-settlement usage billing = status:%s, want pending", billingStatus)
	}
	var queuedTenant, queuedUser int64
	if err := pool.QueryRow(ctx, `
		SELECT tenant_micro, user_micro FROM bill_charge_outbox WHERE request_id = $1
	`, requestID).Scan(&queuedTenant, &queuedUser); err != nil {
		t.Fatalf("read queued charge: %v", err)
	}
	if queuedTenant != wantTenantMicro || queuedUser != wantUserMicro {
		t.Fatalf("queued charge = (%d,%d), want (%d,%d)", queuedTenant, queuedUser, wantTenantMicro, wantUserMicro)
	}

	drainOutbox(t, ctx, pool)

	var settledAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT billing_status, settled_at
		FROM ai_usage_logs WHERE request_id = $1
	`, requestID).Scan(&billingStatus, &settledAt); err != nil {
		t.Fatalf("read settled usage billing state: %v", err)
	}
	if settledAt == nil || billingStatus != "settled" {
		t.Fatalf("settled charge state = settled_at:%v status:%s", settledAt, billingStatus)
	}
	var outboxStatus string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM bill_charge_outbox WHERE request_id = $1
	`, requestID).Scan(&outboxStatus); err != nil {
		t.Fatalf("read settled outbox status: %v", err)
	}
	if outboxStatus != "done" {
		t.Fatalf("outbox status = %s, want done", outboxStatus)
	}
}

func waitForUsageRollup(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int64
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_usage_rollups_hourly WHERE tenant_id = $1`, tenantID).Scan(&count); err != nil {
			t.Fatalf("read usage rollup: %v", err)
		}
		if count > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("usage rollup was not written")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

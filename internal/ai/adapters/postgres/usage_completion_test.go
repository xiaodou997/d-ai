package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/billingledger"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/testsupport"
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
		_, _ = pool.Exec(ctx, `DELETE FROM ai_billing_request_admissions WHERE request_id = ANY($1::text[])`, requestIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_billing_windows WHERE tenant_id = ANY($1::text[])`, tenantIDs)
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
	coordinator := billingledger.New(pool, nil, billingledger.Config{}, nil)
	logger := NewUsageLogger(pool, fixedUsageBiller{result: billing}).WithBillingCoordinator(coordinator)

	req := usageCompletionRequest(requestID, tenantID, userID)
	seedBillingAdmission(t, ctx, pool, req)
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
	var callerCharge int64
	if err := pool.QueryRow(ctx, `SELECT caller_charge FROM ai_async_tasks WHERE request_id = $1`, requestID).Scan(&callerCharge); err != nil {
		t.Fatalf("read cancelled async task charge: %v", err)
	}
	if callerCharge != 900 {
		t.Fatalf("cancelled async task caller charge = %d, want 900", callerCharge)
	}
	upstreamRows, err := NewUsageRepo(dbgen.New(pool), pool).UpstreamSummary(ctx, observabilitycontrol.SummaryFilter{TenantID: tenantID})
	if err != nil {
		t.Fatalf("read upstream reference-cost summary: %v", err)
	}
	if len(upstreamRows) != 1 || upstreamRows[0].TargetKind != "direct_upstream" ||
		upstreamRows[0].TargetID != "44444444-4444-4444-4444-444444444444" ||
		upstreamRows[0].CatalogBaseMicro != 500 || upstreamRows[0].TenantPayableMicro != 700 {
		t.Fatalf("upstream reference-cost summary = %#v", upstreamRows)
	}
	waitForUsageRollup(t, ctx, pool, tenantID)

	rollbackReq := usageCompletionRequest(rollbackRequestID, rollbackTenantID, rollbackUserID)
	seedBillingAdmission(t, ctx, pool, rollbackReq)
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
	failedSubLogger := NewUsageLogger(pool, fixedUsageBiller{result: failedSubBilling}).WithBillingCoordinator(coordinator)
	failedSubReq := usageCompletionRequest(failedSubRequestID, failedSubTenantID, failedSubUserID)
	seedBillingAdmission(t, ctx, pool, failedSubReq)
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
	req.FailedStep = "billing_admission"
	req.BillingWindowID = ""
	req.BillingLeaseID = ""
	req.BillingAdmissionActive = false
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
		Attempts:               []serving.AttemptRecord{{RouteID: "11111111-1111-1111-1111-111111111111"}},
		BillingSource:          subscription.BillingSourcePayg,
		BillingWindowID:        "bw_" + requestID,
		BillingLeaseID:         "CL_" + requestID,
		BillingAdmissionActive: true,
		RequestStatus:          domain.RequestSuccess,
		HTTPStatus:             200,
		RequestID:              requestID,
		StartedAt:              time.Now().Add(-time.Second),
	}
}

func seedBillingAdmission(t *testing.T, ctx context.Context, pool *pgxpool.Pool, req *serving.Request) {
	t.Helper()
	now := time.Now()
	subject := req.RuntimeSubject()
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_billing_windows (
		  window_id, owner_type, tenant_id, user_id, want_tenant, want_user,
		  lease_id, lease_version, requested_tenant_micro, requested_user_micro,
		  granted_tenant_micro, granted_user_micro, state, expires_at, grace_until,
		  max_age_at, opened_at, created_at, updated_at
		) VALUES (
		  $1, 'user', $2, $3, true, true, $4, 1, 10000, 10000,
		  10000, 10000, 'active', $5, $6, $7, $8, $8, $8
		)
	`, req.BillingWindowID, subject.TenantID, subject.UserID, req.BillingLeaseID,
		now.Add(5*time.Minute), now.Add(20*time.Minute), now.Add(3*time.Minute), now); err != nil {
		t.Fatalf("seed billing window: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_billing_request_admissions (
		  request_id, window_id, lease_id, status, request_expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,'active',$4,$5,$5)
	`, req.RequestID, req.BillingWindowID, req.BillingLeaseID, now.Add(10*time.Minute), now); err != nil {
		t.Fatalf("seed billing admission: %v", err)
	}
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
	var tenantMicro, userMicro int64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(w.accrued_tenant_micro, 0), COALESCE(w.accrued_user_micro, 0)
		FROM ai_billing_request_admissions a
		JOIN ai_billing_windows w ON w.window_id=a.window_id
		WHERE a.request_id=$1 AND w.tenant_id=$2 AND w.user_id=$3
	`, requestID, tenantID, userID).Scan(&tenantMicro, &userMicro)
	if err != nil {
		t.Fatalf("read local ledger: %v", err)
	}
	if tenantMicro != wantTenantMicro || userMicro != wantUserMicro {
		t.Fatalf("local ledger = (%d,%d), want (%d,%d)", tenantMicro, userMicro, wantTenantMicro, wantUserMicro)
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

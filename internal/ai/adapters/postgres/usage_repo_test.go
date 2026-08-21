package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestUsageRepoAllowsGlobalAdminLogsWithoutTenantFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Bind to the canonical schema loaded from db/init.sql. This test previously
	// hand-copied CREATE TABLE for ai_usage_logs and ai_request_payloads, which
	// drifted from the real schema every time a money column changed and only
	// surfaced as "column does not exist" rather than as a behavioural failure.
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open usage repo test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()

	older := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	newer := older.Add(5 * time.Minute)

	mustExecUsageRepo(t, ctx, pool, `
		INSERT INTO ai_usage_logs (
		  id, request_id, key_owner_type, auth_method, request_source, tenant_id, client_user_agent,
		  model_code, capability_type, prompt_tokens, completion_tokens, total_tokens,
		  billable_unit_type, billable_units, resolution, catalog_base, tenant_payable, retail_base, user_payable, user_charged,
		  api_key_quota_cost, billing_status, request_status, latency_ms, created_at
		) VALUES (
		  '10000000-0000-0000-0000-000000000001', 'req-tenant-a', 'tenant', 'api_key', 'api_key', 'tenant-a', 'curl/8.7.1',
		  'gpt-5.4-mini', 'chat', 10, 20, 30,
		  'token', 30, NULL, 100, 200, 250, 300, 300,
          50, 'pending', 'success', 150, $1
		)
	`, older)
	mustExecUsageRepo(t, ctx, pool, `
		INSERT INTO ai_usage_logs (
		  id, request_id, key_owner_type, auth_method, request_source, tenant_id, client_user_agent,
		  model_code, capability_type, prompt_tokens, completion_tokens, total_tokens,
		  billable_unit_type, billable_units, resolution, catalog_base, tenant_payable, retail_base, user_payable, user_charged,
		  api_key_quota_cost, billing_status, request_status, error_message, created_at
		) VALUES (
		  '10000000-0000-0000-0000-000000000002', 'req-tenant-b', 'tenant', 'api_key', 'api_key', 'tenant-b', 'Mozilla/5.0',
		  'gpt-image-1', 'image_generation', 7, 11, 18,
		  'image', 1, '1024x1024', 80, 120, 140, 160, 160,
		  20, 'pending', 'failed', 'upstream failed', $1
		)
	`, newer)

	repo := NewUsageRepo(dbgen.New(pool), pool)

	globalFilter := domain.UsageFilter{}

	total, err := repo.CountLogs(ctx, globalFilter)
	if err != nil {
		t.Fatalf("CountLogs(global): %v", err)
	}
	if total != 2 {
		t.Fatalf("global total = %d, want 2", total)
	}

	stats, err := repo.StatsFor(ctx, globalFilter)
	if err != nil {
		t.Fatalf("StatsFor(global): %v", err)
	}
	if stats.TotalRequests != 2 {
		t.Fatalf("global stats total requests = %d, want 2", stats.TotalRequests)
	}
	if stats.SuccessCount != 1 {
		t.Fatalf("global success count = %d, want 1", stats.SuccessCount)
	}
	if stats.FailedCount != 1 {
		t.Fatalf("global failed count = %d, want 1", stats.FailedCount)
	}
	if stats.TotalTokens != 48 {
		t.Fatalf("global total tokens = %d, want 48", stats.TotalTokens)
	}
	if stats.AvgLatencyMs != 150 {
		t.Fatalf("global avg latency = %v, want 150", stats.AvgLatencyMs)
	}

	logs, err := repo.ListLogs(ctx, globalFilter, 20, 0)
	if err != nil {
		t.Fatalf("ListLogs(global): %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("global logs len = %d, want 2", len(logs))
	}
	if logs[0].RequestID != "req-tenant-b" {
		t.Fatalf("global first request_id = %q, want req-tenant-b", logs[0].RequestID)
	}
	if logs[1].RequestID != "req-tenant-a" {
		t.Fatalf("global second request_id = %q, want req-tenant-a", logs[1].RequestID)
	}
	if logs[0].ClientUserAgent != "Mozilla/5.0" {
		t.Fatalf("global first client user agent = %q, want Mozilla/5.0", logs[0].ClientUserAgent)
	}
	if logs[0].Resolution != "1024x1024" {
		t.Fatalf("global first resolution = %q, want 1024x1024", logs[0].Resolution)
	}
	if logs[1].ClientUserAgent != "curl/8.7.1" {
		t.Fatalf("global second client user agent = %q, want curl/8.7.1", logs[1].ClientUserAgent)
	}

	detail, err := repo.GetLogDetail(ctx, "req-tenant-b")
	if err != nil {
		t.Fatalf("GetLogDetail: %v", err)
	}
	if detail.UserAgent != "Mozilla/5.0" {
		t.Fatalf("detail user agent = %q, want Mozilla/5.0", detail.UserAgent)
	}
	if detail.Resolution != "1024x1024" {
		t.Fatalf("detail resolution = %q, want 1024x1024", detail.Resolution)
	}

	scopedFilter := domain.UsageFilter{TenantID: "tenant-a"}

	total, err = repo.CountLogs(ctx, scopedFilter)
	if err != nil {
		t.Fatalf("CountLogs(scoped): %v", err)
	}
	if total != 1 {
		t.Fatalf("scoped total = %d, want 1", total)
	}

	logs, err = repo.ListLogs(ctx, scopedFilter, 20, 0)
	if err != nil {
		t.Fatalf("ListLogs(scoped): %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("scoped logs len = %d, want 1", len(logs))
	}
	if logs[0].TenantID != "tenant-a" {
		t.Fatalf("scoped tenant_id = %q, want tenant-a", logs[0].TenantID)
	}
}

func mustExecUsageRepo(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec sql failed: %v", err)
	}
}

func TestUsageLogFromUserRowPreservesSelfServiceProjection(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 8, 21, 9, 30, 0, 0, time.UTC)
	var id pgtype.UUID
	if err := id.Scan("10000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatalf("scan id: %v", err)
	}
	var groupID pgtype.UUID
	if err := groupID.Scan("20000000-0000-0000-0000-000000000002"); err != nil {
		t.Fatalf("scan group id: %v", err)
	}
	var multiplier pgtype.Numeric
	if err := multiplier.Scan("1.25"); err != nil {
		t.Fatalf("scan multiplier: %v", err)
	}

	got := usageLogFromUserRow(dbgen.ListUsageLogsByTenantUserRow{
		ID:                              id,
		RequestID:                       "request-1",
		TraceID:                         pgtype.Text{String: "trace-1", Valid: true},
		TenantID:                        "tenant-1",
		UserID:                          pgtype.Text{String: "user-1", Valid: true},
		RequestSource:                   "workspace",
		GroupID:                         groupID,
		GroupNameSnapshot:               "default",
		EffectiveUserMultiplierSnapshot: multiplier,
		BillingGroupLabelSnapshot:       "Default",
		ModelCode:                       "gpt-test",
		Stream:                          true,
		PromptTokens:                    11,
		CompletionTokens:                13,
		CacheWriteTokens:                17,
		CacheReadTokens:                 19,
		ReasoningTokens:                 23,
		ReasoningEffort:                 pgtype.Text{String: "high", Valid: true},
		TotalTokens:                     83,
		BillableUnitType:                "token",
		BillableUnits:                   83,
		UserPayable:                     2900,
		UserCharged:                     3100,
		ServiceTier:                     "fast",
		BillingStatus:                   "settled",
		RefundStatus:                    "none",
		BillingSource:                   "payg",
		RequestStatus:                   "success",
		HttpStatus:                      pgtype.Int4{Int32: 200, Valid: true},
		LatencyMs:                       pgtype.Int4{Int32: 400, Valid: true},
		FirstTokenLatencyMs:             pgtype.Int4{Int32: 75, Valid: true},
		ErrorCode:                       pgtype.Text{String: "code", Valid: true},
		ErrorMessage:                    pgtype.Text{String: "message", Valid: true},
		CreatedAt:                       pgtype.Timestamptz{Time: createdAt, Valid: true},
	})

	if got.ID != "10000000-0000-0000-0000-000000000001" || got.GroupID != "20000000-0000-0000-0000-000000000002" {
		t.Fatalf("ids = %q %q", got.ID, got.GroupID)
	}
	if got.RequestID != "request-1" || got.TraceID != "trace-1" || got.TenantID != "tenant-1" || got.UserID != "user-1" || got.RequestSource != "workspace" {
		t.Fatalf("identity projection = %+v", got)
	}
	if got.EffectiveUserMultiplierSnapshot != 1.25 || got.UserPayableMicro != 2900 || got.UserChargedMicro != 3100 {
		t.Fatalf("billing projection = %+v", got)
	}
	if !got.Stream || got.CacheWriteTokens != 17 || got.CacheReadTokens != 19 || got.ReasoningTokens != 23 || got.ReasoningEffort != "high" {
		t.Fatalf("token projection = %+v", got)
	}
	if got.HTTPStatus == nil || *got.HTTPStatus != 200 || got.LatencyMs == nil || *got.LatencyMs != 400 || got.FirstTokenLatencyMs == nil || *got.FirstTokenLatencyMs != 75 {
		t.Fatalf("latency projection = %+v", got)
	}
	if got.ErrorCode != "code" || got.ErrorMessage != "message" || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("result projection = %+v", got)
	}
}

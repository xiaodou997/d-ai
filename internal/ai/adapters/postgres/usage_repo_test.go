package postgres

import (
	"context"
	"testing"
	"time"

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
		  50, 'pending_settle', 'success', 150, $1
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
		  20, 'pending_settle', 'failed', 'upstream failed', $1
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

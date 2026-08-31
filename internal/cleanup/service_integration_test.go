package cleanup

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/dai/internal/dbtest"
)

func TestCleanupTargetsRespectRetentionAndProtectionRules(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	bodyOld := time.Now().UTC().Add(-100 * 24 * time.Hour)
	old := time.Now().UTC().Add(-400 * 24 * time.Hour)
	veryOld := time.Now().UTC().Add(-800 * 24 * time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_request_payloads (
			request_id, created_at, client_protocol, request_path, request_model,
			request_messages, request_params, response_message, media_refs,
			request_status, internal_error_detail, attempts_detail
		) VALUES
			('cleanup-body', $1, 'openai_chat', '/v1/chat/completions', 'gpt-test', '{"secret":"request"}', '{"temperature":1}', '{"answer":"response"}', '[{"sha256":"referenced"}]', 'completed', 'upstream detail', '[{"attempt":1}]'),
			('cleanup-delete', $2, 'openai_chat', '/v1/chat/completions', 'gpt-test', '{"old":true}', NULL, NULL, NULL, 'failed', NULL, NULL),
			('cleanup-reference', now(), 'openai_chat', '/v1/chat/completions', 'gpt-test', NULL, NULL, NULL, '[{"sha256":"referenced"}]', 'completed', NULL, NULL)
	`, bodyOld, veryOld); err != nil {
		t.Fatalf("seed request payloads: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_audit_blobs (sha256, content, content_type, size_bytes, created_at)
		VALUES ('referenced', '\x01'::bytea, 'image/png', 1, $1), ('unreferenced', '\x02'::bytea, 'image/png', 1, $1)
	`, old); err != nil {
		t.Fatalf("seed audit blobs: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sys_notification_deliveries (event_key, channel, title, body, status, created_at)
		VALUES ('cleanup.sent', 'in_app', 'old sent', 'body', 'sent', $1),
		       ('cleanup.pending', 'in_app', 'old pending', 'body', 'pending', $1)
	`, old); err != nil {
		t.Fatalf("seed notifications: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_content_moderation_logs (mode, action, flagged, created_at)
		VALUES ('observe', 'allow', false, $1), ('observe', 'block', true, $1)
	`, old); err != nil {
		t.Fatalf("seed moderation logs: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_risk_events (summary, status, created_at)
		VALUES ('old resolved', 'resolved', $1), ('old open', 'open', $1)
	`, old); err != nil {
		t.Fatalf("seed risk events: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_admin_audit_logs (action, result, created_at)
		VALUES ('cleanup.old', 'ok', $1)
	`, old); err != nil {
		t.Fatalf("seed admin audit logs: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_rollups_hourly (bucket_start, tenant_id, model_code, request_status, billable_unit_type)
		VALUES ($1, 'tenant-cleanup', 'gpt-test', 'completed', 'request')
	`, veryOld); err != nil {
		t.Fatalf("seed usage rollups: %v", err)
	}

	svc := NewService(pool, zap.NewNop())
	policy := DefaultPolicy()
	if got, err := svc.clearRequestBodies(ctx, time.Now().UTC().Add(-30*24*time.Hour), policy.BatchSize); err != nil || got != 2 {
		t.Fatalf("clear request bodies = %d, err=%v; want 2", got, err)
	}
	if got, err := svc.deleteRequestPayloads(ctx, time.Now().UTC().Add(-180*24*time.Hour), policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("delete request payloads = %d, err=%v; want 1", got, err)
	}
	if got, err := svc.deleteNotifications(ctx, time.Now().UTC().Add(-90*24*time.Hour), policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("delete notifications = %d, err=%v; want 1", got, err)
	}
	if got, err := svc.deleteModerationLogs(ctx, time.Now().UTC().Add(-90*24*time.Hour), policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("delete moderation logs = %d, err=%v; want 1", got, err)
	}
	if got, err := svc.deleteRiskEvents(ctx, time.Now().UTC().Add(-365*24*time.Hour), policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("delete risk events = %d, err=%v; want 1", got, err)
	}
	if got, err := svc.deleteAdminAuditLogs(ctx, time.Now().UTC().Add(-365*24*time.Hour), policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("delete admin audit logs = %d, err=%v; want 1", got, err)
	}
	if got, err := svc.deleteUsageRollups(ctx, time.Now().UTC().Add(-730*24*time.Hour), policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("delete usage rollups = %d, err=%v; want 1", got, err)
	}
	if got, err := svc.deleteUnreferencedBlobs(ctx, time.Now().UTC().Add(-180*24*time.Hour), policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("delete audit blobs = %d, err=%v; want 1", got, err)
	}
	if got, err := svc.clearAllRequestBodies(ctx, policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("clear all request bodies = %d, err=%v; want 1", got, err)
	}

	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM ai_request_payloads WHERE request_id = 'cleanup-body'`, 1)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM ai_request_payloads WHERE request_id = 'cleanup-delete'`, 0)
	var requestStatus string
	var requestMessages []byte
	if err := pool.QueryRow(ctx, `SELECT request_status, request_messages FROM ai_request_payloads WHERE request_id = 'cleanup-reference'`).Scan(&requestStatus, &requestMessages); err != nil {
		t.Fatalf("read purged request payload: %v", err)
	}
	if requestStatus != "completed" || requestMessages != nil {
		t.Fatalf("purged request payload = status:%q messages:%v, want completed and NULL messages", requestStatus, requestMessages)
	}
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM sys_notification_deliveries WHERE status = 'pending'`, 1)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM ai_content_moderation_logs WHERE flagged = true`, 1)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM ai_risk_events WHERE status = 'open'`, 1)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM ai_audit_blobs WHERE sha256 = 'referenced'`, 1)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM ai_audit_blobs WHERE sha256 = 'unreferenced'`, 0)
}

func assertCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, query).Scan(&got); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("count %q = %d, want %d", query, got, want)
	}
}

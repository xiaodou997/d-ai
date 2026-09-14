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

	old := time.Now().UTC().Add(-400 * 24 * time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO ai_request_debug_payloads(created_at,request_id,session_id,content_gzip,original_bytes,truncated) VALUES(now(),'cleanup-debug',gen_random_uuid(),'\x01',1,false); INSERT INTO bill_settlements(created_at,request_id,tenant_id,state,reason) VALUES(now(),'cleanup-debug','tenant','waived','missing_usage')`); err != nil {
		t.Fatal(err)
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

	svc := NewService(pool, zap.NewNop())
	policy := DefaultPolicy()
	preview, err := svc.Preview(ctx)
	if err != nil {
		t.Fatalf("preview cleanup: %v", err)
	}
	if preview.RequestBodyPurge.EligibleRows != 1 || preview.RequestBodyPurge.OccupiedBytes <= 0 {
		t.Fatalf("request body purge preview = %+v, want 1 row and positive occupied bytes", preview.RequestBodyPurge)
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
	if got, err := svc.clearAllRequestBodies(ctx, policy.BatchSize); err != nil || got != 1 {
		t.Fatalf("debug purge=%d %v", got, err)
	}
	assertCount(t, ctx, pool, `SELECT count(*) FROM ai_request_debug_payloads`, 0)
	assertCount(t, ctx, pool, `SELECT count(*) FROM bill_settlements WHERE request_id='cleanup-debug'`, 1)
	assertCount(t, ctx, pool, `SELECT count(*) FROM sys_notification_deliveries WHERE status='pending'`, 1)
	assertCount(t, ctx, pool, `SELECT count(*) FROM ai_content_moderation_logs WHERE flagged`, 1)
	assertCount(t, ctx, pool, `SELECT count(*) FROM ai_risk_events WHERE status='open'`, 1)
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

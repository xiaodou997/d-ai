package pg

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/dbtest"
)

func TestListFailedTransactionAlertsKeepsProjectionInRepository(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, tenant_id, model_code, billing_status, request_status, settlement_error, created_at)
		VALUES
			('alert-recent', 'tenant', 'tenant-alerts', 'gpt-test', 'failed', 'failed', 'upstream timeout', $1),
			('alert-newest', 'tenant', 'tenant-alerts', 'gpt-test', 'failed', 'failed', NULL, $2),
			('alert-old', 'tenant', 'tenant-alerts', 'gpt-test', 'failed', 'failed', 'old failure', $3),
			('alert-pending', 'tenant', 'tenant-alerts', 'gpt-test', 'pending', 'success', NULL, $2)
	`, now.Add(-2*time.Hour), now.Add(-time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("seed usage alerts: %v", err)
	}

	alerts, err := NewSystemRepository(pool).ListFailedTransactionAlerts(ctx)
	if err != nil {
		t.Fatalf("ListFailedTransactionAlerts: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("alerts len = %d, want 2", len(alerts))
	}
	if alerts[0].RequestID != "alert-newest" || alerts[1].RequestID != "alert-recent" {
		t.Fatalf("alert order = %#v", alerts)
	}
	if alerts[0].SettlementError != "" || alerts[1].SettlementError != "upstream timeout" {
		t.Fatalf("settlement errors = %#v", alerts)
	}
}

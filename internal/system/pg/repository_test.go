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
		INSERT INTO bill_settlements(request_id,tenant_id,state,reason,last_error,created_at)
 VALUES ('alert-recent','tenant-alerts','review','reported_usage','upstream timeout',$1),('alert-newest','tenant-alerts','review','reported_usage','',$2),('alert-old','tenant-alerts','review','reported_usage','old failure',$3),('alert-pending','tenant-alerts','pending','reported_usage','',$2)
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

package cleanup

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/dbtest"
)

func TestCleanupLeaseHeartbeatAndTerminalWriteAreFenced(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	first := NewService(pool, zap.NewNop())
	second := NewService(pool, zap.NewNop())
	run, err := first.queueRun(ctx, "manual", []string{TargetNotifications}, "operator-1")
	if err != nil {
		t.Fatalf("queue cleanup run: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE sys_data_cleanup_runs
		SET status = 'running', started_at = now(), heartbeat_at = now(), lease_until = now() + interval '2 minutes'
		WHERE id = $1::uuid
	`, run.ID); err != nil {
		t.Fatalf("claim cleanup run fixture: %v", err)
	}

	held, err := first.renewLease(ctx, run.ID)
	if err != nil || !held {
		t.Fatalf("heartbeat by owner = held:%t err:%v, want held:true", held, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE sys_data_cleanup_runs
		SET owner_id = $2, lease_until = now() + interval '2 minutes'
		WHERE id = $1::uuid
	`, run.ID, second.ownerID); err != nil {
		t.Fatalf("take over cleanup lease fixture: %v", err)
	}
	held, err = first.renewLease(ctx, run.ID)
	if err != nil || held {
		t.Fatalf("heartbeat after takeover = held:%t err:%v, want held:false", held, err)
	}

	first.finishRunWithProgress(ctx, run.ID, run.Trigger, run.RequestedBy, run.Targets, nil, RunProgress{Phase: "failed"}, first.ownerID, errors.New("stale worker"))
	var status, owner string
	if err := pool.QueryRow(ctx, `SELECT status, owner_id FROM sys_data_cleanup_runs WHERE id = $1::uuid`, run.ID).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if status != "running" || owner != second.ownerID {
		t.Fatalf("fenced terminal write changed run to status:%q owner:%q", status, owner)
	}

	if _, err := pool.Exec(ctx, `UPDATE sys_data_cleanup_runs SET lease_until = now() - interval '1 second' WHERE id = $1::uuid`, run.ID); err != nil {
		t.Fatalf("expire cleanup lease fixture: %v", err)
	}
	if err := second.recoverStaleRuns(ctx); err != nil {
		t.Fatalf("recover expired cleanup lease: %v", err)
	}
	var recovered string
	if err := pool.QueryRow(ctx, `SELECT status FROM sys_data_cleanup_runs WHERE id = $1::uuid`, run.ID).Scan(&recovered); err != nil {
		t.Fatal(err)
	}
	if recovered != "failed" {
		t.Fatalf("recovered stale run status = %q, want failed", recovered)
	}
	if _, err := second.queueRun(ctx, "automatic", []string{TargetNotifications}, ""); err != nil {
		t.Fatalf("queue after expired lease recovery: %v", err)
	}
}

func TestCleanupExecuteClaimsQueuedRun(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	service := NewService(pool, zap.NewNop())
	run, err := service.queueRun(ctx, "manual", []string{TargetNotifications}, "operator-1")
	if err != nil {
		t.Fatalf("queue cleanup run: %v", err)
	}
	service.execute(ctx, run.ID, "manual", []string{TargetNotifications}, "operator-1")

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM sys_data_cleanup_runs WHERE id = $1::uuid`, run.ID).Scan(&status); err != nil {
		t.Fatalf("read cleanup run status: %v", err)
	}
	if status != "completed" {
		t.Fatalf("cleanup run status = %q, want completed", status)
	}
}

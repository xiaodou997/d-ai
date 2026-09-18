package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"xiaodou/dai/internal/dbtest"
)

func TestBanReconcilerLifecycleIsIdempotent(t *testing.T) {
	r := NewBanReconciler(nil, nil, nil, 0)
	r.Start()
	r.Start()
	if got := r.Health(); got.Started || got.Stopped {
		t.Fatalf("unconfigured reconciler health = %+v, want not started", got)
	}
	r.Stop()
	r.Stop()
	if got := r.Health(); !got.Stopped {
		t.Fatalf("stopped reconciler health = %+v, want stopped", got)
	}
}

func TestBanReconcilerCannotStartAfterStop(t *testing.T) {
	r := NewBanReconciler(nil, nil, nil, 0)
	if err := r.Stop(); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	r.Start()
	if got := r.Health(); got.Started || !got.Stopped {
		t.Fatalf("reconciler health after stop-before-start = %+v", got)
	}
}

func TestBanReconcilerStopCanRetryAfterDeadline(t *testing.T) {
	r := NewBanReconciler(nil, nil, nil, 0)
	release := make(chan struct{})
	r.lifecycleMu.Lock()
	r.started = true
	r.workerCtx, r.workerCancel = context.WithCancel(context.Background())
	r.wg.Add(1)
	r.lifecycleMu.Unlock()
	go func() {
		<-release
		r.wg.Done()
	}()

	shortCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if err := r.Stop(shortCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Stop error = %v, want deadline exceeded", err)
	}
	cancel()

	retryDone := make(chan struct{})
	go func() {
		if err := r.Stop(context.Background()); err != nil {
			t.Errorf("retry Stop: %v", err)
		}
		close(retryDone)
	}()
	select {
	case <-retryDone:
		t.Fatal("retry Stop returned before reconcile loop exited")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-retryDone:
	case <-time.After(time.Second):
		t.Fatal("retry Stop did not finish after reconcile loop exited")
	}
}


func TestBanReconcilerTreatsEveryNonActiveDatabaseStateAsBanned(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES
		  ('ban-active-tenant', 'Active Tenant', 'active'),
		  ('ban-suspended-tenant', 'Suspended Tenant', 'suspended'),
		  ('ban-deleting-tenant', 'Deleting Tenant', 'deleting')
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status)
		VALUES
		  ('ban-active-user', 'ban-active-user', 'unused', 2, 'active'),
		  ('ban-locked-user', 'ban-locked-user', 'unused', 4, 'locked'),
		  ('ban-deleted-user', 'ban-deleted-user', 'unused', 4, 'deleted')
	`); err != nil {
		t.Fatal(err)
	}

	r := NewBanReconciler(pool, nil, nil, time.Minute)
	users, err := r.trueBannedUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if users["ban-active-user"] || !users["ban-locked-user"] || !users["ban-deleted-user"] {
		t.Fatalf("true banned users = %#v", users)
	}
	tenants, err := r.trueBannedTenants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tenants["ban-active-tenant"] || !tenants["ban-suspended-tenant"] || !tenants["ban-deleting-tenant"] {
		t.Fatalf("true banned tenants = %#v", tenants)
	}
}

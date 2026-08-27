package auth

import (
	"context"
	"errors"
	"testing"
	"time"
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

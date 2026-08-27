package gateway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRuntimeAuthToucherStopWaitsAndFencesNewTouches(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	toucher := newRuntimeAuthToucher(func(context.Context, pgtype.UUID) error {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return nil
	})

	if got := toucher.Health(); got.Started || got.Stopped {
		t.Fatalf("initial touch health = %+v", got)
	}
	toucher.Start()
	keyID := pgtype.UUID{Valid: true}
	toucher.Enqueue(context.Background(), keyID)
	<-started

	shortCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := toucher.Stop(shortCtx); err == nil {
		t.Fatal("short Stop unexpectedly waited for blocked touch")
	}
	if got := toucher.Health(); !got.Started || got.Stopped {
		t.Fatalf("health after timed-out Stop = %+v, want still in-flight", got)
	}

	toucher.Enqueue(context.Background(), keyID)
	if got := calls.Load(); got != 1 {
		t.Fatalf("touches after Stop fence = %d, want 1", got)
	}
	close(release)
	if err := toucher.Stop(context.Background()); err != nil {
		t.Fatalf("final Stop: %v", err)
	}
	if got := toucher.Health(); !got.Started || !got.Stopped {
		t.Fatalf("final touch health = %+v, want stopped", got)
	}
}

func TestRuntimeAuthToucherSkipsCanceledRequest(t *testing.T) {
	var calls atomic.Int32
	toucher := newRuntimeAuthToucher(func(context.Context, pgtype.UUID) error {
		calls.Add(1)
		return nil
	})
	toucher.Start()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	toucher.Enqueue(ctx, pgtype.UUID{Valid: true})
	if got := calls.Load(); got != 0 {
		t.Fatalf("canceled request touch calls = %d, want 0", got)
	}
	if err := toucher.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

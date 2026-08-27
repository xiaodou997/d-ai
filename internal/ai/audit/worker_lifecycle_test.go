package audit

import (
	"context"
	"testing"
)

func TestWorkerLifecycleIsIdempotent(t *testing.T) {
	w := NewWorker(&fakeStore{}, nil, WorkerOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	w.Start(ctx)
	if got := w.Health(); !got.Started || got.Stopped {
		t.Fatalf("running worker health = %+v, want started and not stopped", got)
	}
	w.Stop(context.Background())
	w.Stop(context.Background())
	if got := w.Health(); !got.Stopped {
		t.Fatalf("stopped worker health = %+v, want stopped", got)
	}
	cancel()
}

func TestWorkerCannotStartAfterStop(t *testing.T) {
	w := NewWorker(&fakeStore{}, nil, WorkerOptions{})
	w.Stop(context.Background())
	w.Start(context.Background())
	if got := w.Health(); !got.Stopped || got.Started {
		t.Fatalf("worker health after stop-before-start = %+v", got)
	}
}

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
	w.Stop(context.Background())
	w.Stop(context.Background())
	cancel()
}

func TestWorkerCannotStartAfterStop(t *testing.T) {
	w := NewWorker(&fakeStore{}, nil, WorkerOptions{})
	w.Stop(context.Background())
	w.Start(context.Background())
}

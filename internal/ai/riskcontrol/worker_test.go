package riskcontrol

import (
	"context"
	"testing"
)

func TestWorkerLifecycleIsIdempotent(t *testing.T) {
	w := NewWorker(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx, 1)
	w.Start(ctx, 1)
	w.Stop(context.Background())
	w.Stop(context.Background())
	cancel()
}

func TestWorkerCannotStartAfterStop(t *testing.T) {
	w := NewWorker(nil, nil)
	w.Stop(context.Background())
	w.Start(context.Background(), 1)
}

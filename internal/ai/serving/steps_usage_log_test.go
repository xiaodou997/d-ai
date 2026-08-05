package serving

import (
	"context"
	"testing"
	"time"
)

type usageLoggerFunc func(context.Context, *Request) error

func (f usageLoggerFunc) Log(ctx context.Context, req *Request) error {
	return f(ctx, req)
}

func TestUsageLogFinalizerCompletesAfterClientContextCancellation(t *testing.T) {
	clientCtx, cancelClient := context.WithCancel(context.Background())
	cancelClient()

	called := false
	finalizer := &UsageLogFinalizer{Logger: usageLoggerFunc(func(ctx context.Context, _ *Request) error {
		called = true
		if err := ctx.Err(); err != nil {
			t.Fatalf("completion context inherited client cancellation: %v", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("completion context has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > financialCompletionTimeout {
			t.Fatalf("completion deadline remaining = %v", remaining)
		}
		return nil
	})}

	finalizer.Finalize(clientCtx, &Request{})
	if !called {
		t.Fatal("usage logger was not called")
	}
}

func TestUsageLogFinalizerRunsAfterPipelineFailure(t *testing.T) {
	called := false
	finalizer := &UsageLogFinalizer{Logger: usageLoggerFunc(func(_ context.Context, req *Request) error {
		called = true
		if req.RequestStatus != "failed" {
			t.Fatalf("request status = %q, want failed", req.RequestStatus)
		}
		if req.FailedStep != "upstream" {
			t.Fatalf("failed step = %q, want upstream", req.FailedStep)
		}
		return nil
	})}

	pipeline := NewPipeline(&fakeStep{name: "upstream", execErr: context.DeadlineExceeded}).WithFinalizers(finalizer)
	if err := pipeline.Run(context.Background(), &Request{}); err == nil {
		t.Fatal("pipeline failure was not returned")
	}
	if !called {
		t.Fatal("usage finalizer was not called")
	}
}

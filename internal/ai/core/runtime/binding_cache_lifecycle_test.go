package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	coreupstream "xiaodou/dai/internal/ai/core/upstream"
)

func TestCachedBindingResolverStopCancelsAndWaitsForSharedLoad(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	started := make(chan string, 1)
	source := &countingBindingResolver{gate: gate, started: started}
	resolver := NewCachedBindingResolver(source, BindingResolverOptions{LoadTimeout: time.Minute})
	resolver.Start(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveRuntimeBinding(context.Background(), coreupstream.RuntimeBindingRequest{TargetID: "in-flight"})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("shared binding load did not start")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := resolver.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("shared load error = %v, want context.Canceled", err)
	}
	if got := resolver.Health(); !got.Started || !got.Stopped {
		t.Fatalf("stopped resolver health = %+v", got)
	}

	close(gate)
}

func TestCachedBindingResolverCannotResolveAfterStop(t *testing.T) {
	resolver := NewCachedBindingResolver(&countingBindingResolver{}, BindingResolverOptions{})
	if err := resolver.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() before Start error = %v", err)
	}
	resolver.Start(context.Background())
	_, err := resolver.ResolveRuntimeBinding(context.Background(), coreupstream.RuntimeBindingRequest{TargetID: "stopped"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve after Stop error = %v, want context.Canceled", err)
	}
}

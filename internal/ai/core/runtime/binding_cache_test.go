package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	coreupstream "xiaodou/dai/internal/ai/core/upstream"
)

func TestCachedBindingResolverCachesAndClonesSuccessfulBinding(t *testing.T) {
	t.Parallel()

	source := &countingBindingResolver{}
	resolver := NewCachedBindingResolver(source, BindingResolverOptions{TTL: time.Minute})
	req := coreupstream.RuntimeBindingRequest{TargetID: "upstream-1", ResolvedModelID: "gpt-5.4"}

	first, err := resolver.ResolveRuntimeBinding(context.Background(), req)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	first.ExtraHeaders["X-Test"] = "mutated"
	first.ModelBinding.Config["mode"] = "mutated"

	second, err := resolver.ResolveRuntimeBinding(context.Background(), req)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if source.callCount() != 1 {
		t.Fatalf("source calls = %d, want 1", source.callCount())
	}
	if second.ExtraHeaders["X-Test"] != "original" || second.ModelBinding.Config["mode"] != "original" {
		t.Fatalf("cached binding was mutated through caller-owned maps: %#v", second)
	}
}

func TestCachedBindingResolverAuthorizesEveryCacheHit(t *testing.T) {
	t.Parallel()

	source := &countingBindingResolver{}
	authorizer := &countingBindingAuthorizer{}
	resolver := NewCachedBindingResolver(source, BindingResolverOptions{TTL: time.Minute, Authorizer: authorizer})
	req := coreupstream.RuntimeBindingRequest{TenantID: "tenant-a", TargetID: "upstream-1"}

	for range 2 {
		if _, err := resolver.ResolveRuntimeBinding(context.Background(), req); err != nil {
			t.Fatalf("ResolveRuntimeBinding() error = %v", err)
		}
	}
	if source.callCount() != 1 {
		t.Fatalf("source calls = %d, want one cached load", source.callCount())
	}
	if authorizer.calls != 2 {
		t.Fatalf("authorizer calls = %d, want one per request", authorizer.calls)
	}
}

func TestCachedBindingResolverCanDisableStaleResults(t *testing.T) {
	source := &countingBindingResolver{}
	resolver := NewCachedBindingResolver(source, BindingResolverOptions{TTL: time.Minute, DisableCache: true})
	req := coreupstream.RuntimeBindingRequest{TenantID: "tenant-a", TargetID: "upstream-1"}
	for range 2 {
		if _, err := resolver.ResolveRuntimeBinding(context.Background(), req); err != nil {
			t.Fatalf("ResolveRuntimeBinding() error = %v", err)
		}
	}
	if source.callCount() != 2 {
		t.Fatalf("source calls = %d, want 2 with disabled cache", source.callCount())
	}
}

func TestCachedBindingResolverResolvesBatchConcurrentlyAndPreservesOrder(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	started := make(chan string, 4)
	source := &countingBindingResolver{gate: gate, started: started}
	resolver := NewCachedBindingResolver(source, BindingResolverOptions{
		TTL: time.Minute, MaxConcurrency: 2,
	})
	requests := []coreupstream.RuntimeBindingRequest{
		{TargetID: "upstream-1"},
		{TargetID: "upstream-2"},
		{TargetID: "upstream-3"},
		{TargetID: "upstream-4"},
	}

	done := make(chan []bindingResolution, 1)
	go func() {
		done <- resolver.resolveRuntimeBindings(context.Background(), requests)
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("two binding reads did not start concurrently")
		}
	}
	close(gate)

	results := <-done
	if source.maxConcurrent() != 2 {
		t.Fatalf("maximum concurrent binding reads = %d, want 2", source.maxConcurrent())
	}
	for index, result := range results {
		if result.err != nil {
			t.Fatalf("result %d: %v", index, result.err)
		}
		if result.binding.Upstream.ID != requests[index].TargetID {
			t.Fatalf("result %d upstream = %q, want %q", index, result.binding.Upstream.ID, requests[index].TargetID)
		}
	}
}

func TestCachedBindingResolverCanceledCallerDoesNotPoisonSharedLoad(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	started := make(chan string, 1)
	source := &countingBindingResolver{gate: gate, started: started}
	resolver := NewCachedBindingResolver(source, BindingResolverOptions{TTL: time.Minute, LoadTimeout: time.Second})
	req := coreupstream.RuntimeBindingRequest{TargetID: "shared-upstream"}

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveRuntimeBinding(firstCtx, req)
		firstDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("shared binding load did not start")
	}
	cancelFirst()
	if err := <-firstDone; err != context.Canceled {
		t.Fatalf("canceled caller error = %v, want context.Canceled", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveRuntimeBinding(context.Background(), req)
		secondDone <- err
	}()
	close(gate)
	if err := <-secondDone; err != nil {
		t.Fatalf("healthy waiter inherited another request's cancellation: %v", err)
	}
	if source.callCount() != 1 {
		t.Fatalf("source calls = %d, want one shared load", source.callCount())
	}
}

type countingBindingResolver struct {
	mu        sync.Mutex
	calls     int
	active    int
	maxActive int
	gate      <-chan struct{}
	started   chan<- string
}

type countingBindingAuthorizer struct {
	calls int
}

func (a *countingBindingAuthorizer) AuthorizeRuntimeBinding(context.Context, coreupstream.RuntimeBindingRequest) error {
	a.calls++
	return nil
}

func (r *countingBindingResolver) ResolveRuntimeBinding(ctx context.Context, req coreupstream.RuntimeBindingRequest) (coreupstream.RuntimeBinding, error) {
	r.mu.Lock()
	r.calls++
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.active--
		r.mu.Unlock()
	}()

	if r.started != nil {
		r.started <- req.TargetID
	}
	if r.gate != nil {
		select {
		case <-ctx.Done():
			return coreupstream.RuntimeBinding{}, context.Cause(ctx)
		case <-r.gate:
		}
	}
	return coreupstream.RuntimeBinding{
		Upstream:     coreupstream.Upstream{ID: req.TargetID},
		ExtraHeaders: map[string]string{"X-Test": "original"},
		ModelBinding: coreupstream.ModelBinding{Config: map[string]any{"mode": "original"}},
	}, nil
}

func (r *countingBindingResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *countingBindingResolver) maxConcurrent() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxActive
}

package runtime

import (
	"context"
	"sync"
	"time"

	coreupstream "xiaodou/dai/internal/ai/core/upstream"
	"xiaodou/dai/internal/lifecycle"
)

const (
	defaultBindingCacheTTL        = 5 * time.Second
	defaultBindingCacheMaxEntries = 4096
	defaultBindingResolveWorkers  = 8
	defaultBindingLoadTimeout     = 5 * time.Second
)

type BindingResolverOptions struct {
	TTL            time.Duration
	MaxEntries     int
	MaxConcurrency int
	LoadTimeout    time.Duration
	// DisableCache keeps in-flight request coalescing while forcing every
	// completed resolve to reload current credentials, model status and pricing
	// metadata. Production uses this because those values are authorization and
	// billing inputs, not safe stale configuration.
	DisableCache bool
	Authorizer   RuntimeBindingAuthorizer
}

type RuntimeBindingAuthorizer interface {
	AuthorizeRuntimeBinding(context.Context, coreupstream.RuntimeBindingRequest) error
}

type bindingCacheEntry struct {
	binding   coreupstream.RuntimeBinding
	expiresAt time.Time
}

type bindingCall struct {
	done    chan struct{}
	binding coreupstream.RuntimeBinding
	err     error
}

type bindingResolution struct {
	binding coreupstream.RuntimeBinding
	err     error
}

// CachedBindingResolver keeps route configuration reads off the request hot
// path while coalescing concurrent cache misses for the same physical target.
type CachedBindingResolver struct {
	source         coreupstream.RuntimeBindingResolver
	ttl            time.Duration
	maxEntries     int
	maxConcurrency int
	loadTimeout    time.Duration
	disableCache   bool
	authorizer     RuntimeBindingAuthorizer

	mu       sync.Mutex
	entries  map[coreupstream.RuntimeBindingRequest]bindingCacheEntry
	inflight map[coreupstream.RuntimeBindingRequest]*bindingCall

	lifecycleMu sync.Mutex
	root        context.Context
	cancel      context.CancelFunc
	stopParent  func() bool
	started     bool
	stopped     bool
	loadWG      sync.WaitGroup
}

var _ lifecycle.Component = (*CachedBindingResolver)(nil)

func NewCachedBindingResolver(source coreupstream.RuntimeBindingResolver, options BindingResolverOptions) *CachedBindingResolver {
	if options.TTL <= 0 {
		options.TTL = defaultBindingCacheTTL
	}
	if options.MaxEntries <= 0 {
		options.MaxEntries = defaultBindingCacheMaxEntries
	}
	if options.MaxConcurrency <= 0 {
		options.MaxConcurrency = defaultBindingResolveWorkers
	}
	if options.LoadTimeout <= 0 {
		options.LoadTimeout = defaultBindingLoadTimeout
	}
	root, cancel := context.WithCancel(context.Background())
	return &CachedBindingResolver{
		source:         source,
		ttl:            options.TTL,
		maxEntries:     options.MaxEntries,
		maxConcurrency: options.MaxConcurrency,
		loadTimeout:    options.LoadTimeout,
		disableCache:   options.DisableCache,
		authorizer:     options.Authorizer,
		entries:        make(map[coreupstream.RuntimeBindingRequest]bindingCacheEntry),
		inflight:       make(map[coreupstream.RuntimeBindingRequest]*bindingCall),
		root:           root,
		cancel:         cancel,
	}
}

// Start binds detached cache-miss loads to the process lifecycle. Requests
// still wait on their own context, while the shared load is cancelled by Stop
// instead of continuing against released runtime dependencies.
func (r *CachedBindingResolver) Start(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.lifecycleMu.Lock()
	if r.started || r.stopped {
		r.lifecycleMu.Unlock()
		return
	}
	if r.root == nil {
		r.root, r.cancel = context.WithCancel(context.Background())
	}
	r.started = true
	if r.cancel != nil {
		r.stopParent = context.AfterFunc(ctx, r.cancel)
	}
	r.lifecycleMu.Unlock()
}

// Stop cancels all detached cache-miss loads and waits for them to finish.
// A later call may continue waiting after an earlier deadline expires.
func (r *CachedBindingResolver) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.lifecycleMu.Lock()
	if !r.stopped {
		r.stopped = true
	}
	cancel := r.cancel
	stopParent := r.stopParent
	r.stopParent = nil
	r.lifecycleMu.Unlock()
	if stopParent != nil {
		stopParent()
	}
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		r.loadWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Health returns a lock-safe lifecycle snapshot for management probes.
func (r *CachedBindingResolver) Health() lifecycle.HealthSnapshot {
	if r == nil {
		return lifecycle.HealthSnapshot{}
	}
	r.lifecycleMu.Lock()
	started, stopped := r.started, r.stopped
	r.lifecycleMu.Unlock()
	return lifecycle.HealthSnapshot{Started: started, Stopped: stopped}
}

func (r *CachedBindingResolver) ResolveRuntimeBinding(ctx context.Context, req coreupstream.RuntimeBindingRequest) (coreupstream.RuntimeBinding, error) {
	if r == nil || r.source == nil {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionBindingInvalid, "binding resolver is unavailable")
	}
	r.lifecycleMu.Lock()
	stopped := r.stopped
	r.lifecycleMu.Unlock()
	if stopped {
		return coreupstream.RuntimeBinding{}, context.Canceled
	}
	if err := context.Cause(ctx); err != nil {
		return coreupstream.RuntimeBinding{}, err
	}
	if r.authorizer != nil {
		if err := r.authorizer.AuthorizeRuntimeBinding(ctx, req); err != nil {
			return coreupstream.RuntimeBinding{}, err
		}
	}
	now := time.Now()
	r.mu.Lock()
	if !r.disableCache {
		if cached, ok := r.entries[req]; ok {
			if now.Before(cached.expiresAt) {
				r.mu.Unlock()
				return cloneRuntimeBinding(cached.binding), nil
			}
			delete(r.entries, req)
		}
	}
	if call, ok := r.inflight[req]; ok {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return coreupstream.RuntimeBinding{}, context.Cause(ctx)
		case <-call.done:
			return cloneRuntimeBinding(call.binding), call.err
		}
	}
	call := &bindingCall{done: make(chan struct{})}
	r.inflight[req] = call
	r.mu.Unlock()
	loadRoot, ok := r.reserveLoad()
	if !ok {
		r.finishCacheMiss(req, call, coreupstream.RuntimeBinding{}, context.Canceled)
	} else {
		loadCtx, cancelLoad := context.WithTimeout(loadRoot, r.loadTimeout)
		go func() {
			defer r.loadWG.Done()
			defer cancelLoad()
			r.resolveCacheMiss(loadCtx, req, call)
		}()
	}
	select {
	case <-ctx.Done():
		return coreupstream.RuntimeBinding{}, context.Cause(ctx)
	case <-call.done:
		return cloneRuntimeBinding(call.binding), call.err
	}
}

func (r *CachedBindingResolver) resolveCacheMiss(ctx context.Context, req coreupstream.RuntimeBindingRequest, call *bindingCall) {
	binding, err := r.source.ResolveRuntimeBinding(ctx, req)
	r.finishCacheMiss(req, call, binding, err)
}

func (r *CachedBindingResolver) reserveLoad() (context.Context, bool) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.stopped {
		return nil, false
	}
	if r.root == nil {
		r.root, r.cancel = context.WithCancel(context.Background())
	}
	r.loadWG.Add(1)
	return r.root, true
}

func (r *CachedBindingResolver) finishCacheMiss(req coreupstream.RuntimeBindingRequest, call *bindingCall, binding coreupstream.RuntimeBinding, err error) {
	r.mu.Lock()
	call.binding = cloneRuntimeBinding(binding)
	call.err = err
	delete(r.inflight, req)
	if err == nil && !r.disableCache {
		now := time.Now()
		r.makeRoomLocked(now)
		r.entries[req] = bindingCacheEntry{binding: cloneRuntimeBinding(binding), expiresAt: now.Add(r.ttl)}
	}
	close(call.done)
	r.mu.Unlock()
}

func (r *CachedBindingResolver) resolveRuntimeBindings(ctx context.Context, requests []coreupstream.RuntimeBindingRequest) []bindingResolution {
	results := make([]bindingResolution, len(requests))
	if len(requests) == 0 {
		return results
	}
	workers := min(r.maxConcurrency, len(requests))
	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index].binding, results[index].err = r.ResolveRuntimeBinding(ctx, requests[index])
			}
		}()
	}
	for index := range requests {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}

func (r *CachedBindingResolver) makeRoomLocked(now time.Time) {
	if len(r.entries) < r.maxEntries {
		return
	}
	for key, entry := range r.entries {
		if !now.Before(entry.expiresAt) {
			delete(r.entries, key)
		}
	}
	if len(r.entries) < r.maxEntries {
		return
	}
	for key := range r.entries {
		delete(r.entries, key)
		break
	}
}

func cloneRuntimeBinding(binding coreupstream.RuntimeBinding) coreupstream.RuntimeBinding {
	binding.ExtraHeaders = cloneStringMap(binding.ExtraHeaders)
	binding.ModelBinding.Config = cloneAnyMap(binding.ModelBinding.Config)
	return binding
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

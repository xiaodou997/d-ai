package billingcontrol

import (
	"context"
	"sync"
	"time"
)

const llmRetryDelay = time.Minute

// liteLLMPriceSource owns remote refresh, last-success caching, stale reads,
// fallback data, and refresh de-duplication behind one snapshot interface.
type liteLLMPriceSource struct {
	fetcher    LiteLLMFetcher
	cacheTTL   time.Duration
	retryDelay time.Duration
	now        func() time.Time

	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	cache       map[string]LiteLLMModel
	expiresAt   time.Time
	nextAttempt time.Time
	refreshing  bool
	started     bool
	stopped     bool
	refreshDone chan struct{}
}

func newLiteLLMPriceSource(fetcher LiteLLMFetcher, cacheTTL, retryDelay time.Duration) *liteLLMPriceSource {
	ctx, cancel := context.WithCancel(context.Background())
	return &liteLLMPriceSource{
		fetcher:    fetcher,
		cacheTTL:   cacheTTL,
		retryDelay: retryDelay,
		now:        time.Now,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (s *liteLLMPriceSource) Start(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	if !s.refreshing {
		if s.cancel != nil {
			s.cancel()
		}
		s.ctx, s.cancel = context.WithCancel(ctx)
	}
	s.mu.Unlock()
	_ = s.Snapshot()
}

// Stop cancels an in-flight remote refresh and waits for it to leave the
// network path. It is safe to call more than once; a later call may provide a
// longer deadline after an earlier call timed out.
func (s *liteLLMPriceSource) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	s.stopped = true
	cancel := s.cancel
	done := s.refreshDone
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Snapshot never waits for network I/O. It returns the newest successful
// remote snapshot, even when stale, or built-in defaults until one exists.
func (s *liteLLMPriceSource) Snapshot() map[string]LiteLLMModel {
	s.mu.Lock()
	now := s.now()
	data := s.cache
	if data == nil {
		data = mergedLiteLLMData(nil)
	}
	needsRefresh := s.cache == nil || !now.Before(s.expiresAt)
	startRefresh := needsRefresh && s.fetcher != nil && !s.refreshing && !now.Before(s.nextAttempt)
	var refreshCtx context.Context
	var refreshDone chan struct{}
	if startRefresh {
		s.refreshing = true
		refreshCtx = s.ctx
		if refreshCtx == nil {
			refreshCtx = context.Background()
		}
		refreshDone = make(chan struct{})
		s.refreshDone = refreshDone
	}
	s.mu.Unlock()

	if startRefresh {
		go func() {
			defer close(refreshDone)
			s.refresh(refreshCtx)
		}()
	}
	return data
}

func (s *liteLLMPriceSource) refresh(ctx context.Context) {
	data, err := s.fetcher.Fetch(ctx)
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshing = false
	if err != nil {
		s.nextAttempt = now.Add(s.retryDelay)
		return
	}
	s.cache = mergedLiteLLMData(data)
	s.expiresAt = now.Add(s.cacheTTL)
	s.nextAttempt = time.Time{}
}

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
	cache       map[string]LiteLLMModel
	expiresAt   time.Time
	nextAttempt time.Time
	refreshing  bool
}

func newLiteLLMPriceSource(fetcher LiteLLMFetcher, cacheTTL, retryDelay time.Duration) *liteLLMPriceSource {
	return &liteLLMPriceSource{
		fetcher:    fetcher,
		cacheTTL:   cacheTTL,
		retryDelay: retryDelay,
		now:        time.Now,
	}
}

func (s *liteLLMPriceSource) Start(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	_ = s.Snapshot()
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
	if startRefresh {
		s.refreshing = true
		refreshCtx = s.ctx
		if refreshCtx == nil {
			refreshCtx = context.Background()
		}
	}
	s.mu.Unlock()

	if startRefresh {
		go s.refresh(refreshCtx)
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

package clientcatalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/lifecycle"
)

const (
	DefaultCacheTTL       = 10 * time.Minute
	DefaultFailureBackoff = time.Minute
	DefaultFetchTimeout   = 15 * time.Second
	FallbackRevision      = "builtin-model-catalog@2026-07-28"
)

type CredentialSelector interface {
	SelectCredentialFromPool(ctx context.Context, poolID, strategy string) (*domain.OAuthCredential, error)
}

type Result struct {
	Models          []clientruntime.ModelCard
	Source          string
	ProfileRevision string
	ObservedAt      time.Time
}

type cacheEntry struct {
	result      Result
	etag        string
	expiresAt   time.Time
	nextAttempt time.Time
}

var errServiceStopped = errors.New("client catalog service is stopped")

// Service owns live fixed-provider model discovery, cache revalidation,
// stale-if-error behavior, and versioned built-in fallbacks.
type Service struct {
	selector  CredentialSelector
	inspector clientruntime.Inspector
	logger    *zap.Logger

	cacheTTL       time.Duration
	failureBackoff time.Duration
	fetchTimeout   time.Duration
	now            func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
	flights singleflight.Group

	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	ownerCtx    context.Context
	nextLoadID  uint64
	activeLoads map[uint64]context.CancelFunc
	loadWG      sync.WaitGroup
}

var _ lifecycle.Component = (*Service)(nil)

func New(selector CredentialSelector, inspector clientruntime.Inspector, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{
		selector:       selector,
		inspector:      inspector,
		logger:         logger,
		cacheTTL:       DefaultCacheTTL,
		failureBackoff: DefaultFailureBackoff,
		fetchTimeout:   DefaultFetchTimeout,
		now:            time.Now,
		entries:        make(map[string]cacheEntry),
		activeLoads:    make(map[uint64]context.CancelFunc),
	}
}

// Start marks the catalog as owned by the process lifecycle. Resolve remains
// usable before Start for lightweight callers and tests; Start is idempotent
// and a stopped catalog cannot be restarted.
func (s *Service) Start(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.lifecycleMu.Lock()
	if s.started || s.stopped {
		s.lifecycleMu.Unlock()
		return
	}
	s.started = true
	s.ownerCtx = ctx
	s.lifecycleMu.Unlock()
}

// Stop cancels every in-flight provider discovery and waits for the
// singleflight leader to leave the selector/inspector path. It is safe to call
// again with a longer context after an earlier wait timed out.
func (s *Service) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.lifecycleMu.Lock()
	s.stopped = true
	s.ownerCtx = nil
	cancels := make([]context.CancelFunc, 0, len(s.activeLoads))
	for _, cancel := range s.activeLoads {
		cancels = append(cancels, cancel)
	}
	s.lifecycleMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.loadWG.Wait()
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
func (s *Service) Health() lifecycle.HealthSnapshot {
	if s == nil {
		return lifecycle.HealthSnapshot{}
	}
	s.lifecycleMu.Lock()
	started, stopped := s.started, s.stopped
	s.lifecycleMu.Unlock()
	return lifecycle.HealthSnapshot{Started: started, Stopped: stopped}
}

func (s *Service) Resolve(ctx context.Context, pool domain.CredentialPool) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.selector == nil || s.inspector == nil ||
		!s.inspector.SupportsInspection(pool.FixedProviderType, clientruntime.InspectModels) {
		if s != nil && s.now != nil {
			return fallbackResult(pool.FixedProviderType, s.now())
		}
		return fallbackResult(pool.FixedProviderType, time.Now())
	}

	if cached, ok := s.cachedResult(pool.ID); ok {
		return cached
	}

	ch := s.flights.DoChan(pool.ID, func() (any, error) {
		if cached, ok := s.cachedResult(pool.ID); ok {
			return cached, nil
		}
		fetchCtx, release, ok := s.beginLoad(ctx)
		if !ok {
			return Result{}, errServiceStopped
		}
		defer release()
		return s.refresh(fetchCtx, pool)
	})
	select {
	case <-ctx.Done():
		return s.staleOrFallback(pool)
	case outcome := <-ch:
		if outcome.Err == nil {
			return cloneResult(outcome.Val.(Result))
		}
		if errors.Is(outcome.Err, errServiceStopped) {
			return s.staleOrFallback(pool)
		}
		s.logger.Warn("fixed-provider model discovery failed",
			zap.String("pool_id", pool.ID),
			zap.String("fixed_provider_type", string(pool.FixedProviderType)),
			zap.Error(outcome.Err),
		)
		return s.staleOrFallback(pool)
	}
}

func (s *Service) beginLoad(callerCtx context.Context) (context.Context, func(), bool) {
	base := context.WithoutCancel(callerCtx)
	timeout := s.fetchTimeout
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	fetchCtx, cancel := context.WithTimeout(base, timeout)
	s.lifecycleMu.Lock()
	if s.stopped {
		s.lifecycleMu.Unlock()
		cancel()
		return nil, nil, false
	}
	if s.activeLoads == nil {
		s.activeLoads = make(map[uint64]context.CancelFunc)
	}
	ownerCtx := s.ownerCtx
	s.nextLoadID++
	id := s.nextLoadID
	s.activeLoads[id] = cancel
	s.loadWG.Add(1)
	s.lifecycleMu.Unlock()

	stopOwner := func() {}
	if ownerCtx != nil {
		stop := context.AfterFunc(ownerCtx, cancel)
		stopOwner = func() { stop() }
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			stopOwner()
			cancel()
			s.lifecycleMu.Lock()
			delete(s.activeLoads, id)
			s.lifecycleMu.Unlock()
			s.loadWG.Done()
		})
	}
	return fetchCtx, release, true
}

func (s *Service) cachedResult(poolID string) (Result, bool) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[poolID]
	if !ok {
		return Result{}, false
	}
	if !now.Before(entry.expiresAt) {
		if now.Before(entry.nextAttempt) && len(entry.result.Models) > 0 {
			result := cloneResult(entry.result)
			result.Source = "stale"
			return result, true
		}
		return Result{}, false
	}
	result := cloneResult(entry.result)
	result.Source = "cache"
	return result, true
}

func (s *Service) refresh(ctx context.Context, pool domain.CredentialPool) (Result, error) {
	now := s.now()
	s.mu.Lock()
	entry := s.entries[pool.ID]
	if now.Before(entry.nextAttempt) {
		s.mu.Unlock()
		return Result{}, fmt.Errorf("model discovery retry is cooling down")
	}
	etag := entry.etag
	s.mu.Unlock()

	credential, err := s.selector.SelectCredentialFromPool(ctx, pool.ID, pool.OAuthStrategy)
	if err != nil {
		s.recordFailure(pool.ID, now)
		return Result{}, fmt.Errorf("select inspection credential: %w", err)
	}
	snapshot, err := s.inspector.Inspect(ctx, clientruntime.Inspection{
		Provider:    pool.FixedProviderType,
		Credential:  clientruntime.SnapshotCredential(credential),
		Want:        clientruntime.InspectModels,
		IfNoneMatch: etag,
	})
	if err != nil {
		s.recordFailure(pool.ID, now)
		return Result{}, fmt.Errorf("inspect provider models: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if snapshot.NotModified {
		entry = s.entries[pool.ID]
		if len(entry.result.Models) == 0 {
			entry.nextAttempt = now.Add(s.failureBackoff)
			s.entries[pool.ID] = entry
			return Result{}, fmt.Errorf("provider returned not-modified without a cached manifest")
		}
		entry.expiresAt = now.Add(s.cacheTTL)
		entry.nextAttempt = time.Time{}
		entry.result.ObservedAt = snapshot.ObservedAt
		s.entries[pool.ID] = entry
		result := cloneResult(entry.result)
		result.Source = "cache"
		return result, nil
	}
	if len(snapshot.Models) == 0 {
		entry.nextAttempt = now.Add(s.failureBackoff)
		s.entries[pool.ID] = entry
		return Result{}, fmt.Errorf("provider returned an empty model manifest")
	}
	result := Result{
		Models:          cloneModels(snapshot.Models),
		Source:          "live",
		ProfileRevision: snapshot.ProfileRevision,
		ObservedAt:      snapshot.ObservedAt,
	}
	s.entries[pool.ID] = cacheEntry{
		result:    cloneResult(result),
		etag:      snapshot.ETag,
		expiresAt: now.Add(s.cacheTTL),
	}
	return result, nil
}

func (s *Service) recordFailure(poolID string, now time.Time) {
	s.mu.Lock()
	entry := s.entries[poolID]
	entry.nextAttempt = now.Add(s.failureBackoff)
	s.entries[poolID] = entry
	s.mu.Unlock()
}

func (s *Service) staleOrFallback(pool domain.CredentialPool) Result {
	s.mu.Lock()
	entry := s.entries[pool.ID]
	s.mu.Unlock()
	if len(entry.result.Models) > 0 {
		result := cloneResult(entry.result)
		result.Source = "stale"
		return result
	}
	return fallbackResult(pool.FixedProviderType, s.now())
}

func FallbackModels(provider domain.FixedProviderType) []clientruntime.ModelCard {
	var ids []string
	switch provider {
	case domain.FixedProviderCodex:
		ids = []string{
			"gpt-5.6-sol",
			"gpt-5.6-terra",
			"gpt-5.6-luna",
			"gpt-5.5",
			"gpt-5.4",
			"gpt-5.4-mini",
			"gpt-5.2",
		}
	case domain.FixedProviderClaudeOAuth:
		ids = []string{
			"claude-fable-5",
			"claude-opus-5",
			"claude-sonnet-5",
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
			"claude-haiku-4-5-20251001",
		}
	case domain.FixedProviderGeminiCLI:
		ids = []string{
			"gemini-3.5-flash",
			"gemini-3.1-pro-preview",
			"gemini-3.1-pro-preview-customtools",
			"gemini-3.1-flash-image",
			"gemini-3-pro-preview",
			"gemini-3-flash-preview",
			"gemini-2.5-pro",
			"gemini-2.5-flash",
		}
	case domain.FixedProviderAntigravity:
		ids = []string{
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6-thinking",
			"claude-sonnet-4-6",
			"gemini-3.1-pro-high",
			"gemini-3.1-pro-low",
			"gemini-3.1-flash-image",
			"gemini-3-flash",
		}
	default:
		return []clientruntime.ModelCard{}
	}
	models := make([]clientruntime.ModelCard, 0, len(ids))
	for _, id := range ids {
		models = append(models, clientruntime.ModelCard{ID: id})
	}
	return models
}

func fallbackResult(provider domain.FixedProviderType, observedAt time.Time) Result {
	return Result{
		Models:          FallbackModels(provider),
		Source:          "fallback",
		ProfileRevision: FallbackRevision,
		ObservedAt:      observedAt.UTC(),
	}
}

func cloneResult(result Result) Result {
	result.Models = cloneModels(result.Models)
	return result
}

func cloneModels(models []clientruntime.ModelCard) []clientruntime.ModelCard {
	cloned := make([]clientruntime.ModelCard, 0, len(models))
	for _, model := range models {
		card := clientruntime.ModelCard{ID: model.ID}
		if model.Capabilities != nil {
			card.Capabilities = make(map[string]any, len(model.Capabilities))
			for key, value := range model.Capabilities {
				card.Capabilities[key] = value
			}
		}
		cloned = append(cloned, card)
	}
	sort.Slice(cloned, func(i, j int) bool { return cloned[i].ID < cloned[j].ID })
	return cloned
}

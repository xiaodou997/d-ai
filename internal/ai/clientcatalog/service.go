package clientcatalog

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/lifecycle"
)

const (
	DefaultCacheTTL       = 10 * time.Minute
	DefaultFailureBackoff = time.Minute
	DefaultFetchTimeout   = 15 * time.Second
	FallbackRevision      = "builtin-model-catalog@2026-10-17"
)

type CredentialReader interface {
	ListModelCredentials(context.Context, string) ([]domain.CredentialModelIdentity, error)
	GetDecryptedByID(context.Context, string) (*domain.OAuthCredential, error)
}

type Result struct {
	Models          []clientruntime.ModelCard
	Source          string
	ProfileRevision string
	ObservedAt      time.Time
	Accounts        []domain.CredentialModelCatalog
}

type cacheEntry struct {
	identity    domain.CredentialModelIdentity
	result      Result
	etag        string
	expiresAt   time.Time
	nextAttempt time.Time
}

// Service owns live fixed-provider model discovery, cache revalidation,
// stale-if-error behavior, and versioned built-in fallbacks.
type Service struct {
	reader    CredentialReader
	inspector clientruntime.Inspector
	logger    *zap.Logger

	cacheTTL       time.Duration
	failureBackoff time.Duration
	fetchTimeout   time.Duration
	now            func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
	pending map[domain.CredentialModelIdentity]*accountLoad
	slots   chan struct{}

	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	ownerCtx    context.Context
	nextLoadID  uint64
	activeLoads map[uint64]context.CancelFunc
	loadWG      sync.WaitGroup
}

var _ lifecycle.Component = (*Service)(nil)

func New(reader CredentialReader, inspector clientruntime.Inspector, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{
		reader:         reader,
		inspector:      inspector,
		logger:         logger,
		cacheTTL:       DefaultCacheTTL,
		failureBackoff: DefaultFailureBackoff,
		fetchTimeout:   DefaultFetchTimeout,
		now:            time.Now,
		entries:        make(map[string]cacheEntry),
		pending:        make(map[domain.CredentialModelIdentity]*accountLoad),
		slots:          make(chan struct{}, 4),
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
// account workers to leave the credential reader/inspector path. It is safe to call
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

func FallbackModels(provider domain.FixedProviderType) []clientruntime.ModelCard {
	var ids []string
	switch provider {
	case domain.FixedProviderCodex:
		ids = []string{
			"gpt-6-astra",
			"gpt-6.1-sol",
			"gpt-6-sol",
			"gpt-6-luna",
			"gpt-5.6-sol",
			"gpt-5.6-terra",
			"gpt-5.6-luna",
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

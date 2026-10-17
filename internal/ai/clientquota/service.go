// Package clientquota owns bounded, read-only subscription quota inspection.
package clientquota

import (
	"context"
	"sync"
	"time"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/lifecycle"
)

type Store interface {
	ListPools(context.Context) ([]domain.CredentialPool, error)
	ListModelCredentials(context.Context, string) ([]domain.CredentialModelIdentity, error)
	GetDecryptedByID(context.Context, string) (*domain.OAuthCredential, error)
	ObserveCredentialQuota(context.Context, string, int64, domain.CredentialQuota) error
}

type observation struct {
	identity domain.CredentialModelIdentity
	result   domain.CredentialQuotaInspection
	next     time.Time
}
type load struct {
	done        chan struct{}
	observation observation
}

type Service struct {
	store             Store
	inspector         clientruntime.Inspector
	ctx               context.Context
	cancel            context.CancelFunc
	mu                sync.Mutex
	started, stopped  bool
	stopParent        func() bool
	entries           map[string]observation
	pending           map[domain.CredentialModelIdentity]*load
	slots             chan struct{}
	wg                sync.WaitGroup
	interval, timeout time.Duration
}

func New(store Store, inspector clientruntime.Inspector) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{store: store, inspector: inspector, ctx: ctx, cancel: cancel, entries: map[string]observation{}, pending: map[domain.CredentialModelIdentity]*load{}, slots: make(chan struct{}, 4), interval: 5 * time.Minute, timeout: 12 * time.Second}
}

func (s *Service) Start(parent context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped {
		return
	}
	s.started = true
	s.stopParent = context.AfterFunc(parent, s.cancel)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			pools, err := s.store.ListPools(s.ctx)
			if err == nil {
				for _, pool := range pools {
					if s.ctx.Err() != nil {
						return
					}
					if pool.Status == "active" {
						_, _ = s.RefreshPool(s.ctx, pool.ID, false)
					}
				}
			}
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.stopped = true
	s.cancel()
	if s.stopParent != nil {
		s.stopParent()
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Health() lifecycle.HealthSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return lifecycle.HealthSnapshot{Started: s.started, Stopped: s.stopped}
}

// RefreshPool joins per-account loads. Cancelling one HTTP reader never cancels
// another reader's shared inspection; shutdown still cancels every native call.
func (s *Service) RefreshPool(ctx context.Context, poolID string, force bool) (map[string]domain.CredentialQuotaInspection, error) {
	refs, err := s.store.ListModelCredentials(ctx, poolID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	loads := make(map[string]*load, len(refs))
	for _, ref := range refs {
		loads[ref.ID] = s.request(ref, force)
	}
	for _, l := range loads {
		select {
		case <-l.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	// Account edits/disable/delete during a query must not publish stale health.
	current, err := s.store.ListModelCredentials(ctx, poolID)
	if err != nil {
		return nil, err
	}
	out := map[string]domain.CredentialQuotaInspection{}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ref := range current {
		if l := loads[ref.ID]; l != nil && l.observation.identity == ref {
			out[ref.ID] = l.observation.result
			continue
		}
		entry, ok := s.entries[ref.ID]
		if ok && entry.identity == ref {
			out[ref.ID] = entry.result
		} else {
			out[ref.ID] = domain.CredentialQuotaInspection{State: "unknown"}
		}
	}
	return out, nil
}

func (s *Service) request(ref domain.CredentialModelIdentity, force bool) *load {
	s.mu.Lock()
	defer s.mu.Unlock()
	finished := func(result domain.CredentialQuotaInspection) *load {
		l := &load{done: make(chan struct{}), observation: observation{identity: ref, result: result}}
		close(l.done)
		return l
	}
	if s.stopped || s.ctx.Err() != nil {
		return finished(domain.CredentialQuotaInspection{State: "unknown"})
	}
	if l := s.pending[ref]; l != nil {
		return l
	}
	if entry, ok := s.entries[ref.ID]; ok && entry.identity == ref && time.Now().Before(entry.next) {
		// Manual clicks may bypass the normal interval, with a short shared
		// throttle to avoid repeatedly hitting upstream quota endpoints.
		if !force || time.Now().UnixMilli()-entry.result.ObservedAt < 10_000 {
			result := entry.result
			if result.State == "live" {
				result.State = "cache"
			}
			return finished(result)
		}
	}
	l := &load{done: make(chan struct{})}
	s.pending[ref] = l
	s.wg.Add(1)
	go s.fetch(ref, l)
	return l
}

func (s *Service) fetch(ref domain.CredentialModelIdentity, l *load) {
	defer s.wg.Done()
	entry := observation{identity: ref, result: domain.CredentialQuotaInspection{State: "failed", ObservedAt: time.Now().UnixMilli()}, next: time.Now().Add(time.Minute)}
	defer func() {
		s.mu.Lock()
		entry.result.ObservedAt = time.Now().UnixMilli()
		if old, ok := s.entries[ref.ID]; !ok || old.identity.TokenVersion <= entry.identity.TokenVersion {
			s.entries[ref.ID] = entry
		}
		l.observation = entry
		delete(s.pending, ref)
		close(l.done)
		s.mu.Unlock()
	}()
	if s.inspector == nil || !s.inspector.SupportsInspection(ref.ProviderType, clientruntime.InspectQuota) {
		entry.result.State = "unsupported"
		entry.next = time.Now().Add(s.interval)
		return
	}
	// Time waiting for a concurrency slot does not consume the native timeout.
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-s.ctx.Done():
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()
	credential, err := s.store.GetDecryptedByID(ctx, ref.ID)
	if err != nil || credential == nil || credential.TokenVersion != ref.TokenVersion || credential.PoolID != ref.PoolID || credential.ProviderType != ref.ProviderType || credential.Status != "active" {
		return
	}
	snapshot, err := s.inspector.Inspect(ctx, clientruntime.Inspection{Provider: ref.ProviderType, Credential: clientruntime.SnapshotCredential(credential), Want: clientruntime.InspectQuota})
	if err != nil {
		return
	}
	version := snapshot.TokenVersion
	for _, quota := range snapshot.Quotas {
		if err = s.store.ObserveCredentialQuota(ctx, ref.ID, version, quota); err != nil {
			return
		}
	}
	current, err := s.store.GetDecryptedByID(ctx, ref.ID)
	if err != nil || current == nil || current.Status != "active" || current.TokenVersion != version || current.PoolID != ref.PoolID {
		return
	}
	entry.identity.TokenVersion = version
	entry.result.State = "live"
	if len(snapshot.Quotas) == 0 {
		entry.result.State = "unknown"
	}
	entry.next = time.Now().Add(s.interval)
}

var _ lifecycle.Component = (*Service)(nil)

package clientcatalog

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
)

type accountLoad struct {
	done   chan struct{}
	result Result
}

func (s *Service) supports(provider domain.FixedProviderType) bool {
	return s != nil && s.reader != nil && s.inspector != nil && s.inspector.SupportsInspection(provider, clientruntime.InspectModels)
}

// ReadCredentialModels is the serving path: one non-secret database read and
// cache reads only. At most four background inspections run across all pools.
// A cold cache never adds provider latency to the user's generation request.
func (s *Service) ReadCredentialModels(ctx context.Context, poolID string) (map[string]domain.CredentialModelCatalog, error) {
	out := map[string]domain.CredentialModelCatalog{}
	if s == nil || s.reader == nil {
		return out, nil
	}
	refs, err := s.reader.ListModelCredentials(ctx, poolID)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		result, until := s.accountSnapshot(ref)
		out[ref.ID] = accountCatalog(ref.ID, result, until)
		if s.supports(ref.ProviderType) {
			s.loadAccount(ctx, ref, false, false)
		}
	}
	return out, nil
}

// Resolve reports the union of current accounts' observed models. Built-in
// fallback models remain suggestions only and are never written to an account.
func (s *Service) Resolve(ctx context.Context, pool domain.CredentialPool) Result {
	return s.resolve(ctx, pool, false)
}

func (s *Service) ResolveFresh(ctx context.Context, pool domain.CredentialPool) Result {
	return s.resolve(ctx, pool, true)
}

func (s *Service) resolve(ctx context.Context, pool domain.CredentialPool, force bool) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.reader == nil {
		return fallbackResult(pool.FixedProviderType, timeNow(s))
	}
	ctx, cancel := context.WithTimeout(ctx, s.fetchTimeout)
	defer cancel()
	refs, err := s.reader.ListModelCredentials(ctx, pool.ID)
	if err != nil {
		s.logger.Warn("model discovery identities unavailable", zap.String("pool_id", pool.ID), zap.Error(err))
		return fallbackResult(pool.FixedProviderType, s.now())
	}
	results := make([]Result, len(refs))
	jobs := make(chan int, len(refs))
	for i := range refs {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for range min(4, len(refs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				ref := refs[i]
				if ref.ProviderType != pool.FixedProviderType || !s.supports(ref.ProviderType) {
					continue
				}
				load := s.loadAccount(ctx, ref, true, force)
				if load != nil {
					select {
					case <-ctx.Done():
						continue
					case <-load.done:
						results[i] = cloneResult(load.result)
					}
				}
			}
		}()
	}
	workers.Wait()

	// Re-read revisions after waiting: an old inspection may finish after an
	// account edit, disable, deletion or refresh. Such a result is not current.
	current, err := s.reader.ListModelCredentials(ctx, pool.ID)
	if err != nil {
		return fallbackResult(pool.FixedProviderType, s.now())
	}
	observed := make(map[domain.CredentialModelIdentity]Result, len(refs))
	for i, ref := range refs {
		observed[ref] = results[i]
	}
	out := Result{Models: []clientruntime.ModelCard{}, Source: "cache", Accounts: []domain.CredentialModelCatalog{}}
	models := map[string]clientruntime.ModelCard{}
	for _, ref := range current {
		result, until := s.accountSnapshot(ref)
		if live := observed[ref]; live.Source != "" {
			result = live
		}
		out.Accounts = append(out.Accounts, accountCatalog(ref.ID, result, until))
		if result.Source == "unknown" || result.Source == "" {
			continue
		}
		for _, model := range result.Models {
			models[model.ID] = model
		}
		if out.ProfileRevision == "" {
			out.ProfileRevision = result.ProfileRevision
		}
		if out.ObservedAt.IsZero() || result.ObservedAt.Before(out.ObservedAt) {
			out.ObservedAt = result.ObservedAt
		}
		if result.Source == "stale" || out.Source != "stale" && result.Source == "live" {
			out.Source = result.Source
		}
	}
	for _, card := range models {
		out.Models = append(out.Models, card)
	}
	if len(out.Models) == 0 {
		fallback := fallbackResult(pool.FixedProviderType, s.now())
		fallback.Accounts = out.Accounts
		return fallback
	}
	out.Models = cloneModels(out.Models)
	sort.Slice(out.Accounts, func(i, j int) bool { return out.Accounts[i].CredentialID < out.Accounts[j].CredentialID })
	return out
}

func (s *Service) accountSnapshot(ref domain.CredentialModelIdentity) (Result, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[ref.ID]
	if entry.identity != ref || len(entry.result.Models) == 0 {
		return Result{Source: "unknown", Models: []clientruntime.ModelCard{}}, 0
	}
	result := cloneResult(entry.result)
	result.Source = "cache"
	if !s.now().Before(entry.expiresAt) {
		result.Source = "stale"
	}
	return result, entry.expiresAt.UnixMilli()
}

func (s *Service) needsLoad(ref domain.CredentialModelIdentity, force bool) bool {
	entry, exists := s.entries[ref.ID]
	if exists && entry.identity.TokenVersion > ref.TokenVersion {
		return false
	}
	return force || entry.identity != ref || !s.now().Before(entry.expiresAt) && !s.now().Before(entry.nextAttempt)
}

// loadAccount deduplicates by identity and revision, including ETag validators.
// Background admission is non-blocking; explicit management discovery may wait
// for a slot, but is still bounded by its caller and the service lifecycle.
func (s *Service) loadAccount(ctx context.Context, ref domain.CredentialModelIdentity, wait, force bool) *accountLoad {
	if ctx.Err() != nil {
		return nil
	}
	s.mu.Lock()
	if load := s.pending[ref]; load != nil {
		s.mu.Unlock()
		return load
	}
	needed := s.needsLoad(ref, force)
	s.mu.Unlock()
	if !needed {
		return nil
	}
	if wait {
		select {
		case s.slots <- struct{}{}:
		case <-ctx.Done():
			return nil
		}
	} else {
		select {
		case s.slots <- struct{}{}:
		default:
			return nil
		}
	}
	s.mu.Lock()
	if load := s.pending[ref]; load != nil {
		s.mu.Unlock()
		<-s.slots
		return load
	}
	if !s.needsLoad(ref, force) {
		s.mu.Unlock()
		<-s.slots
		return nil
	}
	shared, release, ok := s.beginLoad(ctx)
	if !ok {
		s.mu.Unlock()
		<-s.slots
		return nil
	}
	entry := s.entries[ref.ID]
	if entry.identity != ref {
		entry = cacheEntry{identity: ref}
		s.entries[ref.ID] = entry
	}
	load := &accountLoad{done: make(chan struct{})}
	s.pending[ref] = load
	s.mu.Unlock()
	go func() {
		defer release()
		defer func() { <-s.slots }()
		result, etag, err := s.inspectAccount(shared, ref, entry)
		s.mu.Lock()
		current := s.entries[ref.ID]
		if current.identity == ref {
			if err != nil {
				current.nextAttempt = s.now().Add(s.failureBackoff)
				s.entries[ref.ID] = current
			} else {
				s.entries[ref.ID] = cacheEntry{identity: ref, result: result, etag: etag, expiresAt: s.now().Add(s.cacheTTL)}
				load.result = cloneResult(result)
			}
		}
		delete(s.pending, ref)
		close(load.done)
		s.mu.Unlock()
		if err != nil && shared.Err() == nil {
			s.logger.Warn("account model discovery failed", zap.String("credential_id", ref.ID), zap.Error(err))
		}
	}()
	return load
}

func (s *Service) inspectAccount(ctx context.Context, ref domain.CredentialModelIdentity, entry cacheEntry) (Result, string, error) {
	credential, err := s.reader.GetDecryptedByID(ctx, ref.ID)
	if err != nil {
		return Result{}, "", fmt.Errorf("read inspection credential: %w", err)
	}
	if credential == nil || credential.ID != ref.ID || credential.PoolID != ref.PoolID || credential.ProviderType != ref.ProviderType || credential.TokenVersion != ref.TokenVersion || credential.Status != domain.OAuthCredentialActive {
		return Result{}, "", domain.ErrConflict
	}
	snapshot, err := s.inspector.Inspect(ctx, clientruntime.Inspection{
		Provider: ref.ProviderType, Credential: clientruntime.SnapshotCredential(credential), Want: clientruntime.InspectModels, IfNoneMatch: entry.etag,
	})
	if err != nil {
		return Result{}, "", err
	}
	observedAt := snapshot.ObservedAt
	if observedAt.IsZero() {
		observedAt = s.now().UTC()
	}
	if snapshot.NotModified {
		if len(entry.result.Models) == 0 {
			return Result{}, "", fmt.Errorf("not-modified without a cached account manifest")
		}
		result := cloneResult(entry.result)
		result.Source, result.ObservedAt = "cache", observedAt
		return result, entry.etag, nil
	}
	models := []clientruntime.ModelCard{}
	seen := map[string]bool{}
	for _, card := range snapshot.Models {
		card.ID = strings.TrimSpace(card.ID)
		if card.ID != "" && !seen[card.ID] {
			models = append(models, card)
			seen[card.ID] = true
		}
	}
	if len(models) == 0 {
		return Result{}, "", fmt.Errorf("provider returned an empty account model manifest")
	}
	return Result{Models: cloneModels(models), Source: "live", ProfileRevision: snapshot.ProfileRevision, ObservedAt: observedAt}, snapshot.ETag, nil
}

func accountCatalog(id string, result Result, until int64) domain.CredentialModelCatalog {
	out := domain.CredentialModelCatalog{CredentialID: id, Models: []string{}, Source: "unknown", ValidUntil: until}
	if result.Source != "" {
		out.Source = result.Source
	}
	if !result.ObservedAt.IsZero() {
		out.ObservedAt = result.ObservedAt.UnixMilli()
	}
	for _, model := range result.Models {
		out.Models = append(out.Models, model.ID)
	}
	return out
}

func timeNow(s *Service) time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

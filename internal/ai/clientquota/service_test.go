package clientquota

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
)

type quotaStore struct {
	mu          sync.Mutex
	credentials map[string]domain.OAuthCredential
	quotas      map[string][]domain.CredentialQuota
}

func (s *quotaStore) ListPools(context.Context) ([]domain.CredentialPool, error) {
	return []domain.CredentialPool{{ID: "pool", Status: "active"}}, nil
}
func (s *quotaStore) ListModelCredentials(context.Context, string) ([]domain.CredentialModelIdentity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.CredentialModelIdentity{}
	for _, c := range s.credentials {
		if c.Status == "active" {
			out = append(out, domain.CredentialModelIdentity{ID: c.ID, PoolID: c.PoolID, ProviderType: c.ProviderType, TokenVersion: c.TokenVersion})
		}
	}
	return out, nil
}
func (s *quotaStore) GetDecryptedByID(_ context.Context, id string) (*domain.OAuthCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.credentials[id]
	return &c, nil
}
func (s *quotaStore) ObserveCredentialQuota(_ context.Context, id string, version int64, q domain.CredentialQuota) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.credentials[id]; c.TokenVersion == version && c.Status == "active" {
		s.quotas[id] = append(s.quotas[id], q)
	}
	return nil
}

type inspectFunc func(context.Context, clientruntime.Inspection) (clientruntime.InspectionSnapshot, error)

func (f inspectFunc) SupportsInspection(domain.FixedProviderType, clientruntime.InspectionWant) bool {
	return true
}
func (f inspectFunc) Inspect(ctx context.Context, in clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
	return f(ctx, in)
}
func testStore() *quotaStore {
	return &quotaStore{credentials: map[string]domain.OAuthCredential{"c": {ID: "c", PoolID: "pool", ProviderType: domain.FixedProviderCodex, TokenVersion: 1, AccessToken: "secret", Status: "active"}}, quotas: map[string][]domain.CredentialQuota{}}
}
func quotaSnapshot(version int64) clientruntime.InspectionSnapshot {
	remaining := float64(50)
	return clientruntime.InspectionSnapshot{TokenVersion: version, Quotas: []domain.CredentialQuota{{Source: "quota_api", ObservedAt: time.Now().UnixMilli(), Windows: []domain.CredentialQuotaWindow{{Key: "primary", Remaining: &remaining}}}}}
}

func TestQuotaRefreshJoinsReadersAndSurvivesOneCancellation(t *testing.T) {
	store := testStore()
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	s := New(store, inspectFunc(func(ctx context.Context, in clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return quotaSnapshot(in.Credential.TokenVersion), nil
		case <-ctx.Done():
			return clientruntime.InspectionSnapshot{}, ctx.Err()
		}
	}))
	defer s.Stop(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := s.RefreshPool(ctx, "pool", true); first <- err }()
	<-started
	cancel()
	if !errors.Is(<-first, context.Canceled) {
		t.Fatal("reader did not cancel")
	}
	second := make(chan map[string]domain.CredentialQuotaInspection, 1)
	go func() { out, _ := s.RefreshPool(context.Background(), "pool", true); second <- out }()
	close(release)
	if out := <-second; out["c"].State != "live" && out["c"].State != "cache" {
		t.Fatalf("result=%+v", out)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d", calls.Load())
	}
	out, err := s.RefreshPool(context.Background(), "pool", true)
	if err != nil || out["c"].State != "cache" || calls.Load() != 1 {
		t.Fatalf("manual throttle=%+v calls=%d err=%v", out, calls.Load(), err)
	}
}

func TestQuotaRefreshCannotRestoreAnEditedIdentity(t *testing.T) {
	store := testStore()
	started, release := make(chan struct{}), make(chan struct{})
	s := New(store, inspectFunc(func(context.Context, clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
		close(started)
		<-release
		return quotaSnapshot(1), nil
	}))
	defer s.Stop(context.Background())
	done := make(chan map[string]domain.CredentialQuotaInspection, 1)
	go func() { out, _ := s.RefreshPool(context.Background(), "pool", true); done <- out }()
	<-started
	store.mu.Lock()
	c := store.credentials["c"]
	c.TokenVersion = 2
	store.credentials["c"] = c
	store.mu.Unlock()
	close(release)
	if out := <-done; out["c"].State != "unknown" {
		t.Fatalf("late identity observation=%+v", out)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.quotas["c"]) != 0 {
		t.Fatal("old identity quota restored")
	}
}

func TestQuotaFailurePreservesFactsAndDoesNotChangeAccountStatus(t *testing.T) {
	store := testStore()
	store.quotas["c"] = quotaSnapshot(1).Quotas
	s := New(store, inspectFunc(func(context.Context, clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
		return clientruntime.InspectionSnapshot{}, errors.New("provider denied")
	}))
	defer s.Stop(context.Background())
	out, err := s.RefreshPool(context.Background(), "pool", true)
	if err != nil || out["c"].State != "failed" || len(store.quotas["c"]) != 1 || store.credentials["c"].Status != "active" {
		t.Fatalf("failure=%+v err=%v", out, err)
	}
}

func TestQuotaShutdownCancelsNativeCalls(t *testing.T) {
	started := make(chan struct{})
	s := New(testStore(), inspectFunc(func(ctx context.Context, _ clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
		close(started)
		<-ctx.Done()
		return clientruntime.InspectionSnapshot{}, ctx.Err()
	}))
	done := make(chan struct{})
	go func() { s.RefreshPool(context.Background(), "pool", true); close(done) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("shared quota load leaked")
	}
}

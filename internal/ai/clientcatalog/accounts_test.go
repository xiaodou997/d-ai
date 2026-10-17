package clientcatalog

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
)

func modelReader(ids ...string) *fixtureReader {
	r := &fixtureReader{credentials: map[string]*domain.OAuthCredential{}}
	for _, id := range ids {
		r.credentials[id] = &domain.OAuthCredential{ID: id, PoolID: "pool", ProviderType: domain.FixedProviderCodex, Status: domain.OAuthCredentialActive, TokenVersion: 1, AccessToken: "test-" + id}
	}
	return r
}

func stopCatalog(t *testing.T, s *Service) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Stop(ctx); err != nil {
			t.Errorf("stop model catalog: %v", err)
		}
	})
}

func TestAccountCatalogUnionAndETagsStayScopedToEachAccount(t *testing.T) {
	reader := modelReader("a", "b")
	var calls atomic.Int32
	s := New(reader, inspectorStub{supported: true, inspect: func(_ context.Context, in clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
		calls.Add(1)
		etag := `"` + in.Credential.ID + `"`
		if in.IfNoneMatch != "" {
			if in.IfNoneMatch != etag {
				t.Errorf("cross-account ETag: account=%s etag=%s", in.Credential.ID, in.IfNoneMatch)
			}
			return clientruntime.InspectionSnapshot{NotModified: true}, nil
		}
		return clientruntime.InspectionSnapshot{Models: []clientruntime.ModelCard{{ID: "model-" + in.Credential.ID}, {ID: "shared"}}, ETag: etag}, nil
	}}, nil)
	stopCatalog(t, s)
	pool := domain.CredentialPool{ID: "pool", FixedProviderType: domain.FixedProviderCodex}
	result := s.Resolve(context.Background(), pool)
	if len(result.Models) != 3 || len(result.Accounts) != 2 || result.Source != "live" {
		t.Fatalf("pool union: %+v", result)
	}
	accounts, err := s.ReadCredentialModels(context.Background(), "pool")
	if err != nil || !accounts["a"].Excludes("model-b", time.Now().UnixMilli()) || accounts["b"].Excludes("model-b", time.Now().UnixMilli()) {
		t.Fatalf("independent account permissions: %+v error=%v", accounts, err)
	}
	s.Resolve(context.Background(), pool)
	if calls.Load() != 2 {
		t.Fatalf("cached pool unexpectedly inspected accounts: %d calls", calls.Load())
	}
	refreshed := s.ResolveFresh(context.Background(), pool)
	if calls.Load() != 4 || len(refreshed.Models) != 3 {
		t.Fatalf("account revalidation: calls=%d result=%+v", calls.Load(), refreshed)
	}
	reader.mu.Lock()
	reader.credentials["b"].Status = domain.OAuthCredentialDisabled
	reader.mu.Unlock()
	afterDisable := s.Resolve(context.Background(), pool)
	if len(afterDisable.Accounts) != 1 || len(afterDisable.Models) != 2 {
		t.Fatalf("disabled account remained in pool catalog: %+v", afterDisable)
	}
	reader.mu.Lock()
	clear(reader.credentials)
	reader.mu.Unlock()
	if empty := s.Resolve(context.Background(), pool); empty.Source != "fallback" || len(empty.Accounts) != 0 {
		t.Fatalf("removed account observations survived in the pool catalog: %+v", empty)
	}
}

func TestServingDiscoveryIsNonblockingAndGloballyBounded(t *testing.T) {
	ids := []string{}
	for i := 0; i < 40; i++ {
		ids = append(ids, fmt.Sprintf("account-%02d", i))
	}
	reader := modelReader(ids...)
	started := make(chan struct{}, 40)
	var calls atomic.Int32
	s := New(reader, inspectorStub{supported: true, inspect: func(ctx context.Context, _ clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
		calls.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		return clientruntime.InspectionSnapshot{}, ctx.Err()
	}}, nil)
	stopCatalog(t, s)
	done := make(chan map[string]domain.CredentialModelCatalog, 1)
	go func() {
		catalog, _ := s.ReadCredentialModels(context.Background(), "pool")
		done <- catalog
	}()
	select {
	case catalog := <-done:
		if len(catalog) != 40 || catalog[ids[0]].Source != "unknown" {
			t.Fatalf("cold account catalog: %+v", catalog)
		}
	case <-time.After(time.Second):
		t.Fatal("serving waited for provider inspection")
	}
	for range 4 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("background discovery never started")
		}
	}
	for range 10 {
		if _, err := s.ReadCredentialModels(context.Background(), "pool"); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("background discovery exceeded four calls: %d", calls.Load())
	}
}

func TestOldAccountProbeCannotOverwriteEditedIdentity(t *testing.T) {
	reader := modelReader("a")
	oldStarted, releaseOld := make(chan struct{}), make(chan struct{})
	s := New(reader, inspectorStub{supported: true, inspect: func(ctx context.Context, in clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
		if in.Credential.TokenVersion == 1 {
			close(oldStarted)
			select {
			case <-releaseOld:
			case <-ctx.Done():
				return clientruntime.InspectionSnapshot{}, ctx.Err()
			}
			return clientruntime.InspectionSnapshot{Models: []clientruntime.ModelCard{{ID: "old-model"}}}, nil
		}
		if in.IfNoneMatch != "" {
			t.Error("new credential revision reused old validator")
		}
		return clientruntime.InspectionSnapshot{Models: []clientruntime.ModelCard{{ID: "new-model"}}}, nil
	}}, nil)
	stopCatalog(t, s)
	s.ReadCredentialModels(context.Background(), "pool")
	<-oldStarted
	reader.mu.Lock()
	reader.credentials["a"].TokenVersion = 2
	reader.credentials["a"].AccessToken = "replacement"
	reader.mu.Unlock()
	result := s.Resolve(context.Background(), domain.CredentialPool{ID: "pool", FixedProviderType: domain.FixedProviderCodex})
	if len(result.Models) != 1 || result.Models[0].ID != "new-model" {
		t.Fatalf("new identity did not discover independently: %+v", result)
	}
	s.mu.Lock()
	oldLoad := s.pending[domain.CredentialModelIdentity{ID: "a", PoolID: "pool", ProviderType: domain.FixedProviderCodex, TokenVersion: 1}]
	s.mu.Unlock()
	close(releaseOld)
	<-oldLoad.done
	accounts, _ := s.ReadCredentialModels(context.Background(), "pool")
	if !slices.Equal(accounts["a"].Models, []string{"new-model"}) {
		t.Fatalf("late observation overwrote edited account: %+v", accounts)
	}
}

func TestExpiredFailedAndUnsupportedDiscoveryNeverExcludesAccounts(t *testing.T) {
	reader := modelReader("a")
	var fail atomic.Bool
	var calls atomic.Int32
	s := New(reader, inspectorStub{supported: true, inspect: func(context.Context, clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
		calls.Add(1)
		if fail.Load() {
			return clientruntime.InspectionSnapshot{}, errors.New("temporary provider failure")
		}
		return clientruntime.InspectionSnapshot{Models: []clientruntime.ModelCard{{ID: "known"}}}, nil
	}}, nil)
	stopCatalog(t, s)
	pool := domain.CredentialPool{ID: "pool", FixedProviderType: domain.FixedProviderCodex}
	s.Resolve(context.Background(), pool)
	fail.Store(true)
	s.mu.Lock()
	entry := s.entries["a"]
	entry.expiresAt = time.Now().Add(-time.Second)
	s.entries["a"] = entry
	s.mu.Unlock()
	stale := s.Resolve(context.Background(), pool)
	if stale.Source != "stale" || len(stale.Accounts) != 1 || stale.Accounts[0].Excludes("new-model", time.Now().UnixMilli()) {
		t.Fatalf("expired manifest treated as authorization: %+v", stale)
	}
	s.Resolve(context.Background(), pool)
	if calls.Load() != 2 {
		t.Fatalf("failed discovery did not back off: %d calls", calls.Load())
	}
	unsupported := New(reader, inspectorStub{supported: false}, nil)
	unknown := unsupported.Resolve(context.Background(), pool)
	if unknown.Source != "fallback" || len(unknown.Accounts) != 1 || unknown.Accounts[0].Source != "unknown" || unknown.Accounts[0].Excludes("any-model", time.Now().UnixMilli()) {
		t.Fatalf("fallback granted or denied account permissions: %+v", unknown)
	}
}

func TestCodexFallbackContainsCurrentSubscriptionSuggestionsOnly(t *testing.T) {
	ids := []string{}
	for _, card := range FallbackModels(domain.FixedProviderCodex) {
		ids = append(ids, card.ID)
	}
	for _, retired := range []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.2"} {
		if slices.Contains(ids, retired) {
			t.Fatalf("retired subscription model remains suggested: %s", retired)
		}
	}
	if !slices.Contains(ids, "gpt-6.1-sol") || !slices.Contains(ids, "gpt-6-luna") {
		t.Fatalf("current subscription suggestions missing: %v", ids)
	}
}

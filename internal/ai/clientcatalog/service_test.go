package clientcatalog

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
)

type selectorFunc func(context.Context, string, string) (*domain.OAuthCredential, error)

func (f selectorFunc) SelectCredentialFromPool(ctx context.Context, poolID, strategy string) (*domain.OAuthCredential, error) {
	return f(ctx, poolID, strategy)
}

type inspectorStub struct {
	supported bool
	inspect   func(context.Context, clientruntime.Inspection) (clientruntime.InspectionSnapshot, error)
}

func (s inspectorStub) SupportsInspection(domain.FixedProviderType, clientruntime.InspectionWant) bool {
	return s.supported
}

func (s inspectorStub) Inspect(ctx context.Context, in clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
	return s.inspect(ctx, in)
}

func TestServiceDiscoversThenCachesPoolModels(t *testing.T) {
	var selections atomic.Int32
	var inspections atomic.Int32
	service := New(selectorFunc(func(_ context.Context, poolID, strategy string) (*domain.OAuthCredential, error) {
		selections.Add(1)
		if poolID != "pool-1" || strategy != "round_robin" {
			t.Fatalf("selection = %q, %q", poolID, strategy)
		}
		return &domain.OAuthCredential{ID: "credential-1", AccessToken: "token"}, nil
	}), inspectorStub{
		supported: true,
		inspect: func(_ context.Context, in clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
			inspections.Add(1)
			if in.Credential.ID != "credential-1" || in.Want != clientruntime.InspectModels {
				t.Fatalf("inspection = %#v", in)
			}
			return clientruntime.InspectionSnapshot{
				ProfileRevision: "codex-cli@test",
				Models: []clientruntime.ModelCard{
					{ID: "gpt-z"},
					{ID: "gpt-a"},
				},
				ETag:       `"v1"`,
				Source:     "live",
				ObservedAt: time.Unix(100, 0).UTC(),
			}, nil
		},
	}, nil)
	pool := domain.CredentialPool{
		ID:                "pool-1",
		FixedProviderType: domain.FixedProviderCodex,
		OAuthStrategy:     "round_robin",
	}

	live := service.Resolve(context.Background(), pool)
	cached := service.Resolve(context.Background(), pool)
	if live.Source != "live" || cached.Source != "cache" {
		t.Fatalf("sources = %q, %q", live.Source, cached.Source)
	}
	if live.ProfileRevision != "codex-cli@test" ||
		len(live.Models) != 2 ||
		live.Models[0].ID != "gpt-a" {
		t.Fatalf("live result = %#v", live)
	}
	if selections.Load() != 1 || inspections.Load() != 1 {
		t.Fatalf("calls = selections:%d inspections:%d", selections.Load(), inspections.Load())
	}
}

func TestServiceUsesStaleSnapshotAfterDiscoveryFailure(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	var fail atomic.Bool
	service := New(selectorFunc(func(context.Context, string, string) (*domain.OAuthCredential, error) {
		return &domain.OAuthCredential{ID: "credential-1", AccessToken: "token"}, nil
	}), inspectorStub{
		supported: true,
		inspect: func(_ context.Context, _ clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
			if fail.Load() {
				return clientruntime.InspectionSnapshot{}, errors.New("upstream unavailable")
			}
			return clientruntime.InspectionSnapshot{
				ProfileRevision: "codex-cli@test",
				Models:          []clientruntime.ModelCard{{ID: "gpt-live"}},
				ObservedAt:      now,
			}, nil
		},
	}, nil)
	service.now = func() time.Time { return now }
	service.cacheTTL = time.Minute
	pool := domain.CredentialPool{ID: "pool-1", FixedProviderType: domain.FixedProviderCodex}

	if result := service.Resolve(context.Background(), pool); result.Source != "live" {
		t.Fatalf("initial source = %q", result.Source)
	}
	now = now.Add(2 * time.Minute)
	fail.Store(true)
	result := service.Resolve(context.Background(), pool)
	if result.Source != "stale" || len(result.Models) != 1 || result.Models[0].ID != "gpt-live" {
		t.Fatalf("stale result = %#v", result)
	}
}

func TestServiceUsesVersionedFallbackWhenInspectionIsUnsupported(t *testing.T) {
	service := New(nil, inspectorStub{supported: false}, nil)
	result := service.Resolve(context.Background(), domain.CredentialPool{
		FixedProviderType: domain.FixedProviderGeminiCLI,
	})
	if result.Source != "fallback" || result.ProfileRevision != FallbackRevision {
		t.Fatalf("fallback result = %#v", result)
	}
	if len(result.Models) == 0 || result.Models[0].ID == "gemini-2.0-flash" {
		t.Fatalf("fallback models = %#v", result.Models)
	}
}

func TestServiceCallerCancellationDoesNotPoisonSharedDiscovery(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var inspections atomic.Int32
	service := New(selectorFunc(func(context.Context, string, string) (*domain.OAuthCredential, error) {
		return &domain.OAuthCredential{ID: "credential-1", AccessToken: "token"}, nil
	}), inspectorStub{
		supported: true,
		inspect: func(ctx context.Context, _ clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
			if inspections.Add(1) == 1 {
				close(started)
			}
			select {
			case <-release:
				return clientruntime.InspectionSnapshot{
					ProfileRevision: "codex-cli@test",
					Models:          []clientruntime.ModelCard{{ID: "gpt-live"}},
					ObservedAt:      time.Unix(100, 0).UTC(),
				}, nil
			case <-ctx.Done():
				return clientruntime.InspectionSnapshot{}, ctx.Err()
			}
		},
	}, nil)
	pool := domain.CredentialPool{ID: "pool-1", FixedProviderType: domain.FixedProviderCodex}

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan Result, 1)
	go func() {
		firstDone <- service.Resolve(firstCtx, pool)
	}()
	<-started
	cancelFirst()
	if result := <-firstDone; result.Source != "fallback" {
		t.Fatalf("canceled caller source = %q", result.Source)
	}

	secondDone := make(chan Result, 1)
	go func() {
		secondDone <- service.Resolve(context.Background(), pool)
	}()
	close(release)
	result := <-secondDone
	if (result.Source != "live" && result.Source != "cache") ||
		len(result.Models) != 1 || result.Models[0].ID != "gpt-live" {
		t.Fatalf("shared discovery result = %#v", result)
	}
	if inspections.Load() != 1 {
		t.Fatalf("inspection calls = %d, want 1", inspections.Load())
	}
}

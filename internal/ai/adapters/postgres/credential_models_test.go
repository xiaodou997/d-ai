package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

type accountModelInspector struct{}

func (accountModelInspector) SupportsInspection(domain.FixedProviderType, clientruntime.InspectionWant) bool {
	return true
}

func (accountModelInspector) Inspect(_ context.Context, in clientruntime.Inspection) (clientruntime.InspectionSnapshot, error) {
	return clientruntime.InspectionSnapshot{Models: []clientruntime.ModelCard{{ID: fmt.Sprintf("model-v%d", in.Credential.TokenVersion)}}}, nil
}

func TestOAuthModelDiscoveryUsesCurrentRevisionsWithoutChangingRotation(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("account model database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	poolID, err := store.CreatePool(ctx, domain.CredentialPoolCreate{Name: "model-accounts", FixedProviderType: domain.FixedProviderCodex, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.Create(ctx, poolID, domain.OAuthCredentialCreate{Name: "account", ProviderType: domain.FixedProviderCodex, AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}
	catalog := clientcatalog.New(store, accountModelInspector{}, nil)
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := catalog.Stop(stop); err != nil {
			t.Error(err)
		}
	})
	definition := domain.CredentialPool{ID: poolID, FixedProviderType: domain.FixedProviderCodex}
	first := catalog.Resolve(ctx, definition)
	if len(first.Accounts) != 1 || len(first.Models) != 1 || first.Models[0].ID != "model-v1" {
		t.Fatalf("initial account observation: %+v", first)
	}
	row, err := store.GetByID(ctx, id)
	if err != nil || row.LastUsedAt != nil {
		t.Fatalf("model inspection disturbed pool rotation: lastUsed=%v error=%v", row, err)
	}
	if err := store.UpdateCredential(ctx, id, domain.OAuthCredentialUpdate{AccessToken: "replacement-access"}); err != nil {
		t.Fatal(err)
	}
	second := catalog.Resolve(ctx, definition)
	if len(second.Models) != 1 || second.Models[0].ID != "model-v2" {
		t.Fatalf("edited credential reused previous permissions: %+v", second)
	}
	if err := store.UpdateStatus(ctx, id, "disabled"); err != nil {
		t.Fatal(err)
	}
	refs, err := store.ListModelCredentials(ctx, poolID)
	if err != nil || len(refs) != 0 {
		t.Fatalf("disabled account remained discoverable: %+v error=%v", refs, err)
	}
	if err := store.UpdateStatus(ctx, id, "active"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePoolStatus(ctx, poolID, "disabled"); err != nil {
		t.Fatal(err)
	}
	refs, err = store.ListModelCredentials(ctx, poolID)
	if err != nil || len(refs) != 0 {
		t.Fatalf("disabled pool remained discoverable: %+v error=%v", refs, err)
	}
}

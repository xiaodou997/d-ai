package postgres

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"sync"
	"testing"
	"time"
	"xiaodou/dai/internal/ai/routing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestOAuthImportIdentityAndAtomicAccountEditing(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("account database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	poolID, err := store.CreatePool(ctx, domain.CredentialPoolCreate{Name: "accounts", FixedProviderType: domain.FixedProviderCodex, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	input := domain.OAuthCredentialCreate{Name: "first", Email: "same@example.com", ProviderType: domain.FixedProviderCodex, AccessToken: "shared-access", RefreshToken: "shared-refresh", AuthMetadata: map[string]any{"account_id": "workspace-one", "user_id": "user"}}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := store.Create(ctx, poolID, input); outcomes <- err }()
	}
	wg.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else if errors.Is(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("imports success/conflict=%d/%d", successes, conflicts)
	}
	rows, err := store.ListForPool(ctx, poolID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	first := rows[0]
	input.AuthMetadata = map[string]any{"account_id": "workspace-two", "user_id": "user"}
	secondID, err := store.Create(ctx, poolID, input)
	if err != nil {
		t.Fatalf("different workspace rejected: %v", err)
	}
	name, email, weight, status := "renamed", "same@example.com", 0, "disabled"
	if err = store.UpdateCredential(ctx, first.ID, domain.OAuthCredentialUpdate{Name: &name, Email: &email, Weight: &weight, Status: &status, AccessToken: "access-v2"}); err != nil {
		t.Fatal(err)
	}
	currentRow, err := store.GetByID(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.decryptRow(ctx, *currentRow)
	if err != nil {
		t.Fatal(err)
	}
	if current.AccessToken != "access-v2" || current.RefreshToken != "shared-refresh" || current.TokenVersion != 2 || current.Weight != 0 || current.Status != domain.OAuthCredentialDisabled || current.Name != name {
		t.Fatal("account edit lost metadata, tokens, or disabled state")
	}
	if err = store.UpdateCredential(ctx, first.ID, domain.OAuthCredentialUpdate{AuthMetadata: map[string]any{"account_id": "workspace-two"}, Name: &name}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate identity edit=%v", err)
	}
	after, err := store.GetSummaryByID(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.AuthMetadata["account_id"] != "workspace-one" {
		t.Fatal("failed edit did not roll back")
	}
	// Scoped candidate selection must never escape availability's admitted set.
	status = "active"
	weight = 100
	if err = store.UpdateCredential(ctx, first.ID, domain.OAuthCredentialUpdate{Status: &status, Weight: &weight}); err != nil {
		t.Fatal(err)
	}
	eligible := []string{first.ID, secondID}
	firstPick, err := store.SelectEligibleCredential(ctx, poolID, "round_robin", eligible)
	if err != nil {
		t.Fatal(err)
	}
	secondPick, err := store.SelectEligibleCredential(ctx, poolID, "round_robin", eligible)
	if err != nil || firstPick == secondPick {
		t.Fatalf("round robin did not rotate: %v", err)
	}
	weight = 0
	if err = store.UpdateCredential(ctx, first.ID, domain.OAuthCredentialUpdate{Weight: &weight}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		picked, err := store.SelectEligibleCredential(ctx, poolID, "weighted", eligible)
		if err != nil || picked != secondID {
			t.Fatalf("weighted zero selected=%s err=%v", picked, err)
		}
	}
	if _, err = store.SelectEligibleCredential(ctx, poolID, "weighted", []string{first.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("selector escaped eligible set: %v", err)
	}
}

func TestOAuthQuotaObservationsStayWithTheirAccountIdentity(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("quota database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef").WithAvailability(routing.NewRedisAvailability(client))
	poolID, err := store.CreatePool(ctx, domain.CredentialPoolCreate{Name: "quota-pool", FixedProviderType: domain.FixedProviderCodex, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.Create(ctx, poolID, domain.OAuthCredentialCreate{Name: "account", ProviderType: domain.FixedProviderCodex, AccessToken: "access", AuthMetadata: map[string]any{"account_id": "one"}})
	if err != nil {
		t.Fatal(err)
	}
	used := 100.0
	quota := domain.CredentialQuota{Source: "response_headers", ObservedAt: time.Now().UnixMilli(), Windows: []domain.CredentialQuotaWindow{{Key: "primary", UsedPercent: &used, ResetAt: time.Now().Add(time.Hour).UnixMilli()}}}
	if err = store.ObserveCredentialQuota(ctx, id, 1, quota); err != nil {
		t.Fatal(err)
	}
	summary, err := store.GetSummaryByID(ctx, id)
	if err != nil || len(summary.Quotas) != 1 {
		t.Fatalf("quota not returned: %v", err)
	}
	if err = store.UpdateCredential(ctx, id, domain.OAuthCredentialUpdate{AuthMetadata: map[string]any{"account_id": "two"}}); err != nil {
		t.Fatal(err)
	}
	if err = store.ObserveCredentialQuota(ctx, id, 1, quota); err != nil {
		t.Fatal(err)
	}
	summary, err = store.GetSummaryByID(ctx, id)
	if err != nil || len(summary.Quotas) != 0 {
		t.Fatal("old identity quota leaked into the replacement account")
	}
}

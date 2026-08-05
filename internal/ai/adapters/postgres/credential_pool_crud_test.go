package postgres

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/testsupport"
)

// TestCredentialPoolRoundTrip covers the hand-written pool SQL, whose column
// list and positional parameters are aligned by hand rather than by sqlc. A
// mismatch there compiles cleanly and only fails when an administrator saves a
// pool, so the create/read/update path is exercised end to end here.
func TestCredentialPoolRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("credential pool test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	multiplier := 1.5

	id, err := store.CreatePool(ctx, CredentialPoolInput{
		Name:              "codex-pool",
		TenantDisplayName: "Codex Pool",
		TenantAccessMode:  "public",
		FixedProviderType: "codex",
		Notes:             "created by test",
		TenantMultiplier:  &multiplier,
	})
	if err != nil {
		t.Fatalf("CreatePool() error = %v", err)
	}

	created, err := store.GetPool(ctx, id)
	if err != nil {
		t.Fatalf("GetPool() error = %v", err)
	}
	if created.Name != "codex-pool" || created.TenantDisplayName != "Codex Pool" {
		t.Errorf("names = %q/%q, want codex-pool/Codex Pool", created.Name, created.TenantDisplayName)
	}
	if created.TenantMultiplier != 1.5 {
		t.Errorf("TenantMultiplier = %v, want 1.5", created.TenantMultiplier)
	}
	if created.Status != "disabled" {
		t.Errorf("Status = %q, want disabled on create", created.Status)
	}

	updatedMultiplier := 2.0
	if err := store.UpdatePool(ctx, id, CredentialPoolInput{
		Name:              "codex-pool-renamed",
		TenantDisplayName: "Codex Pool 2",
		TenantAccessMode:  "restricted",
		OAuthStrategy:     "weighted",
		Notes:             "updated by test",
		Status:            "active",
		TenantMultiplier:  &updatedMultiplier,
	}); err != nil {
		t.Fatalf("UpdatePool() error = %v", err)
	}

	updated, err := store.GetPool(ctx, id)
	if err != nil {
		t.Fatalf("GetPool() after update error = %v", err)
	}
	if updated.Name != "codex-pool-renamed" || updated.OAuthStrategy != "weighted" {
		t.Errorf("update mismatch: %+v", updated)
	}
	if updated.TenantMultiplier != 2.0 {
		t.Errorf("TenantMultiplier = %v, want 2.0", updated.TenantMultiplier)
	}
	if updated.TenantAccessMode != "restricted" || updated.Status != "active" {
		t.Errorf("mode/status = %q/%q, want restricted/active", updated.TenantAccessMode, updated.Status)
	}
	pools, err := store.ListPools(ctx)
	if err != nil {
		t.Fatalf("ListPools() error = %v", err)
	}
	var found bool
	for _, p := range pools {
		if p.ID == id {
			found = true
		}
	}
	if !found {
		t.Error("ListPools() did not return the created pool")
	}
}

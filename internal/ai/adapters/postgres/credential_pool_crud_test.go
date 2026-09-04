package postgres

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/domain"
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

	id, err := store.CreatePool(ctx, domain.CredentialPoolCreate{
		Name:              "codex-pool",
		TenantDisplayName: "Codex Pool",
		TenantAccessMode:  "public",
		FixedProviderType: domain.FixedProviderCodex,
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
	if err := store.UpdatePool(ctx, id, domain.CredentialPoolUpdate{
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

func TestCredentialPoolActivationRejectsUnsupportedActiveBindings(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("credential pool test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	id, err := store.CreatePool(ctx, domain.CredentialPoolCreate{
		Name: "invalid-codex-pool", TenantDisplayName: "Invalid Codex Pool",
		TenantAccessMode: "public", FixedProviderType: domain.FixedProviderCodex,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_models (upstream_kind, upstream_id, model_code, capability_type, upstream_model_name, status)
		VALUES ('oauth_pool', $1::uuid, 'image-model', 'image', 'image-model', 'active')
	`, id); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePoolStatus(ctx, id, "active"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("UpdatePoolStatus() error = %v, want validation", err)
	}
	poolAfter, err := store.GetPool(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if poolAfter.Status != "disabled" {
		t.Fatalf("pool status after rejected activation = %q, want disabled", poolAfter.Status)
	}
}

func TestDeleteCredentialPoolRemovesModelBindings(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("credential pool test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	id, err := store.CreatePool(ctx, domain.CredentialPoolCreate{
		Name: "delete-codex-pool", TenantDisplayName: "Delete Codex Pool",
		TenantAccessMode: "public", FixedProviderType: domain.FixedProviderCodex,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_models (upstream_kind, upstream_id, model_code, capability_type, upstream_model_name, status)
		VALUES ('oauth_pool', $1::uuid, 'chat-model', 'chat', 'chat-model', 'active')
	`, id); err != nil {
		t.Fatal(err)
	}

	if err := store.DeletePool(ctx, id); err != nil {
		t.Fatalf("DeletePool() error = %v", err)
	}
	var modelCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM ai_upstream_models
		WHERE upstream_kind = 'oauth_pool' AND upstream_id = $1::uuid
	`, id).Scan(&modelCount); err != nil {
		t.Fatal(err)
	}
	if modelCount != 0 {
		t.Fatalf("remaining model bindings = %d, want 0", modelCount)
	}
	if _, err := store.GetPool(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetPool() after delete error = %v, want not found", err)
	}
}

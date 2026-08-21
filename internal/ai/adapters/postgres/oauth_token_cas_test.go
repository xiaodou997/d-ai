package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestOAuthCredentialTokenCASRejectsStaleRefresh(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("credential token CAS test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	poolID, err := store.CreatePool(ctx, domain.CredentialPoolCreate{
		Name:              "token-cas-pool",
		TenantDisplayName: "Token CAS Pool",
		TenantAccessMode:  "public",
		FixedProviderType: domain.FixedProviderCodex,
		Status:            "active",
	})
	if err != nil {
		t.Fatalf("CreatePool() error = %v", err)
	}
	credentialID, err := store.Create(ctx, poolID, domain.OAuthCredentialCreate{
		Name:         "token-cas-credential",
		ProviderType: domain.FixedProviderCodex,
		AccessToken:  "access-v1",
		RefreshToken: "refresh-v1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	credential, err := store.GetDecryptedByID(ctx, credentialID)
	if err != nil {
		t.Fatalf("GetDecryptedByID() error = %v", err)
	}
	if credential.TokenVersion != 1 {
		t.Fatalf("initial token version = %d, want 1", credential.TokenVersion)
	}

	expiresAt := time.Now().Add(time.Hour)
	nextVersion, err := store.UpdateTokens(
		ctx,
		credentialID,
		"access-v2",
		"refresh-v2",
		&expiresAt,
		credential.TokenVersion,
	)
	if err != nil {
		t.Fatalf("UpdateTokens() error = %v", err)
	}
	if nextVersion != 2 {
		t.Fatalf("next token version = %d, want 2", nextVersion)
	}

	_, err = store.UpdateTokens(
		ctx,
		credentialID,
		"stale-access",
		"stale-refresh",
		&expiresAt,
		credential.TokenVersion,
	)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale UpdateTokens() error = %v, want conflict", err)
	}

	current, err := store.GetDecryptedByID(ctx, credentialID)
	if err != nil {
		t.Fatalf("reload credential error = %v", err)
	}
	if current.TokenVersion != 2 || current.AccessToken != "access-v2" || current.RefreshToken != "refresh-v2" {
		t.Fatalf("current credential = %#v", current)
	}
}

func TestOAuthCredentialCooldownExcludesAndThenRestoresSelection(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("credential cooldown test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	poolID, err := store.CreatePool(ctx, domain.CredentialPoolCreate{
		Name:              "cooldown-pool",
		TenantDisplayName: "Cooldown Pool",
		TenantAccessMode:  "public",
		FixedProviderType: domain.FixedProviderCodex,
		Status:            "active",
	})
	if err != nil {
		t.Fatalf("CreatePool() error = %v", err)
	}
	credentialID, err := store.Create(ctx, poolID, domain.OAuthCredentialCreate{
		Name:         "cooldown-credential",
		ProviderType: domain.FixedProviderCodex,
		AccessToken:  "access-v1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	until := time.Now().Add(10 * time.Minute).UTC()
	if err := store.MarkCooldown(ctx, credentialID, until); err != nil {
		t.Fatalf("MarkCooldown() error = %v", err)
	}
	row, err := store.GetByID(ctx, credentialID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if row.CooldownUntil == nil || row.CooldownUntil.Before(until.Add(-time.Second)) {
		t.Fatalf("cooldown_until = %v, want at least %v", row.CooldownUntil, until)
	}
	if _, err := store.SelectCredentialFromPool(ctx, poolID, "round_robin"); err == nil {
		t.Fatal("SelectCredentialFromPool() error = nil during cooldown")
	}

	store.RecordSuccess(ctx, credentialID)
	selected, err := store.SelectCredentialFromPool(ctx, poolID, "round_robin")
	if err != nil {
		t.Fatalf("SelectCredentialFromPool() after success error = %v", err)
	}
	if selected.ID != credentialID || selected.CooldownUntil != nil {
		t.Fatalf("selected credential = %#v", selected)
	}
}

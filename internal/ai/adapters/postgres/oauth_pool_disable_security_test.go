package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestDisabledOAuthPoolCannotSupplyOrRefreshCredentials(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("oauth credential test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	poolID, err := store.CreatePool(ctx, domain.CredentialPoolCreate{
		Name: "disable-boundary-pool", TenantDisplayName: "Disable Boundary Pool",
		TenantAccessMode: "public", FixedProviderType: domain.FixedProviderCodex,
		OAuthStrategy: "round_robin", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	credentialID, err := store.Create(ctx, poolID, domain.OAuthCredentialCreate{
		Name: "disable-boundary-credential", ProviderType: domain.FixedProviderCodex,
		AccessToken: "access-before-disable", RefreshToken: "refresh-before-disable",
		ExpiresAt: &expiresAt, Weight: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.SelectCredentialFromPool(ctx, poolID, "round_robin"); err != nil {
		t.Fatalf("active pool round-robin selection: %v", err)
	}
	if err := store.UpdatePoolStatus(ctx, poolID, "disabled"); err != nil {
		t.Fatalf("disable pool: %v", err)
	}

	for _, tc := range []struct {
		name string
		selectCredential func() error
	}{
		{name: "round robin", selectCredential: func() error {
			_, err := store.SelectCredentialFromPool(ctx, poolID, "round_robin")
			return err
		}},
		{name: "weighted", selectCredential: func() error {
			_, err := store.SelectCredentialFromPool(ctx, poolID, "weighted")
			return err
		}},
		{name: "pinned", selectCredential: func() error {
			_, err := store.SelectPinnedCredential(ctx, poolID, credentialID)
			return err
		}},
		{name: "excluding", selectCredential: func() error {
			_, err := store.SelectCredentialExcluding(ctx, poolID, "round_robin", nil)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.selectCredential(); err == nil {
				t.Fatal("disabled OAuth pool still supplied a credential")
			}
		})
	}

	if _, err := store.GetDecryptedByID(ctx, credentialID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetDecryptedByID() after pool disable = %v, want not found", err)
	}

	expiring, err := store.ListExpiring(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range expiring {
		if row.ID == credentialID {
			t.Fatal("disabled pool credential remained eligible for background refresh")
		}
	}

	row, err := store.GetByID(ctx, credentialID)
	if err != nil {
		t.Fatal(err)
	}
	newExpiry := time.Now().Add(time.Hour)
	if _, err := store.UpdateTokens(ctx, credentialID, "access-after-disable", "refresh-after-disable", &newExpiry, row.TokenVersion); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("UpdateTokens() after pool disable = %v, want conflict", err)
	}
	current, err := store.GetByID(ctx, credentialID)
	if err != nil {
		t.Fatal(err)
	}
	if current.TokenVersion != row.TokenVersion {
		t.Fatalf("disabled pool credential token version changed from %d to %d", row.TokenVersion, current.TokenVersion)
	}
}

func TestDisabledOAuthCredentialCannotBeDecryptedForRefresh(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("oauth credential test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	poolID, err := store.CreatePool(ctx, domain.CredentialPoolCreate{
		Name: "disabled-credential-pool", TenantDisplayName: "Disabled Credential Pool",
		TenantAccessMode: "public", FixedProviderType: domain.FixedProviderCodex,
		Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	credentialID, err := store.Create(ctx, poolID, domain.OAuthCredentialCreate{
		Name: "disabled-credential", ProviderType: domain.FixedProviderCodex,
		AccessToken: "access-token", RefreshToken: "refresh-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateStatus(ctx, credentialID, "disabled"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDecryptedByID(ctx, credentialID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetDecryptedByID() for disabled credential = %v, want not found", err)
	}
}

package postgres

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

// A credential imported without a refresh token stores NULL in
// refresh_token_ciphertext (Create writes an invalid pgtype.Text), and the same
// applies to the nullable email/scope columns. Every read path must tolerate
// that: the row struct models "absent" as the empty string, so a NULL must not
// abort the scan. Regression test for reads failing on such credentials.
func TestOAuthCredentialReadsTolerateNullTextColumns(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("oauth credential test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	store := NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	poolID, err := store.CreatePool(ctx, CredentialPoolInput{
		Name:              "null-text-pool",
		TenantDisplayName: "Null Text Pool",
		TenantAccessMode:  "public",
		FixedProviderType: string(domain.FixedProviderCodex),
		Status:            "active",
	})
	if err != nil {
		t.Fatalf("CreatePool() error = %v", err)
	}

	// No refresh token, no email, no scope.
	credentialID, err := store.Create(ctx, poolID, domain.OAuthCredentialCreate{
		Name:         "no-refresh-token",
		ProviderType: domain.FixedProviderCodex,
		AccessToken:  "access-only",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	row, err := store.GetByID(ctx, credentialID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if row.RefreshTokenCiphertext != "" || row.Email != "" || row.Scope != "" {
		t.Fatalf("nullable columns = %q/%q/%q, want empty strings",
			row.RefreshTokenCiphertext, row.Email, row.Scope)
	}

	if _, err := store.ListForPool(ctx, poolID); err != nil {
		t.Fatalf("ListForPool() error = %v", err)
	}

	// Round-robin selection scans the same columns via a different query.
	selected, err := store.SelectCredentialFromPool(ctx, poolID, "round_robin")
	if err != nil {
		t.Fatalf("SelectCredentialFromPool() error = %v", err)
	}
	if selected.RefreshToken != "" {
		t.Fatalf("refresh token = %q, want empty", selected.RefreshToken)
	}
	if selected.AccessToken != "access-only" {
		t.Fatalf("access token = %q, want %q", selected.AccessToken, "access-only")
	}
}

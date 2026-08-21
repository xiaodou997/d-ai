package postgres

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestOAuthCredentialSummaryAllowsOnlyKnownIdentityMetadata(t *testing.T) {
	row := OAuthCredentialRow{
		ID:                    "cred-1",
		PoolID:                "pool-1",
		AccessTokenCiphertext: "ciphertext-must-stay-private",
		AuthMetadataRaw:       []byte(`{"account_id":"account-1","plan_type":"team","refresh_token":"must-not-leak","private_key":"must-not-leak","nested":{"api_key":"must-not-leak"}}`),
	}

	got := oauthCredentialSummary(row)
	if got.ID != row.ID || got.PoolID != row.PoolID {
		t.Fatalf("summary identity = %#v, want %q/%q", got, row.ID, row.PoolID)
	}
	if got.AuthMetadata["account_id"] != "account-1" {
		t.Fatalf("account metadata was lost: %#v", got.AuthMetadata)
	}
	if _, ok := got.AuthMetadata["refresh_token"]; ok {
		t.Fatal("refresh token metadata leaked into summary")
	}
	if _, ok := got.AuthMetadata["private_key"]; ok {
		t.Fatal("private key metadata leaked into summary")
	}
	if _, ok := got.AuthMetadata["nested"]; ok {
		t.Fatal("opaque nested metadata leaked into summary")
	}
	if got.AuthMetadata["plan_type"] != "team" {
		t.Fatalf("plan metadata was lost: %#v", got.AuthMetadata)
	}

	var _ domain.OAuthCredentialSummary = got
}

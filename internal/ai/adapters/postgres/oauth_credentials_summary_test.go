package postgres

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestOAuthCredentialSummaryRedactsSensitiveMetadata(t *testing.T) {
	row := OAuthCredentialRow{
		ID:                    "cred-1",
		PoolID:                "pool-1",
		AccessTokenCiphertext: "ciphertext-must-stay-private",
		AuthMetadataRaw:       []byte(`{"account_id":"account-1","refresh_token":"must-not-leak","nested":{"api_key":"must-not-leak"}}`),
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
	nested, ok := got.AuthMetadata["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested metadata type = %T", got.AuthMetadata["nested"])
	}
	if _, ok := nested["api_key"]; ok {
		t.Fatal("nested API key metadata leaked into summary")
	}

	var _ domain.OAuthCredentialSummary = got
}

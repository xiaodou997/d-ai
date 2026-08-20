package transport

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestPoolCredentialSummaryToDTORedactsMetadata(t *testing.T) {
	dto := poolCredentialSummaryToDTO(domain.OAuthCredentialSummary{
		ID:     "cred-1",
		Status: "active",
		AuthMetadata: map[string]any{
			"account_id":   "account-1",
			"access_token": "must-not-leak",
			"nested": map[string]any{
				"client_secret": "must-not-leak",
			},
		},
	})

	if dto.AuthMetadata["account_id"] != "account-1" {
		t.Fatalf("account metadata was lost: %#v", dto.AuthMetadata)
	}
	if _, ok := dto.AuthMetadata["access_token"]; ok {
		t.Fatal("access token metadata leaked into DTO")
	}
	nested, ok := dto.AuthMetadata["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested metadata type = %T, want map[string]any", dto.AuthMetadata["nested"])
	}
	if _, ok := nested["client_secret"]; ok {
		t.Fatal("nested client secret metadata leaked into DTO")
	}
}

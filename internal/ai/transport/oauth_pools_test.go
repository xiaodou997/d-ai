package transport

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/ai/domain"
)

type credentialReaderStub struct {
	summary *domain.OAuthCredentialSummary
}

func (s credentialReaderStub) ListForPool(context.Context, string) ([]domain.OAuthCredentialSummary, error) {
	if s.summary == nil {
		return nil, nil
	}
	return []domain.OAuthCredentialSummary{*s.summary}, nil
}

func (s credentialReaderStub) GetSummaryByID(context.Context, string) (*domain.OAuthCredentialSummary, error) {
	return s.summary, nil
}

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

func TestGetPoolCredentialScopedUsesSummaryReader(t *testing.T) {
	reader := credentialReaderStub{summary: &domain.OAuthCredentialSummary{ID: "cred-1", PoolID: "pool-1"}}

	got, err := getPoolCredentialScoped(context.Background(), reader, "pool-1", "cred-1")
	if err != nil {
		t.Fatalf("getPoolCredentialScoped() error = %v", err)
	}
	if got.ID != "cred-1" {
		t.Fatalf("credential ID = %q, want %q", got.ID, "cred-1")
	}
}

func TestGetPoolCredentialScopedRejectsWrongPool(t *testing.T) {
	reader := credentialReaderStub{summary: &domain.OAuthCredentialSummary{ID: "cred-1", PoolID: "pool-2"}}

	if _, err := getPoolCredentialScoped(context.Background(), reader, "pool-1", "cred-1"); err != pgx.ErrNoRows {
		t.Fatalf("getPoolCredentialScoped() error = %v, want pgx.ErrNoRows", err)
	}
}

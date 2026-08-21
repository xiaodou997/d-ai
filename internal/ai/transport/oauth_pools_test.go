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

type credentialCreatorStub struct {
	poolID string
	input  domain.OAuthCredentialCreate
	id     string
}

func (s *credentialCreatorStub) Create(_ context.Context, poolID string, input domain.OAuthCredentialCreate) (string, error) {
	s.poolID = poolID
	s.input = input
	return s.id, nil
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

func TestImportPoolCredentialUsesDedicatedCreatorPort(t *testing.T) {
	creator := &credentialCreatorStub{id: "cred-1"}
	reader := credentialReaderStub{summary: &domain.OAuthCredentialSummary{ID: "cred-1", PoolID: "pool-1"}}
	poolReader := &poolReaderStub{pool: &domain.CredentialPool{
		ID:                "pool-1",
		FixedProviderType: domain.FixedProviderCodex,
	}}

	got, err := importPoolCredential(context.Background(), AIDeps{
		IdentityDeps: IdentityDeps{
			CredentialCreator: creator,
			CredentialReader:  reader,
			PoolReader:        poolReader,
		},
	}, "pool-1", poolCredentialWriteRequest{
		Name:        " Imported account ",
		AccessToken: " access-token ",
	})
	if err != nil {
		t.Fatalf("importPoolCredential() error = %v", err)
	}
	if got != reader.summary {
		t.Fatalf("importPoolCredential() summary = %#v, want %#v", got, reader.summary)
	}
	if creator.poolID != "pool-1" || poolReader.poolID != "pool-1" {
		t.Fatalf("port pool IDs = creator %q, reader %q", creator.poolID, poolReader.poolID)
	}
	if creator.input.ProviderType != domain.FixedProviderCodex || creator.input.AccessToken != "access-token" {
		t.Fatalf("creator input = %#v", creator.input)
	}
}

func TestPoolCredentialSummaryToDTOAllowsOnlyKnownIdentityMetadata(t *testing.T) {
	dto := poolCredentialSummaryToDTO(domain.OAuthCredentialSummary{
		ID:     "cred-1",
		Status: "active",
		AuthMetadata: map[string]any{
			"account_id":    "account-1",
			"plan_type":     "team",
			"access_token":  "must-not-leak",
			"private_key":   "must-not-leak",
			"authorization": "must-not-leak",
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
	for _, key := range []string{"private_key", "authorization", "nested"} {
		if _, ok := dto.AuthMetadata[key]; ok {
			t.Fatalf("metadata key %q leaked into DTO", key)
		}
	}
	if dto.AuthMetadata["plan_type"] != "team" {
		t.Fatalf("plan metadata was lost: %#v", dto.AuthMetadata)
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

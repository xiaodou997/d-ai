package postgres

import (
	"reflect"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/ai/upstreamcontrol"
)

func TestAccountRepoConnectionUpdateIsAtomic(t *testing.T) {
	ctx := t.Context()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open connection update test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	repo := NewAccountRepo(NewQueries(pool), pool)
	svc := upstreamcontrol.New(repo, func(plain string) (string, error) { return "protected:" + plain, nil })
	account, err := svc.CreateAccount(ctx, upstreamcontrol.CreateAccountInput{
		Name: "Original", APIKey: "original-key", Status: domain.UpstreamAccountStatusActive,
		Endpoints: []domain.UpstreamAccountEndpointWrite{
			{APIFormat: domain.ProtocolOpenAIResponses, BaseURL: "https://responses.example"},
			{APIFormat: domain.ProtocolOpenAIChat, BaseURL: "https://chat.example"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first := account.Endpoints[0]
	write := domain.UpstreamAccountEndpointWrite{
		APIFormat: first.APIFormat, BaseURL: "https://new.example", AuthScheme: domain.EndpointAuthFormatDefault, Status: domain.EndpointStatusActive,
	}
	for _, tc := range []struct {
		name      string
		endpoints []upstreamcontrol.AccountEndpointUpdate
	}{
		{"foreign ID", []upstreamcontrol.AccountEndpointUpdate{{ID: "f0000000-0000-0000-0000-000000000001", UpstreamAccountEndpointWrite: write}}},
		{"format conflict", []upstreamcontrol.AccountEndpointUpdate{{ID: first.ID, UpstreamAccountEndpointWrite: write}, {UpstreamAccountEndpointWrite: write}}},
		{"no active endpoint", []upstreamcontrol.AccountEndpointUpdate{{ID: first.ID, UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{
			APIFormat: first.APIFormat, BaseURL: "https://new.example", AuthScheme: domain.EndpointAuthFormatDefault, Status: domain.EndpointStatusDisabled,
		}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.UpdateAccount(ctx, upstreamcontrol.AccountUpdate{
				ID: account.ID, Name: "Must roll back", TenantDisplayName: "Must roll back", TenantAccessMode: "public",
				Ciphertext: "must-not-be-written", Status: domain.UpstreamAccountStatusActive, Endpoints: tc.endpoints,
			})
			if err == nil {
				t.Fatal("invalid combined update succeeded")
			}
			got, err := repo.GetAccount(ctx, account.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, account) {
				t.Fatal("failed update changed account or endpoints")
			}
			secret, err := repo.GetAccountSecret(ctx, account.ID)
			if err != nil {
				t.Fatal(err)
			}
			if secret.Ciphertext != "protected:original-key" {
				t.Fatal("failed endpoint write changed the credential")
			}
		})
	}

	// Edit one existing endpoint, add a format, and remove the omitted endpoint
	// together with a credential rotation.
	updated, err := svc.UpdateAccount(ctx, upstreamcontrol.UpdateAccountInput{
		ID: account.ID, Name: "Updated", APIKey: "new-key",
		Endpoints: []upstreamcontrol.AccountEndpointUpdate{
			{ID: first.ID, UpstreamAccountEndpointWrite: write},
			{UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{APIFormat: domain.ProtocolAnthropicMessages, BaseURL: "https://claude.example"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Updated" || len(updated.Endpoints) != 2 {
		t.Fatal("combined update missing account or endpoints")
	}
	retained, err := repo.GetEndpoint(ctx, account.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retained.BaseURL != write.BaseURL || !retained.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("URL edit replaced the existing endpoint identity")
	}
	secret, err := repo.GetAccountSecret(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secret.Ciphertext != "protected:new-key" {
		t.Fatal("credential rotation was not saved")
	}

	// Saving unchanged connections must not clear the last connectivity check.
	if _, err := repo.UpdateEndpointHealth(ctx, account.ID, first.ID, domain.HealthHealthy, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateAccount(ctx, upstreamcontrol.UpdateAccountInput{
		ID: account.ID, Name: "Metadata only",
		Endpoints: []upstreamcontrol.AccountEndpointUpdate{
			{ID: first.ID, UpstreamAccountEndpointWrite: write},
			{ID: updated.Endpoints[0].ID, UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{APIFormat: domain.ProtocolAnthropicMessages, BaseURL: "https://claude.example"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	retained, err = repo.GetEndpoint(ctx, account.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retained.HealthStatus != domain.HealthHealthy {
		t.Fatal("unchanged connection lost its health check")
	}
}

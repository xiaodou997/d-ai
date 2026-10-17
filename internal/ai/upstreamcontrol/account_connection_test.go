package upstreamcontrol

import (
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestUpdateAccountSavesKeyAndEndpointsTogether(t *testing.T) {
	repo := &repoStub{
		secret: AccountSecret{Ciphertext: "old-key", Status: domain.UpstreamAccountStatusActive},
		endpoints: []domain.UpstreamAccountEndpoint{{
			ID: "endpoint-1", AccountID: "account-1", APIFormat: domain.ProtocolOpenAIResponses,
			ExtraHeaders: []byte(`{"Authorization":"encrypted:header","X-Trace":"old"}`),
		}},
	}
	svc := New(repo, func(plain string) (string, error) { return "encrypted:" + plain, nil })
	_, err := svc.UpdateAccount(t.Context(), UpdateAccountInput{
		ID: "account-1", Name: "Updated", APIKey: "new-key",
		Endpoints: []AccountEndpointUpdate{
			{ID: "endpoint-1", UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{
				APIFormat: domain.ProtocolOpenAIResponses, BaseURL: " https://new.example/v1/ ",
				ExtraHeaders: []byte(`{"Authorization":"***REDACTED***","X-Trace":"new"}`),
			}},
			{UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{
				APIFormat: domain.ProtocolAnthropicMessages, BaseURL: "https://claude.example",
				ExtraHeaders: []byte(`{"X-API-Key":"new-header"}`),
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := repo.lastUpdate
	if got.Ciphertext != "encrypted:new-key" || len(got.Endpoints) != 2 || got.Status != domain.UpstreamAccountStatusActive {
		t.Fatalf("combined update did not preserve key, endpoints and managed status")
	}
	if got.Endpoints[0].ID != "endpoint-1" || got.Endpoints[0].BaseURL != "https://new.example/v1" {
		t.Fatalf("endpoint identity or normalized URL changed: %+v", got.Endpoints[0])
	}
	var headers map[string]string
	if err := json.Unmarshal(got.Endpoints[0].ExtraHeaders, &headers); err != nil {
		t.Fatal(err)
	}
	if headers["Authorization"] != "encrypted:header" || headers["X-Trace"] != "new" {
		t.Fatal("masked header was not preserved")
	}
	if err := json.Unmarshal(got.Endpoints[1].ExtraHeaders, &headers); err != nil {
		t.Fatal(err)
	}
	if headers["X-API-Key"] != "encrypted:new-header" {
		t.Fatal("new sensitive header was not encrypted")
	}
}

func TestUpdateAccountValidatesEndpointSetBeforeWriting(t *testing.T) {
	write := domain.UpstreamAccountEndpointWrite{APIFormat: domain.ProtocolOpenAIResponses, BaseURL: "https://api.example"}
	for _, tc := range []struct {
		name  string
		items []AccountEndpointUpdate
	}{
		{"empty", []AccountEndpointUpdate{}},
		{"duplicate format", []AccountEndpointUpdate{{UpstreamAccountEndpointWrite: write}, {UpstreamAccountEndpointWrite: write}}},
		{"foreign endpoint", []AccountEndpointUpdate{{ID: "foreign", UpstreamAccountEndpointWrite: write}}},
		{"duplicate ID", []AccountEndpointUpdate{{ID: "endpoint-1", UpstreamAccountEndpointWrite: write}, {ID: "endpoint-1", UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{APIFormat: domain.ProtocolOpenAIChat, BaseURL: "https://api.example"}}}},
		{"invalid URL", []AccountEndpointUpdate{{UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{APIFormat: domain.ProtocolOpenAIResponses, BaseURL: "file:///tmp/socket"}}}},
		{"all disabled", []AccountEndpointUpdate{{UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{APIFormat: domain.ProtocolOpenAIResponses, BaseURL: "https://api.example", Status: domain.EndpointStatusDisabled}}}},
		{"unbound masked header", []AccountEndpointUpdate{{UpstreamAccountEndpointWrite: domain.UpstreamAccountEndpointWrite{APIFormat: domain.ProtocolOpenAIResponses, BaseURL: "https://api.example", ExtraHeaders: []byte(`{"Authorization":"***REDACTED***"}`)}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &repoStub{secret: AccountSecret{Ciphertext: "old", Status: domain.UpstreamAccountStatusActive}, endpoints: []domain.UpstreamAccountEndpoint{{ID: "endpoint-1"}}}
			svc := New(repo, func(plain string) (string, error) { return "encrypted:" + plain, nil })
			if _, err := svc.UpdateAccount(t.Context(), UpdateAccountInput{ID: "account-1", Name: "Updated", APIKey: "new", Endpoints: tc.items}); err == nil {
				t.Fatal("invalid endpoint set accepted")
			}
			if repo.lastUpdate.ID != "" {
				t.Fatal("account or API key written before endpoint validation")
			}
		})
	}
}

func TestUpdateAccountOmittedEndpointsPreservesConfiguration(t *testing.T) {
	repo := &repoStub{secret: AccountSecret{Ciphertext: "old", Status: domain.UpstreamAccountStatusDisabled}}
	svc := New(repo, nil)
	if _, err := svc.UpdateAccount(t.Context(), UpdateAccountInput{ID: "account-1", Name: "Metadata only"}); err != nil {
		t.Fatal(err)
	}
	if repo.lastUpdate.Endpoints != nil || repo.lastUpdate.Ciphertext != "old" {
		t.Fatal("omitted connection fields must remain unchanged")
	}
}

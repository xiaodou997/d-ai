package upstreamcontrol

import (
	"context"
	"reflect"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type repoStub struct {
	account           domain.UpstreamAccount
	secret            AccountSecret
	lastCreate        AccountCreate
	lastUpdate        AccountUpdate
	priceBookExists   bool
	updateAccountResp domain.UpstreamAccount
	statusAccountID   string
	statusValue       string
	invalidAccountID  string
	invalidReason     string
	deleteAccountID   string
	deleteErr         error
	endpoints         []domain.UpstreamAccountEndpoint
	lastEndpointWrite domain.UpstreamAccountEndpointWrite
}

func TestGetAccountSecretDelegatesDomainFields(t *testing.T) {
	want := AccountSecret{
		Ciphertext: "cipher",
		Endpoints: []domain.UpstreamAccountEndpoint{{
			ID: "endpoint-1", APIFormat: domain.ProtocolOpenAIResponses, BaseURL: "https://upstream.example",
		}},
		Status: domain.UpstreamAccountStatusActive,
	}
	svc := New(&repoStub{secret: want}, nil)

	got, err := svc.GetAccountSecret(t.Context(), "account-1")
	if err != nil {
		t.Fatalf("GetAccountSecret() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAccountSecret() = %#v, want %#v", got, want)
	}
}

func (s *repoStub) CreateAccount(_ context.Context, in AccountCreate) (domain.UpstreamAccount, error) {
	s.lastCreate = in
	return domain.UpstreamAccount{}, nil
}

func (s *repoStub) ListAccounts(context.Context) ([]domain.UpstreamAccount, error) {
	return nil, nil
}

func (s *repoStub) GetAccountSecret(context.Context, string) (AccountSecret, error) {
	return s.secret, nil
}

func (s *repoStub) UpdateAccount(_ context.Context, e AccountUpdate) (domain.UpstreamAccount, error) {
	s.lastUpdate = e
	if s.updateAccountResp.ID != "" {
		return s.updateAccountResp, nil
	}
	return s.account, nil
}

func (s *repoStub) UpdateAccountStatus(_ context.Context, id, status string) (domain.UpstreamAccount, error) {
	s.statusAccountID = id
	s.statusValue = status
	return domain.UpstreamAccount{}, nil
}

func (s *repoStub) MarkAccountInvalid(_ context.Context, id, reason string) (domain.UpstreamAccount, error) {
	s.invalidAccountID = id
	s.invalidReason = reason
	return domain.UpstreamAccount{}, nil
}

func (s *repoStub) PriceBookExists(context.Context, string) (bool, error) {
	return s.priceBookExists, nil
}

func (s *repoStub) DeleteAccount(_ context.Context, id string) error {
	s.deleteAccountID = id
	return s.deleteErr
}

func (s *repoStub) ListEndpoints(context.Context, string) ([]domain.UpstreamAccountEndpoint, error) {
	return s.endpoints, nil
}

func (s *repoStub) GetEndpoint(context.Context, string, string) (domain.UpstreamAccountEndpoint, error) {
	if len(s.endpoints) == 0 {
		return domain.UpstreamAccountEndpoint{}, domain.ErrNotFound
	}
	return s.endpoints[0], nil
}

func (s *repoStub) CreateEndpoint(_ context.Context, accountID string, write domain.UpstreamAccountEndpointWrite) (domain.UpstreamAccountEndpoint, error) {
	s.lastEndpointWrite = write
	return domain.UpstreamAccountEndpoint{ID: "endpoint-new", AccountID: accountID, APIFormat: write.APIFormat}, nil
}

func (s *repoStub) UpdateEndpoint(_ context.Context, accountID, endpointID string, write domain.UpstreamAccountEndpointWrite) (domain.UpstreamAccountEndpoint, error) {
	s.lastEndpointWrite = write
	return domain.UpstreamAccountEndpoint{ID: endpointID, AccountID: accountID, APIFormat: write.APIFormat}, nil
}

func (s *repoStub) UpdateEndpointHealth(context.Context, string, string, domain.HealthStatus, string) (domain.UpstreamAccountEndpoint, error) {
	return domain.UpstreamAccountEndpoint{}, nil
}

func (s *repoStub) DeleteEndpoint(context.Context, string, string) error { return nil }

func TestUpdateAccountKeepsExistingCiphertextWhenAPIKeyOmitted(t *testing.T) {
	repo := &repoStub{
		account: domain.UpstreamAccount{
			ID:   "acc-1",
			Name: "account",
		},
		secret: AccountSecret{
			Ciphertext: "cipher",
		},
	}
	svc := New(repo, func(plaintext string) (string, error) { return plaintext + "-enc", nil })

	_, err := svc.UpdateAccount(context.Background(), UpdateAccountInput{
		ID:     "acc-1",
		Name:   "account",
		Status: domain.APIKeyStatusActive,
	})
	if err != nil {
		t.Fatalf("UpdateAccount() error = %v", err)
	}
	if repo.lastUpdate.Ciphertext != "cipher" {
		t.Fatalf("ciphertext = %q, want preserved original", repo.lastUpdate.Ciphertext)
	}
}

func TestCreateAccountPersistsConcurrencyLimit(t *testing.T) {
	repo := &repoStub{}
	svc := New(repo, func(plaintext string) (string, error) { return plaintext + "-enc", nil })
	concurrency := 120

	_, err := svc.CreateAccount(context.Background(), CreateAccountInput{
		Name:             "account",
		APIKey:           "secret",
		Endpoints:        testEndpointWrites(),
		ConcurrencyLimit: &concurrency,
	})
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	if repo.lastCreate.ConcurrencyLimit == nil || *repo.lastCreate.ConcurrencyLimit != concurrency {
		t.Fatalf("concurrency limit = %#v, want %d", repo.lastCreate.ConcurrencyLimit, concurrency)
	}
}

func TestCreateAccountRejectsNonPositiveConcurrencyLimit(t *testing.T) {
	repo := &repoStub{}
	svc := New(repo, func(plaintext string) (string, error) { return plaintext, nil })
	concurrency := 0

	_, err := svc.CreateAccount(context.Background(), CreateAccountInput{
		Name:             "account",
		APIKey:           "secret",
		Endpoints:        testEndpointWrites(),
		ConcurrencyLimit: &concurrency,
	})
	if err == nil {
		t.Fatal("CreateAccount() accepted a non-positive concurrency limit")
	}
}

func TestDeleteAccountDelegatesToRepository(t *testing.T) {
	repo := &repoStub{}
	svc := New(repo, nil)

	if err := svc.DeleteAccount(context.Background(), "acc-123"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if repo.deleteAccountID != "acc-123" {
		t.Fatalf("DeleteAccount() repo id = %q, want acc-123", repo.deleteAccountID)
	}
}

func TestUpdateAccountPreservesInvalidStatusWhenStatusOmitted(t *testing.T) {
	repo := &repoStub{
		secret: AccountSecret{
			Ciphertext: "cipher",
			Status:     domain.UpstreamAccountStatusInvalid,
		},
	}
	svc := New(repo, func(plaintext string) (string, error) { return plaintext, nil })

	_, err := svc.UpdateAccount(context.Background(), UpdateAccountInput{
		ID:   "acc-1",
		Name: "account",
	})
	if err != nil {
		t.Fatalf("UpdateAccount() error = %v", err)
	}
	if repo.lastUpdate.Status != domain.UpstreamAccountStatusInvalid {
		t.Fatalf("status = %q, want preserved invalid", repo.lastUpdate.Status)
	}
}

func TestUpdateAccountRejectsAdminSettingInvalidStatus(t *testing.T) {
	repo := &repoStub{secret: AccountSecret{Status: domain.UpstreamAccountStatusActive}}
	svc := New(repo, func(plaintext string) (string, error) { return plaintext, nil })

	_, err := svc.UpdateAccount(context.Background(), UpdateAccountInput{
		ID:     "acc-1",
		Name:   "account",
		Status: domain.UpstreamAccountStatusInvalid,
	})
	if err == nil {
		t.Fatal("UpdateAccount() accepted admin-managed invalid status")
	}
}

func testEndpointWrites() []domain.UpstreamAccountEndpointWrite {
	return []domain.UpstreamAccountEndpointWrite{{
		APIFormat: domain.ProtocolOpenAIResponses,
		BaseURL:   "https://example.com",
		Status:    domain.EndpointStatusActive,
	}}
}

func TestMarkAccountInvalidDelegatesToRepository(t *testing.T) {
	repo := &repoStub{}
	svc := New(repo, nil)

	if _, err := svc.MarkAccountInvalid(context.Background(), "acc-1", "HTTP 401"); err != nil {
		t.Fatalf("MarkAccountInvalid() error = %v", err)
	}
	if repo.invalidAccountID != "acc-1" || repo.invalidReason != "HTTP 401" {
		t.Fatalf("mark invalid = %q/%q", repo.invalidAccountID, repo.invalidReason)
	}
}

func TestCreateAccountRejectsDuplicateEndpointFormats(t *testing.T) {
	svc := New(&repoStub{}, func(plaintext string) (string, error) { return plaintext, nil })
	_, err := svc.CreateAccount(t.Context(), CreateAccountInput{
		Name: "account", APIKey: "secret",
		Endpoints: []domain.UpstreamAccountEndpointWrite{
			{APIFormat: domain.ProtocolOpenAIResponses, BaseURL: "https://one.example"},
			{APIFormat: domain.ProtocolOpenAIResponses, BaseURL: "https://two.example"},
		},
	})
	if err == nil {
		t.Fatal("CreateAccount() accepted duplicate API formats")
	}
}

func TestCreateEndpointNormalizesTransportConfiguration(t *testing.T) {
	repo := &repoStub{}
	svc := New(repo, nil)
	_, err := svc.CreateEndpoint(t.Context(), "account-1", domain.UpstreamAccountEndpointWrite{
		APIFormat:    domain.ProtocolOpenAIResponses,
		BaseURL:      " https://api.example/v1/ ",
		PathOverride: "responses",
		ExtraHeaders: []byte(`{"X-Mode":"strict"}`),
	})
	if err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}
	if repo.lastEndpointWrite.BaseURL != "https://api.example/v1" || repo.lastEndpointWrite.PathOverride != "/responses" {
		t.Fatalf("normalized endpoint = %+v", repo.lastEndpointWrite)
	}
	if string(repo.lastEndpointWrite.ExtraHeaders) != `{"X-Mode":"strict"}` {
		t.Fatalf("normalized headers = %s", repo.lastEndpointWrite.ExtraHeaders)
	}
}

func TestCreateEndpointRejectsNonStringHeaderValues(t *testing.T) {
	svc := New(&repoStub{}, nil)
	_, err := svc.CreateEndpoint(t.Context(), "account-1", domain.UpstreamAccountEndpointWrite{
		APIFormat:    domain.ProtocolOpenAIResponses,
		BaseURL:      "https://api.example",
		ExtraHeaders: []byte(`{"X-Number":42}`),
	})
	if err == nil {
		t.Fatal("CreateEndpoint() accepted a non-string header value")
	}
}

func TestUpdateEndpointPreservesRedactedSensitiveHeaders(t *testing.T) {
	repo := &repoStub{endpoints: []domain.UpstreamAccountEndpoint{{
		ID: "endpoint-1", APIFormat: domain.ProtocolOpenAIResponses, Status: domain.EndpointStatusActive,
		ExtraHeaders: []byte(`{"Authorization":"real-secret","X-Trace":"old"}`),
	}}}
	svc := New(repo, nil)
	_, err := svc.UpdateEndpoint(t.Context(), "account-1", "endpoint-1", domain.UpstreamAccountEndpointWrite{
		APIFormat:    domain.ProtocolOpenAIResponses,
		BaseURL:      "https://api.example",
		ExtraHeaders: []byte(`{"Authorization":"***REDACTED***","X-Trace":"new"}`),
		Status:       domain.EndpointStatusActive,
	})
	if err != nil {
		t.Fatalf("UpdateEndpoint() error = %v", err)
	}
	if string(repo.lastEndpointWrite.ExtraHeaders) != `{"Authorization":"real-secret","X-Trace":"new"}` {
		t.Fatalf("updated headers = %s", repo.lastEndpointWrite.ExtraHeaders)
	}
}

func TestDeleteEndpointKeepsAtLeastOne(t *testing.T) {
	repo := &repoStub{endpoints: []domain.UpstreamAccountEndpoint{{ID: "endpoint-1"}}}
	svc := New(repo, nil)
	if err := svc.DeleteEndpoint(t.Context(), "account-1", "endpoint-1"); err == nil {
		t.Fatal("DeleteEndpoint() deleted the account's last endpoint")
	}
}

func TestDeleteEndpointKeepsLastActiveEndpointForActiveAccount(t *testing.T) {
	repo := &repoStub{
		secret: AccountSecret{Status: domain.UpstreamAccountStatusActive},
		endpoints: []domain.UpstreamAccountEndpoint{
			{ID: "endpoint-active", Status: domain.EndpointStatusActive},
			{ID: "endpoint-disabled", Status: domain.EndpointStatusDisabled},
		},
	}
	svc := New(repo, nil)
	if err := svc.DeleteEndpoint(t.Context(), "account-1", "endpoint-active"); err == nil {
		t.Fatal("DeleteEndpoint() deleted the active account's last active endpoint")
	}
}

func TestEnableAccountRequiresActiveEndpoint(t *testing.T) {
	repo := &repoStub{endpoints: []domain.UpstreamAccountEndpoint{{ID: "endpoint-1", Status: domain.EndpointStatusDisabled}}}
	svc := New(repo, nil)
	if _, err := svc.UpdateAccountStatus(t.Context(), "account-1", domain.UpstreamAccountStatusActive); err == nil {
		t.Fatal("UpdateAccountStatus() enabled an account without active endpoints")
	}
}

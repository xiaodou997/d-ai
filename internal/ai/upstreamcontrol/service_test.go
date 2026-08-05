package upstreamcontrol

import (
	"context"
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

func TestUpdateAccountKeepsExistingCiphertextWhenAPIKeyOmitted(t *testing.T) {
	repo := &repoStub{
		account: domain.UpstreamAccount{
			ID:              "acc-1",
			Name:            "account",
			BaseURL:         "https://example.com",
			DefaultProtocol: string(domain.EndpointProtocolOpenAICompatible),
		},
		secret: AccountSecret{
			Ciphertext:      "cipher",
			DefaultProtocol: string(domain.EndpointProtocolOpenAICompatible),
		},
	}
	svc := New(repo, func(plaintext string) (string, error) { return plaintext + "-enc", nil })

	_, err := svc.UpdateAccount(context.Background(), UpdateAccountInput{
		ID:      "acc-1",
		Name:    "account",
		BaseURL: "https://example.com",
		Status:  domain.APIKeyStatusActive,
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
		BaseURL:          "https://example.com",
		APIKey:           "secret",
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
		BaseURL:          "https://example.com",
		APIKey:           "secret",
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
			Ciphertext:      "cipher",
			DefaultProtocol: string(domain.EndpointProtocolOpenAICompatible),
			Status:          domain.UpstreamAccountStatusInvalid,
		},
	}
	svc := New(repo, func(plaintext string) (string, error) { return plaintext, nil })

	_, err := svc.UpdateAccount(context.Background(), UpdateAccountInput{
		ID:      "acc-1",
		Name:    "account",
		BaseURL: "https://example.com",
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
		ID:      "acc-1",
		Name:    "account",
		BaseURL: "https://example.com",
		Status:  domain.UpstreamAccountStatusInvalid,
	})
	if err == nil {
		t.Fatal("UpdateAccount() accepted admin-managed invalid status")
	}
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

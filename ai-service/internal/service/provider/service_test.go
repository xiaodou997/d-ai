package provider

import (
	"context"
	"errors"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

type mockRepo struct {
	createProvider  ProviderWrite
	updateProvider  ProviderWrite
	endpointCreate  EndpointCreate
	endpointUpdate  EndpointUpdate
	secret          EndpointSecret
	secretErr       error
	deleteErr       error
	deletedProvider string
	deletedEndpoint string
	err             error
}

func (m *mockRepo) CreateProvider(ctx context.Context, w ProviderWrite) (domain.Provider, error) {
	m.createProvider = w
	return domain.Provider{Code: w.Code, Name: w.Name, Status: w.Status}, m.err
}
func (m *mockRepo) ListProviders(ctx context.Context) ([]domain.Provider, error) {
	return nil, m.err
}
func (m *mockRepo) UpdateProvider(ctx context.Context, id string, w ProviderWrite) (domain.Provider, error) {
	m.updateProvider = w
	return domain.Provider{Code: w.Code, Status: w.Status}, m.err
}
func (m *mockRepo) UpdateProviderStatus(ctx context.Context, id, status string) (domain.Provider, error) {
	return domain.Provider{Status: status}, m.err
}
func (m *mockRepo) CreateEndpoint(ctx context.Context, e EndpointCreate) (domain.ProviderEndpoint, error) {
	m.endpointCreate = e
	return domain.ProviderEndpoint{Name: e.Name, Status: e.Status}, m.err
}
func (m *mockRepo) ListEndpoints(ctx context.Context, providerID string) ([]domain.ProviderEndpoint, error) {
	return nil, m.err
}
func (m *mockRepo) GetEndpointSecret(ctx context.Context, providerID, id string) (EndpointSecret, error) {
	return m.secret, m.secretErr
}
func (m *mockRepo) UpdateEndpoint(ctx context.Context, e EndpointUpdate) (domain.ProviderEndpoint, error) {
	m.endpointUpdate = e
	return domain.ProviderEndpoint{Name: e.Name}, m.err
}
func (m *mockRepo) UpdateEndpointStatus(ctx context.Context, providerID, id, status string) (domain.ProviderEndpoint, error) {
	return domain.ProviderEndpoint{Status: status}, m.err
}
func (m *mockRepo) DeleteEndpoint(ctx context.Context, providerID, id string) error {
	m.deletedProvider, m.deletedEndpoint = providerID, id
	return m.deleteErr
}

func fakeEncrypt(plaintext string) (string, error) { return "enc(" + plaintext + ")", nil }

func TestCreateProvider_RequiresCodeAndName(t *testing.T) {
	svc := New(&mockRepo{}, fakeEncrypt)
	if _, err := svc.CreateProvider(context.Background(), ProviderInput{Name: "x"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for missing code, got %v", err)
	}
	if _, err := svc.CreateProvider(context.Background(), ProviderInput{Code: "x"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for missing name, got %v", err)
	}
}

func TestCreateProvider_DefaultsStatus(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, fakeEncrypt)
	if _, err := svc.CreateProvider(context.Background(), ProviderInput{Code: "c", Name: "n"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createProvider.Status != domain.APIKeyStatusActive {
		t.Fatalf("want default active status, got %q", repo.createProvider.Status)
	}
}

func TestCreateEndpoint_RequiresFields(t *testing.T) {
	svc := New(&mockRepo{}, fakeEncrypt)
	_, err := svc.CreateEndpoint(context.Background(), CreateEndpointInput{Name: "n", BaseURL: "u"}) // missing api_key
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreateEndpoint_EncryptsAndDefaults(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, fakeEncrypt)
	_, err := svc.CreateEndpoint(context.Background(), CreateEndpointInput{
		ProviderID: "p", Name: "n", BaseURL: "u", APIKey: "secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.endpointCreate.Ciphertext != "enc(secret)" {
		t.Fatalf("api key not encrypted: %q", repo.endpointCreate.Ciphertext)
	}
	if repo.endpointCreate.Weight != defaultEndpointWeight || repo.endpointCreate.TimeoutMs != defaultTimeoutMs {
		t.Fatalf("defaults not applied: w=%d t=%d", repo.endpointCreate.Weight, repo.endpointCreate.TimeoutMs)
	}
	if repo.endpointCreate.DefaultProtocol != string(domain.EndpointProtocolOpenAICompatible) {
		t.Fatalf("protocol default wrong: %q", repo.endpointCreate.DefaultProtocol)
	}
}

func TestCreateEndpoint_RespectsExplicitWeight(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, fakeEncrypt)
	w := int32(7)
	to := int32(500)
	_, _ = svc.CreateEndpoint(context.Background(), CreateEndpointInput{
		ProviderID: "p", Name: "n", BaseURL: "u", APIKey: "k", Weight: &w, TimeoutMs: &to,
	})
	if repo.endpointCreate.Weight != 7 || repo.endpointCreate.TimeoutMs != 500 {
		t.Fatalf("explicit values overridden: w=%d t=%d", repo.endpointCreate.Weight, repo.endpointCreate.TimeoutMs)
	}
}

func TestUpdateEndpoint_KeepsExistingSecretWhenKeyEmpty(t *testing.T) {
	repo := &mockRepo{secret: EndpointSecret{Ciphertext: "old-cipher", DefaultProtocol: "anthropic_messages"}}
	svc := New(repo, fakeEncrypt)
	_, err := svc.UpdateEndpoint(context.Background(), UpdateEndpointInput{
		ProviderID: "p", ID: "e", Name: "n", BaseURL: "u", // APIKey empty, DefaultProtocol empty
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.endpointUpdate.Ciphertext != "old-cipher" {
		t.Fatalf("want kept old cipher, got %q", repo.endpointUpdate.Ciphertext)
	}
	if repo.endpointUpdate.DefaultProtocol != "anthropic_messages" {
		t.Fatalf("want kept old protocol, got %q", repo.endpointUpdate.DefaultProtocol)
	}
}

func TestUpdateEndpoint_ReencryptsWhenKeyProvided(t *testing.T) {
	repo := &mockRepo{secret: EndpointSecret{Ciphertext: "old", DefaultProtocol: "x"}}
	svc := New(repo, fakeEncrypt)
	_, err := svc.UpdateEndpoint(context.Background(), UpdateEndpointInput{
		ProviderID: "p", ID: "e", Name: "n", BaseURL: "u", APIKey: "new",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.endpointUpdate.Ciphertext != "enc(new)" {
		t.Fatalf("want re-encrypted, got %q", repo.endpointUpdate.Ciphertext)
	}
}

func TestUpdateEndpoint_PropagatesGetSecretError(t *testing.T) {
	repo := &mockRepo{secretErr: domain.ErrNotFound}
	svc := New(repo, fakeEncrypt)
	_, err := svc.UpdateEndpoint(context.Background(), UpdateEndpointInput{ProviderID: "p", ID: "e", Name: "n", BaseURL: "u"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateProviderStatus_RequiresStatus(t *testing.T) {
	svc := New(&mockRepo{}, fakeEncrypt)
	if _, err := svc.UpdateProviderStatus(context.Background(), "id", ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestDeleteEndpoint_PassesThrough(t *testing.T) {
	repo := &mockRepo{deleteErr: domain.ErrNotFound}
	svc := New(repo, fakeEncrypt)
	if err := svc.DeleteEndpoint(context.Background(), "p", "e"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if repo.deletedProvider != "p" || repo.deletedEndpoint != "e" {
		t.Fatalf("ids not passed: %q %q", repo.deletedProvider, repo.deletedEndpoint)
	}
}

func TestUpdateProvider_RequiresCodeAndName(t *testing.T) {
	svc := New(&mockRepo{}, fakeEncrypt)
	if _, err := svc.UpdateProvider(context.Background(), "id", ProviderInput{Name: "x"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestUpdateProvider_DefaultsStatus(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, fakeEncrypt)
	if _, err := svc.UpdateProvider(context.Background(), "id", ProviderInput{Code: "c", Name: "n"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updateProvider.Status != domain.APIKeyStatusActive {
		t.Fatalf("want default active status, got %q", repo.updateProvider.Status)
	}
}

func TestUpdateProviderStatus_OK(t *testing.T) {
	svc := New(&mockRepo{}, fakeEncrypt)
	p, err := svc.UpdateProviderStatus(context.Background(), "id", "inactive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != "inactive" {
		t.Fatalf("want inactive, got %q", p.Status)
	}
}

func TestUpdateEndpoint_RequiresNameAndBaseURL(t *testing.T) {
	svc := New(&mockRepo{}, fakeEncrypt)
	if _, err := svc.UpdateEndpoint(context.Background(), UpdateEndpointInput{ProviderID: "p", ID: "e", BaseURL: "u"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for missing name, got %v", err)
	}
}

func TestUpdateEndpointStatus_RequiresStatusAndOK(t *testing.T) {
	svc := New(&mockRepo{}, fakeEncrypt)
	if _, err := svc.UpdateEndpointStatus(context.Background(), "p", "e", ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
	ep, err := svc.UpdateEndpointStatus(context.Background(), "p", "e", "inactive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Status != "inactive" {
		t.Fatalf("want inactive, got %q", ep.Status)
	}
}

func TestListPassThroughs(t *testing.T) {
	svc := New(&mockRepo{}, fakeEncrypt)
	if _, err := svc.ListProviders(context.Background()); err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if _, err := svc.ListEndpoints(context.Background(), "p"); err != nil {
		t.Fatalf("ListEndpoints: %v", err)
	}
}

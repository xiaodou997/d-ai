package provider

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Default endpoint tuning, mirroring the previous handler-layer constants.
const (
	defaultEndpointWeight int32 = 100
	defaultTimeoutMs      int32 = 30000
)

// Encryptor encrypts a plaintext provider API key into its stored ciphertext.
// Injected so the service can be unit-tested without the real master key.
type Encryptor func(plaintext string) (string, error)

// Service implements provider & endpoint management business logic.
type Service struct {
	repo    Repository
	encrypt Encryptor
}

// New builds a Service. encrypt must be non-nil.
func New(repo Repository, encrypt Encryptor) *Service {
	return &Service{repo: repo, encrypt: encrypt}
}

// ---- Provider ----

// ProviderInput is the decoded create/update provider request.
type ProviderInput struct {
	Code   string
	Name   string
	Config []byte
	Status string
}

func (s *Service) CreateProvider(ctx context.Context, in ProviderInput) (domain.Provider, error) {
	if in.Code == "" || in.Name == "" {
		return domain.Provider{}, domain.NewValidationError("", "code and name are required")
	}
	return s.repo.CreateProvider(ctx, ProviderWrite{
		Code:   in.Code,
		Name:   in.Name,
		Config: in.Config,
		Status: defaultStatus(in.Status),
	})
}

func (s *Service) ListProviders(ctx context.Context) ([]domain.Provider, error) {
	return s.repo.ListProviders(ctx)
}

func (s *Service) UpdateProvider(ctx context.Context, id string, in ProviderInput) (domain.Provider, error) {
	if in.Code == "" || in.Name == "" {
		return domain.Provider{}, domain.NewValidationError("", "code and name are required")
	}
	return s.repo.UpdateProvider(ctx, id, ProviderWrite{
		Code:   in.Code,
		Name:   in.Name,
		Config: in.Config,
		Status: defaultStatus(in.Status),
	})
}

func (s *Service) UpdateProviderStatus(ctx context.Context, id, status string) (domain.Provider, error) {
	if status == "" {
		return domain.Provider{}, domain.NewValidationError("status", "status is required")
	}
	return s.repo.UpdateProviderStatus(ctx, id, status)
}

// ---- Endpoint ----

// CreateEndpointInput is the decoded create-endpoint request. Weight/TimeoutMs
// are nil to request the service defaults.
type CreateEndpointInput struct {
	ProviderID      string
	Name            string
	BaseURL         string
	APIKey          string
	ExtraHeaders    []byte
	Weight          *int32
	TimeoutMs       *int32
	DefaultProtocol string
	PriceBookID     string
	CostMultiplier  *float64
	Status          string
}

func (s *Service) CreateEndpoint(ctx context.Context, in CreateEndpointInput) (domain.ProviderEndpoint, error) {
	if in.Name == "" || in.BaseURL == "" || in.APIKey == "" {
		return domain.ProviderEndpoint{}, domain.NewValidationError("", "name, base_url and api_key are required")
	}
	ciphertext, err := s.encrypt(in.APIKey)
	if err != nil {
		return domain.ProviderEndpoint{}, domain.NewValidationError("api_key", err.Error())
	}
	protocol := in.DefaultProtocol
	if protocol == "" {
		protocol = string(domain.EndpointProtocolOpenAICompatible)
	}
	return s.repo.CreateEndpoint(ctx, EndpointCreate{
		ProviderID:      in.ProviderID,
		Name:            in.Name,
		BaseURL:         in.BaseURL,
		Ciphertext:      ciphertext,
		ExtraHeaders:    in.ExtraHeaders,
		Weight:          int32OrDefault(in.Weight, defaultEndpointWeight),
		TimeoutMs:       int32OrDefault(in.TimeoutMs, defaultTimeoutMs),
		DefaultProtocol: protocol,
		PriceBookID:     in.PriceBookID,
		CostMultiplier:  in.CostMultiplier,
		Status:          defaultStatus(in.Status),
	})
}

func (s *Service) ListEndpoints(ctx context.Context, providerID string) ([]domain.ProviderEndpoint, error) {
	return s.repo.ListEndpoints(ctx, providerID)
}

// UpdateEndpointInput is the decoded update-endpoint request. An empty APIKey
// keeps the existing secret; an empty DefaultProtocol keeps the existing one.
type UpdateEndpointInput struct {
	ProviderID      string
	ID              string
	Name            string
	BaseURL         string
	APIKey          string
	ExtraHeaders    []byte
	Weight          *int32
	TimeoutMs       *int32
	DefaultProtocol string
	PriceBookID     string
	CostMultiplier  *float64
	Status          string
}

func (s *Service) UpdateEndpoint(ctx context.Context, in UpdateEndpointInput) (domain.ProviderEndpoint, error) {
	if in.Name == "" || in.BaseURL == "" {
		return domain.ProviderEndpoint{}, domain.NewValidationError("", "name and base_url are required")
	}
	current, err := s.repo.GetEndpointSecret(ctx, in.ProviderID, in.ID)
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	ciphertext := current.Ciphertext
	if in.APIKey != "" {
		ciphertext, err = s.encrypt(in.APIKey)
		if err != nil {
			return domain.ProviderEndpoint{}, domain.NewValidationError("api_key", err.Error())
		}
	}
	protocol := in.DefaultProtocol
	if protocol == "" {
		protocol = current.DefaultProtocol
	}
	return s.repo.UpdateEndpoint(ctx, EndpointUpdate{
		ProviderID:      in.ProviderID,
		ID:              in.ID,
		Name:            in.Name,
		BaseURL:         in.BaseURL,
		Ciphertext:      ciphertext,
		ExtraHeaders:    in.ExtraHeaders,
		Weight:          int32OrDefault(in.Weight, defaultEndpointWeight),
		TimeoutMs:       int32OrDefault(in.TimeoutMs, defaultTimeoutMs),
		DefaultProtocol: protocol,
		PriceBookID:     in.PriceBookID,
		CostMultiplier:  in.CostMultiplier,
		Status:          defaultStatus(in.Status),
	})
}

func (s *Service) UpdateEndpointStatus(ctx context.Context, providerID, id, status string) (domain.ProviderEndpoint, error) {
	if status == "" {
		return domain.ProviderEndpoint{}, domain.NewValidationError("status", "status is required")
	}
	return s.repo.UpdateEndpointStatus(ctx, providerID, id, status)
}

func (s *Service) DeleteEndpoint(ctx context.Context, providerID, id string) error {
	return s.repo.DeleteEndpoint(ctx, providerID, id)
}

func defaultStatus(status string) string {
	if status == "" {
		return domain.APIKeyStatusActive
	}
	return status
}

func int32OrDefault(v *int32, def int32) int32 {
	if v == nil || *v == 0 {
		return def
	}
	return *v
}

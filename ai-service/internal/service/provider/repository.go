// Package provider holds the business logic for provider & provider-endpoint
// management (the console management plane). Service owns validation, default
// filling, API-key encryption and the "keep existing secret on update" rule;
// persistence is reached through Repository, defined on the consumer side.
package provider

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the provider service.
type Repository interface {
	CreateProvider(ctx context.Context, w ProviderWrite) (domain.Provider, error)
	ListProviders(ctx context.Context) ([]domain.Provider, error)
	UpdateProvider(ctx context.Context, id string, w ProviderWrite) (domain.Provider, error)
	UpdateProviderStatus(ctx context.Context, id, status string) (domain.Provider, error)

	CreateEndpoint(ctx context.Context, e EndpointCreate) (domain.ProviderEndpoint, error)
	ListEndpoints(ctx context.Context, providerID string) ([]domain.ProviderEndpoint, error)
	// GetEndpointSecret returns the fields needed to apply the update-time
	// "keep existing" rules: the current ciphertext and default protocol.
	GetEndpointSecret(ctx context.Context, providerID, id string) (EndpointSecret, error)
	UpdateEndpoint(ctx context.Context, e EndpointUpdate) (domain.ProviderEndpoint, error)
	UpdateEndpointStatus(ctx context.Context, providerID, id, status string) (domain.ProviderEndpoint, error)
	// DeleteEndpoint removes an endpoint, returning domain.ErrNotFound when no
	// row matched.
	DeleteEndpoint(ctx context.Context, providerID, id string) error
}

// ProviderWrite is the persistence-level payload for create/update provider.
type ProviderWrite struct {
	Code   string
	Name   string
	Config []byte // JSON object; repo normalises empty to "{}"
	Status string
}

// EndpointCreate is the persistence-level payload for creating an endpoint.
// Ciphertext is already encrypted by the service.
type EndpointCreate struct {
	ProviderID      string
	Name            string
	BaseURL         string
	Ciphertext      string
	ExtraHeaders    []byte
	Weight          int32
	TimeoutMs       int32
	DefaultProtocol string
	PriceBookID     string
	CostMultiplier  *float64
	Status          string
}

// EndpointUpdate is the persistence-level payload for updating an endpoint.
type EndpointUpdate struct {
	ProviderID      string
	ID              string
	Name            string
	BaseURL         string
	Ciphertext      string
	ExtraHeaders    []byte
	Weight          int32
	TimeoutMs       int32
	DefaultProtocol string
	PriceBookID     string
	CostMultiplier  *float64
	Status          string
}

// EndpointSecret carries the current sensitive/preserved fields of an endpoint.
type EndpointSecret struct {
	Ciphertext      string
	DefaultProtocol string
}

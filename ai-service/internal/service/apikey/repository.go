// Package apikey holds the business logic for API key management (the console
// management plane). Handlers call Service with decoded input; Service owns
// validation, key generation, credit-unit conversion and cache invalidation,
// returning domain types. Persistence is reached through Repository, defined
// here on the consumer side so Service can be unit-tested with a mock.
package apikey

import (
	"context"
	"time"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the apikey service. The
// postgres adapter provides the production implementation.
type Repository interface {
	Create(ctx context.Context, p CreateParams) (domain.APIKey, error)
	List(ctx context.Context, f ListFilter) ([]domain.APIKey, error)
	// Update persists changes and returns the updated key plus its key hash, so
	// the service can invalidate the auth cache keyed by that hash.
	Update(ctx context.Context, p UpdateParams) (key domain.APIKey, keyHash string, err error)
	// UpdateStatus changes the lifecycle status and returns the key hash for the
	// same cache-invalidation reason.
	UpdateStatus(ctx context.Context, id, tenantID, status string) (key domain.APIKey, keyHash string, err error)
	// Rotate swaps the key hash/last-four and returns the updated key together
	// with the previous key hash, so the caller can invalidate caches keyed by
	// the old hash (the new key is not yet cached).
	Rotate(ctx context.Context, p RotateParams) (key domain.APIKey, oldKeyHash string, err error)
	// Delete removes the key and returns its hash for cache invalidation.
	Delete(ctx context.Context, id, tenantID string) (keyHash string, err error)
}

// KeyCache is the optional cache-invalidation port. *apikey.Cache satisfies it.
type KeyCache interface {
	Del(ctx context.Context, keyHash string) error
}

// CreateParams is the persistence-level input for creating a key. Quota is
// already converted to micro-credits by the service.
type CreateParams struct {
	OwnerType       domain.OwnerType
	TenantID        string
	UserID          string // empty for tenant keys
	KeyHash         string
	LastFour        string
	Name            string
	QuotaLimitMicro *int64
	AllowedModels   []string
	Status          string
	ExpiresAt       *time.Time
	CreatedBy       string
}

// ListFilter scopes a key listing. Empty OwnerType/UserID mean "any".
type ListFilter struct {
	TenantID  string
	OwnerType domain.OwnerType
	UserID    string
}

// UpdateParams is the persistence-level input for updating a key.
type UpdateParams struct {
	ID              string
	TenantID        string
	Name            string
	QuotaLimitMicro *int64
	AllowedModels   []string
	Status          string
	ExpiresAt       *time.Time
}

// RotateParams carries the freshly generated hash/last-four for a rotation.
type RotateParams struct {
	ID       string
	TenantID string
	KeyHash  string
	LastFour string
}

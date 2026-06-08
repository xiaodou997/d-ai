package apikey

import (
	"context"
	"time"

	keys "xiaodou/unihub/ai-service/internal/apikey"
	"xiaodou/unihub/ai-service/internal/credits"
	"xiaodou/unihub/ai-service/internal/domain"
)

// maxCreditsPerField caps any single credit-denominated field, mirroring the
// previous handler-layer validation (1,000,000 credits ≈ ¥10,000), guarding
// against entering micro-credits where whole credits are expected.
const maxCreditsPerField int64 = 1_000_000

// Service implements API key management business logic.
type Service struct {
	repo  Repository
	cache KeyCache // optional; nil disables cache invalidation
}

// New builds a Service. cache may be nil (e.g. when Redis is disabled).
func New(repo Repository, cache KeyCache) *Service {
	return &Service{repo: repo, cache: cache}
}

// Created bundles a newly minted key with its one-time plaintext value. The
// plaintext is returned exactly once and never persisted.
type Created struct {
	Key          domain.APIKey
	PlaintextKey string
}

// CreateInput is the decoded create request. QuotaLimitCredits is in whole
// display credits (the API contract unit); the service converts to micro.
// ExpiresAt is already parsed by the handler at the decode boundary (nil = no
// expiry).
type CreateInput struct {
	OwnerType         domain.OwnerType
	TenantID          string
	UserID            string // required for user keys, empty for tenant keys
	Name              string
	QuotaLimitCredits *int64
	AllowedModels     []string
	Status            string
	ExpiresAt         *time.Time
	CreatedBy         string
}

// Create validates input, mints a key and persists it.
func (s *Service) Create(ctx context.Context, in CreateInput) (Created, error) {
	if in.Name == "" {
		return Created{}, domain.NewValidationError("name", "name is required")
	}
	if err := validateOptionalCredits("quota_limit_credits", in.QuotaLimitCredits); err != nil {
		return Created{}, err
	}
	status := in.Status
	if status == "" {
		status = domain.APIKeyStatusActive
	}
	plaintext, err := keys.Generate()
	if err != nil {
		return Created{}, err
	}
	key, err := s.repo.Create(ctx, CreateParams{
		OwnerType:       in.OwnerType,
		TenantID:        in.TenantID,
		UserID:          in.UserID,
		KeyHash:         keys.Hash(plaintext),
		LastFour:        keys.LastFour(plaintext),
		Name:            in.Name,
		QuotaLimitMicro: creditsToMicroPtr(in.QuotaLimitCredits),
		AllowedModels:   in.AllowedModels,
		Status:          status,
		ExpiresAt:       in.ExpiresAt,
		CreatedBy:       in.CreatedBy,
	})
	if err != nil {
		return Created{}, err
	}
	return Created{Key: key, PlaintextKey: plaintext}, nil
}

// ListForTenant returns tenant-owned keys for a tenant.
func (s *Service) ListForTenant(ctx context.Context, tenantID string) ([]domain.APIKey, error) {
	return s.repo.List(ctx, ListFilter{TenantID: tenantID, OwnerType: domain.OwnerTenant})
}

// ListForUser returns user-owned keys for a specific user within a tenant.
func (s *Service) ListForUser(ctx context.Context, tenantID, userID string) ([]domain.APIKey, error) {
	return s.repo.List(ctx, ListFilter{TenantID: tenantID, OwnerType: domain.OwnerUser, UserID: userID})
}

// UpdateInput is the decoded update request. ExpiresAt is already parsed by the
// handler at the decode boundary (nil = no expiry).
type UpdateInput struct {
	ID                string
	TenantID          string
	Name              string
	QuotaLimitCredits *int64
	AllowedModels     []string
	Status            string
	ExpiresAt         *time.Time
}

// Update validates input and persists the changes.
func (s *Service) Update(ctx context.Context, in UpdateInput) (domain.APIKey, error) {
	if in.Name == "" {
		return domain.APIKey{}, domain.NewValidationError("name", "name is required")
	}
	if err := validateOptionalCredits("quota_limit_credits", in.QuotaLimitCredits); err != nil {
		return domain.APIKey{}, err
	}
	key, keyHash, err := s.repo.Update(ctx, UpdateParams{
		ID:              in.ID,
		TenantID:        in.TenantID,
		Name:            in.Name,
		QuotaLimitMicro: creditsToMicroPtr(in.QuotaLimitCredits),
		AllowedModels:   in.AllowedModels,
		Status:          in.Status,
		ExpiresAt:       in.ExpiresAt,
	})
	if err != nil {
		return domain.APIKey{}, err
	}
	// Quota/allowed-models/status changes must take effect immediately, so drop
	// the cached auth row keyed by this hash (otherwise stale until the TTL).
	s.invalidate(ctx, keyHash)
	return key, nil
}

// UpdateStatus changes only the lifecycle status.
func (s *Service) UpdateStatus(ctx context.Context, id, tenantID, status string) (domain.APIKey, error) {
	if status == "" {
		return domain.APIKey{}, domain.NewValidationError("status", "status is required")
	}
	key, keyHash, err := s.repo.UpdateStatus(ctx, id, tenantID, status)
	if err != nil {
		return domain.APIKey{}, err
	}
	s.invalidate(ctx, keyHash)
	return key, nil
}

// Rotate mints a new secret for an existing key and invalidates the old hash.
func (s *Service) Rotate(ctx context.Context, id, tenantID string) (Created, error) {
	plaintext, err := keys.Generate()
	if err != nil {
		return Created{}, err
	}
	key, oldHash, err := s.repo.Rotate(ctx, RotateParams{
		ID:       id,
		TenantID: tenantID,
		KeyHash:  keys.Hash(plaintext),
		LastFour: keys.LastFour(plaintext),
	})
	if err != nil {
		return Created{}, err
	}
	s.invalidate(ctx, oldHash)
	return Created{Key: key, PlaintextKey: plaintext}, nil
}

// Delete removes a key and invalidates its cache entry.
func (s *Service) Delete(ctx context.Context, id, tenantID string) error {
	keyHash, err := s.repo.Delete(ctx, id, tenantID)
	if err != nil {
		return err
	}
	s.invalidate(ctx, keyHash)
	return nil
}

func (s *Service) invalidate(ctx context.Context, keyHash string) {
	if s.cache == nil || keyHash == "" {
		return
	}
	_ = s.cache.Del(ctx, keyHash)
}

func validateOptionalCredits(field string, v *int64) error {
	if v == nil {
		return nil
	}
	if *v < 0 {
		return domain.NewValidationError(field, field+" must be a non-negative credit value")
	}
	if *v > maxCreditsPerField {
		return domain.NewValidationError(field, field+" exceeds maximum allowed value")
	}
	return nil
}

func creditsToMicroPtr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	micro := credits.WholeCreditsToMicro(*v)
	return &micro
}

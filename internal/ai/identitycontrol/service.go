package identitycontrol

import (
	"context"
	"strings"
	"time"

	keys "xiaodou/dai/internal/ai/apikey"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/credits"
	"xiaodou/dai/internal/ai/domain"
)

// maxCreditsPerField caps any single credit-denominated field, mirroring the
// previous handler-layer validation (1,000,000 credits ≈ ¥10,000), guarding
// against entering micro-credits where whole credits are expected.
const maxCreditsPerField int64 = 1_000_000

// Service implements API key management business logic.
type Service struct {
	repo    Repository
	cache   KeyCache // optional; nil disables cache invalidation
	encrypt Encryptor
	decrypt Decryptor
}

type Encryptor func(string) (string, error)
type Decryptor func(string) (string, error)

// New builds a Service. cache may be nil (e.g. when Redis is disabled).
func New(repo Repository, cache KeyCache, encrypt Encryptor, decrypt Decryptor) *Service {
	return &Service{repo: repo, cache: cache, encrypt: encrypt, decrypt: decrypt}
}

// Created bundles a newly minted key with its one-time plaintext value. The
// plaintext is returned exactly once and never persisted.
type Created struct {
	Key          identity.APIKey
	PlaintextKey string
}

// CreateInput is the decoded create request. QuotaLimitCredits is in whole
// display credits (the API contract unit); the service converts to micro.
// ExpiresAt is already parsed by the handler at the decode boundary (nil = no
// expiry).
type CreateInput struct {
	OwnerScope        identity.Scope
	TenantID          string
	UserID            string // required for user keys, empty for tenant keys
	GroupID           string
	Name              string
	QuotaLimitCredits *int64
	AllowedModelIDs   []string
	Status            string
	ExpiresAt         *time.Time
	CreatedBy         string
}

// UpdateInput is the decoded update request. ExpiresAt is already parsed by the
// handler at the decode boundary (nil = no expiry).
type UpdateInput struct {
	ID                string
	TenantID          string
	GroupID           string
	Name              string
	QuotaLimitCredits *int64
	AllowedModelIDs   []string
	Status            string
	ExpiresAt         *time.Time
}

// Create validates input, mints a key and persists it.
func (s *Service) Create(ctx context.Context, in CreateInput) (Created, error) {
	if in.Name == "" {
		return Created{}, domain.NewValidationError("name", "name is required")
	}
	if strings.TrimSpace(in.GroupID) == "" {
		return Created{}, domain.NewValidationError("group_id", "group_id is required")
	}
	if err := validateOptionalCredits("quota_limit_credits", in.QuotaLimitCredits); err != nil {
		return Created{}, err
	}
	status, err := normalizeAPIKeyStatus(in.Status, true)
	if err != nil {
		return Created{}, err
	}
	plaintext, err := keys.Generate()
	if err != nil {
		return Created{}, err
	}
	if s.encrypt == nil {
		return Created{}, domain.NewValidationError("api_key", "api key encryptor is not configured")
	}
	ciphertext, err := s.encrypt(plaintext)
	if err != nil {
		return Created{}, err
	}
	key, err := s.repo.Create(ctx, identity.APIKeyCreate{
		OwnerScope:      in.OwnerScope,
		TenantID:        in.TenantID,
		UserID:          in.UserID,
		GroupID:         strings.TrimSpace(in.GroupID),
		KeyHash:         keys.Hash(plaintext),
		KeyCiphertext:   ciphertext,
		LastFour:        keys.LastFour(plaintext),
		Name:            in.Name,
		QuotaLimitMicro: creditsToMicroPtr(in.QuotaLimitCredits),
		AllowedModelIDs: append([]string(nil), in.AllowedModelIDs...),
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
func (s *Service) ListForTenant(ctx context.Context, tenantID string) ([]identity.APIKey, error) {
	return s.repo.List(ctx, identity.APIKeyListFilter{TenantID: tenantID, OwnerScope: identity.ScopeTenant})
}

// ListForUser returns user-owned keys for a specific user within a tenant.
func (s *Service) ListForUser(ctx context.Context, tenantID, userID string) ([]identity.APIKey, error) {
	return s.repo.List(ctx, identity.APIKeyListFilter{TenantID: tenantID, OwnerScope: identity.ScopeUser, UserID: userID})
}

// Update validates input and persists the changes.
func (s *Service) Update(ctx context.Context, in UpdateInput) (identity.APIKey, error) {
	if in.Name == "" {
		return identity.APIKey{}, domain.NewValidationError("name", "name is required")
	}
	if strings.TrimSpace(in.GroupID) == "" {
		return identity.APIKey{}, domain.NewValidationError("group_id", "group_id is required")
	}
	if err := validateOptionalCredits("quota_limit_credits", in.QuotaLimitCredits); err != nil {
		return identity.APIKey{}, err
	}
	status, err := normalizeAPIKeyStatus(in.Status, true)
	if err != nil {
		return identity.APIKey{}, err
	}
	key, keyHash, err := s.repo.Update(ctx, identity.APIKeyUpdate{
		ID:              in.ID,
		TenantID:        in.TenantID,
		GroupID:         strings.TrimSpace(in.GroupID),
		Name:            in.Name,
		QuotaLimitMicro: creditsToMicroPtr(in.QuotaLimitCredits),
		AllowedModelIDs: append([]string(nil), in.AllowedModelIDs...),
		Status:          status,
		ExpiresAt:       in.ExpiresAt,
	})
	if err != nil {
		return identity.APIKey{}, err
	}
	s.invalidate(ctx, keyHash)
	return key, nil
}

// UpdateStatus changes only the lifecycle status.
func (s *Service) UpdateStatus(ctx context.Context, id, tenantID, status string) (identity.APIKey, error) {
	status, err := normalizeAPIKeyStatus(status, false)
	if err != nil {
		return identity.APIKey{}, err
	}
	key, keyHash, err := s.repo.UpdateStatus(ctx, id, tenantID, status)
	if err != nil {
		return identity.APIKey{}, err
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
	if s.encrypt == nil {
		return Created{}, domain.NewValidationError("api_key", "api key encryptor is not configured")
	}
	ciphertext, err := s.encrypt(plaintext)
	if err != nil {
		return Created{}, err
	}
	key, oldHash, err := s.repo.Rotate(ctx, identity.APIKeyRotate{
		ID:            id,
		TenantID:      tenantID,
		KeyHash:       keys.Hash(plaintext),
		KeyCiphertext: ciphertext,
		LastFour:      keys.LastFour(plaintext),
	})
	if err != nil {
		return Created{}, err
	}
	s.invalidate(ctx, oldHash)
	return Created{Key: key, PlaintextKey: plaintext}, nil
}

func (s *Service) Reveal(ctx context.Context, id, tenantID string) (string, error) {
	if s.decrypt == nil {
		return "", domain.NewValidationError("api_key", "api key decryptor is not configured")
	}
	ciphertext, err := s.repo.Reveal(ctx, id, tenantID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(ciphertext) == "" {
		return "", domain.ErrNotFound
	}
	return s.decrypt(ciphertext)
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

func normalizeAPIKeyStatus(status string, allowDefault bool) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		if allowDefault {
			return domain.APIKeyStatusActive, nil
		}
		return "", domain.NewValidationError("status", "status is required")
	}
	switch status {
	case domain.APIKeyStatusActive, domain.APIKeyStatusDisabled:
		return status, nil
	default:
		return "", domain.NewValidationError("status", "status must be active or disabled")
	}
}

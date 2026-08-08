package billingcontrol

import (
	"context"
	"fmt"
	"math"
	"strings"

	corebilling "xiaodou/dai/internal/ai/core/billing"
	"xiaodou/dai/internal/ai/domain"
)

const maxPricePerToken = 1.0
const maxMultiplier = 10000.0

// Service implements price book and sell binding business logic.
type Service struct {
	repo      Repository
	llmSource *liteLLMPriceSource
}

func New(repo Repository, fetcher LiteLLMFetcher) *Service {
	return &Service{
		repo:      repo,
		llmSource: newLiteLLMPriceSource(fetcher, llmCacheTTL, llmRetryDelay),
	}
}

// Start begins the non-blocking LiteLLM price cache warmup. The supplied
// context controls in-flight background refreshes during service shutdown.
func (s *Service) Start(ctx context.Context) {
	s.llmSource.Start(ctx)
}

func (s *Service) CreatePriceBook(ctx context.Context, name, description string) (domain.PriceBook, error) {
	return s.createPriceBook(ctx, domain.PriceBookOwnerPlatform, "", name, description)
}

func (s *Service) CreateTenantPriceBook(ctx context.Context, tenantID, name, description string) (domain.PriceBook, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return domain.PriceBook{}, domain.NewValidationError("tenant_id", "tenant_id is required")
	}
	return s.createPriceBook(ctx, domain.PriceBookOwnerTenant, tenantID, name, description)
}

func (s *Service) createPriceBook(ctx context.Context, ownerType domain.PriceBookOwnerType, ownerTenantID, name, description string) (domain.PriceBook, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.PriceBook{}, domain.NewValidationError("name", "name is required")
	}
	return s.repo.CreatePriceBook(ctx, ownerType, ownerTenantID, name, strings.TrimSpace(description))
}

func (s *Service) GetPriceBook(ctx context.Context, id string) (domain.PriceBook, error) {
	return s.repo.GetPriceBook(ctx, id)
}

func (s *Service) GetPlatformPriceBook(ctx context.Context, id string) (domain.PriceBook, error) {
	if err := s.requireOwner(ctx, id, domain.PriceBookOwnerPlatform, ""); err != nil {
		return domain.PriceBook{}, err
	}
	return s.repo.GetPriceBook(ctx, id)
}

func (s *Service) GetVisiblePriceBook(ctx context.Context, tenantID, id string) (domain.PriceBook, error) {
	book, err := s.repo.GetPriceBook(ctx, id)
	if err != nil {
		return domain.PriceBook{}, err
	}
	if book.OwnerType == domain.PriceBookOwnerPlatform ||
		(book.OwnerType == domain.PriceBookOwnerTenant && book.OwnerTenantID == strings.TrimSpace(tenantID)) {
		return book, nil
	}
	return domain.PriceBook{}, domain.ErrNotFound
}

func (s *Service) ListPriceBooks(ctx context.Context) ([]domain.PriceBook, error) {
	return s.repo.ListPriceBooks(ctx)
}

func (s *Service) ListVisiblePriceBooks(ctx context.Context, tenantID string) ([]domain.PriceBook, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, domain.NewValidationError("tenant_id", "tenant_id is required")
	}
	return s.repo.ListVisiblePriceBooks(ctx, tenantID)
}

func (s *Service) UpdatePriceBook(ctx context.Context, id, name, description, status string) (domain.PriceBook, error) {
	return s.updatePriceBook(ctx, domain.PriceBookOwnerPlatform, "", id, name, description, status)
}

func (s *Service) UpdateTenantPriceBook(ctx context.Context, tenantID, id, name, description, status string) (domain.PriceBook, error) {
	return s.updatePriceBook(ctx, domain.PriceBookOwnerTenant, tenantID, id, name, description, status)
}

func (s *Service) updatePriceBook(ctx context.Context, ownerType domain.PriceBookOwnerType, ownerTenantID, id, name, description, status string) (domain.PriceBook, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.PriceBook{}, domain.NewValidationError("name", "name is required")
	}
	if status != "active" && status != "disabled" {
		return domain.PriceBook{}, domain.NewValidationError("status", "status must be active or disabled")
	}
	if err := s.requireOwner(ctx, id, ownerType, ownerTenantID); err != nil {
		return domain.PriceBook{}, err
	}
	return s.repo.UpdatePriceBook(ctx, id, name, strings.TrimSpace(description), status)
}

func (s *Service) DeletePriceBook(ctx context.Context, id string) error {
	return s.deletePriceBook(ctx, domain.PriceBookOwnerPlatform, "", id)
}

func (s *Service) DeleteTenantPriceBook(ctx context.Context, tenantID, id string) error {
	return s.deletePriceBook(ctx, domain.PriceBookOwnerTenant, tenantID, id)
}

func (s *Service) deletePriceBook(ctx context.Context, ownerType domain.PriceBookOwnerType, ownerTenantID, id string) error {
	if err := s.requireOwner(ctx, id, ownerType, ownerTenantID); err != nil {
		return err
	}
	refs, err := s.repo.CountPriceBookReferences(ctx, id)
	if err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("price book is still referenced by %d resource(s), unbind it before deleting: %w", refs, domain.ErrConflict)
	}
	return s.repo.DeletePriceBook(ctx, id)
}

func (s *Service) UpsertEntry(ctx context.Context, e domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	return s.upsertEntry(ctx, domain.PriceBookOwnerPlatform, "", e)
}

func (s *Service) UpsertTenantEntry(ctx context.Context, tenantID string, e domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	return s.upsertEntry(ctx, domain.PriceBookOwnerTenant, tenantID, e)
}

func (s *Service) upsertEntry(ctx context.Context, ownerType domain.PriceBookOwnerType, ownerTenantID string, e domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	if err := validateEntry(e); err != nil {
		return domain.PriceBookEntry{}, err
	}
	if err := s.requireOwner(ctx, e.PriceBookID, ownerType, ownerTenantID); err != nil {
		return domain.PriceBookEntry{}, domain.NewValidationError("price_book_id", "price book does not exist")
	}
	return s.repo.UpsertEntry(ctx, e)
}

func (s *Service) GetEntry(ctx context.Context, priceBookID, modelCode, capabilityType string) (domain.PriceBookEntry, error) {
	return s.repo.GetEntry(ctx, priceBookID, modelCode, capabilityType)
}

func (s *Service) ListEntries(ctx context.Context, priceBookID string) ([]domain.PriceBookEntry, error) {
	if err := s.requireOwner(ctx, priceBookID, domain.PriceBookOwnerPlatform, ""); err != nil {
		return nil, err
	}
	return s.repo.ListEntries(ctx, priceBookID)
}

func (s *Service) ListVisibleEntries(ctx context.Context, tenantID, priceBookID string) ([]domain.PriceBookEntry, error) {
	if _, err := s.GetVisiblePriceBook(ctx, tenantID, priceBookID); err != nil {
		return nil, err
	}
	return s.repo.ListEntries(ctx, priceBookID)
}

func (s *Service) CloneVisiblePriceBook(ctx context.Context, tenantID, sourceID, name string) (domain.PriceBook, error) {
	source, err := s.GetVisiblePriceBook(ctx, tenantID, sourceID)
	if err != nil {
		return domain.PriceBook{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = source.Name + " copy"
	}
	clone, err := s.CreateTenantPriceBook(ctx, tenantID, name, source.Description)
	if err != nil {
		return domain.PriceBook{}, err
	}
	entries, err := s.repo.ListEntries(ctx, sourceID)
	if err != nil {
		return domain.PriceBook{}, err
	}
	for _, entry := range entries {
		entry.ID = ""
		entry.PriceBookID = clone.ID
		if _, err := s.upsertEntry(ctx, domain.PriceBookOwnerTenant, tenantID, entry); err != nil {
			return domain.PriceBook{}, err
		}
	}
	return s.repo.GetPriceBook(ctx, clone.ID)
}

func (s *Service) DeleteEntry(ctx context.Context, priceBookID, modelCode, capabilityType string) error {
	if err := s.requireOwner(ctx, priceBookID, domain.PriceBookOwnerPlatform, ""); err != nil {
		return err
	}
	return s.repo.DeleteEntry(ctx, priceBookID, modelCode, capabilityType)
}

func (s *Service) DeleteTenantEntry(ctx context.Context, tenantID, priceBookID, modelCode, capabilityType string) error {
	if err := s.requireOwner(ctx, priceBookID, domain.PriceBookOwnerTenant, tenantID); err != nil {
		return err
	}
	return s.repo.DeleteEntry(ctx, priceBookID, modelCode, capabilityType)
}

func (s *Service) requireOwner(ctx context.Context, id string, ownerType domain.PriceBookOwnerType, ownerTenantID string) error {
	book, err := s.repo.GetPriceBook(ctx, id)
	if err != nil {
		return err
	}
	if book.OwnerType != ownerType || book.OwnerTenantID != strings.TrimSpace(ownerTenantID) {
		return domain.ErrNotFound
	}
	return nil
}

var validCapabilities = map[string]struct{}{
	"chat": {}, "image": {}, "video": {}, "embedding": {},
	"audio_tts": {}, "audio_stt": {}, "rerank": {},
}

func validateEntry(e domain.PriceBookEntry) error {
	if strings.TrimSpace(e.PriceBookID) == "" {
		return domain.NewValidationError("price_book_id", "price_book_id is required")
	}
	if strings.TrimSpace(e.ModelCode) == "" {
		return domain.NewValidationError("model_code", "model_code is required")
	}
	if _, ok := validCapabilities[e.CapabilityType]; !ok {
		return domain.NewValidationError("capability_type", "invalid capability_type")
	}
	scalars := []struct {
		field string
		value float64
	}{
		{"image_default_price", e.ImageDefaultPrice},
		{"video_default_price", e.VideoDefaultPrice},
		{"audio_tts_per_char", e.AudioTTSPerChar},
		{"audio_stt_per_minute", e.AudioSTTPerMinute},
	}
	for _, sc := range scalars {
		if err := validatePrice(sc.field, sc.value); err != nil {
			return err
		}
	}
	if corebilling.IsTokenPricedCapability(e.CapabilityType) {
		if err := corebilling.ValidateTokenPriceTiers(e.TokenPriceTiers); err != nil {
			return domain.NewValidationError("token_price_tiers", err.Error())
		}
		for _, tier := range e.TokenPriceTiers {
			for _, price := range []float64{tier.InputPerToken, tier.OutputPerToken, tier.CacheWritePerToken, tier.CacheReadPerToken} {
				if price > maxPricePerToken {
					return domain.NewValidationError("token_price_tiers", "price exceeds maximum")
				}
			}
		}
	}
	switch e.CapabilityType {
	case "image":
		if e.ImageDefaultPrice <= 0 {
			return domain.NewValidationError("image_default_price", "image_default_price is required for image models")
		}
		if err := validateImagePriceTiers(e.ImagePrices); err != nil {
			return err
		}
	case "video":
		if e.VideoDefaultPrice <= 0 {
			return domain.NewValidationError("video_default_price", "video_default_price is required for video models")
		}
	}
	return validateResolutions("video_prices", e.VideoPrices)
}

func validatePrice(field string, v float64) error {
	if v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return domain.NewValidationError(field, "must be a non-negative number")
	}
	if v > maxPricePerToken {
		return domain.NewValidationError(field, "price exceeds maximum")
	}
	return nil
}

func validateResolutions(field string, rs []domain.ResolutionUSDPrice) error {
	seen := make(map[string]struct{}, len(rs))
	for _, r := range rs {
		res := strings.TrimSpace(r.Resolution)
		if res == "" {
			return domain.NewValidationError(field, "resolution is required")
		}
		if _, dup := seen[res]; dup {
			return domain.NewValidationError(field, "duplicate resolution "+res)
		}
		seen[res] = struct{}{}
		if err := validatePrice(field, r.Price); err != nil {
			return err
		}
	}
	return nil
}

func validateImagePriceTiers(prices []domain.ResolutionUSDPrice) error {
	allowed := map[string]struct{}{"1k": {}, "2k": {}, "4k": {}}
	seen := make(map[string]struct{}, len(prices))
	for _, price := range prices {
		tier := strings.TrimSpace(price.Resolution)
		if price.Resolution != tier {
			return domain.NewValidationError("image_prices", "image price tier must be an exact lowercase tier")
		}
		if _, ok := allowed[tier]; !ok {
			return domain.NewValidationError("image_prices", "image price tier must be one of 1k, 2k, 4k")
		}
		if _, duplicate := seen[tier]; duplicate {
			return domain.NewValidationError("image_prices", "duplicate resolution "+tier)
		}
		seen[tier] = struct{}{}
		if err := validatePrice("image_prices", price.Price); err != nil {
			return err
		}
	}
	return nil
}

func validateMultiplier(field string, v float64) error {
	if v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return domain.NewValidationError(field, "must be a non-negative number")
	}
	if v > maxMultiplier {
		return domain.NewValidationError(field, "multiplier exceeds maximum")
	}
	return nil
}

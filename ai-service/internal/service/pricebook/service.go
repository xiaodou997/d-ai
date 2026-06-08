package pricebook

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"xiaodou/unihub/ai-service/internal/domain"
)

// maxPricePerToken caps any single USD-per-token field. 1 USD/token is already
// absurd (≈ $1M per 1M tokens); this just guards against fat-finger/overflow.
const maxPricePerToken = 1.0

// maxMultiplier caps cost/sell/user multipliers.
const maxMultiplier = 10000.0

// Service implements price book + sell binding + global rate business logic.
type Service struct {
	repo    Repository
	fetcher LiteLLMFetcher

	// LiteLLM reference data is kept in memory (sub2api-style): never bulk
	// materialized into the DB. Admins search it and pull single models into a
	// price book on demand.
	llmMu    sync.Mutex
	llmCache map[string]LiteLLMModel
	llmExp   time.Time
}

// New constructs a Service. fetcher may be nil (LiteLLM import then errors).
func New(repo Repository, fetcher LiteLLMFetcher) *Service {
	return &Service{repo: repo, fetcher: fetcher}
}

// ============================================================================
// Price books
// ============================================================================

func (s *Service) CreatePriceBook(ctx context.Context, name, description string) (domain.PriceBook, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.PriceBook{}, domain.NewValidationError("name", "name is required")
	}
	return s.repo.CreatePriceBook(ctx, name, strings.TrimSpace(description))
}

func (s *Service) GetPriceBook(ctx context.Context, id string) (domain.PriceBook, error) {
	return s.repo.GetPriceBook(ctx, id)
}

func (s *Service) ListPriceBooks(ctx context.Context) ([]domain.PriceBook, error) {
	return s.repo.ListPriceBooks(ctx)
}

func (s *Service) UpdatePriceBook(ctx context.Context, id, name, description, status string) (domain.PriceBook, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.PriceBook{}, domain.NewValidationError("name", "name is required")
	}
	if status != "active" && status != "disabled" {
		return domain.PriceBook{}, domain.NewValidationError("status", "status must be active or disabled")
	}
	return s.repo.UpdatePriceBook(ctx, id, name, strings.TrimSpace(description), status)
}

func (s *Service) DeletePriceBook(ctx context.Context, id string) error {
	return s.repo.DeletePriceBook(ctx, id)
}

// ============================================================================
// Entries
// ============================================================================

func (s *Service) UpsertEntry(ctx context.Context, e domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	if err := validateEntry(e); err != nil {
		return domain.PriceBookEntry{}, err
	}
	return s.repo.UpsertEntry(ctx, e)
}

func (s *Service) GetEntry(ctx context.Context, priceBookID, modelCode string) (domain.PriceBookEntry, error) {
	return s.repo.GetEntry(ctx, priceBookID, modelCode)
}

func (s *Service) ListEntries(ctx context.Context, priceBookID string) ([]domain.PriceBookEntry, error) {
	return s.repo.ListEntries(ctx, priceBookID)
}

func (s *Service) DeleteEntry(ctx context.Context, priceBookID, modelCode string) error {
	return s.repo.DeleteEntry(ctx, priceBookID, modelCode)
}

// ============================================================================
// Global USD→credit rate
// ============================================================================

// GetCreditsPerUSD returns the configured rate, falling back to the default when
// the setting is missing or unparsable.
func (s *Service) GetCreditsPerUSD(ctx context.Context) (float64, error) {
	raw, err := s.repo.GetSetting(ctx, domain.SettingCreditsPerUSD)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.DefaultCreditsPerUSD, nil
	}
	if err != nil {
		return 0, err
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil || v <= 0 {
		return domain.DefaultCreditsPerUSD, nil
	}
	return v, nil
}

// SetCreditsPerUSD validates and persists the USD→credit rate.
func (s *Service) SetCreditsPerUSD(ctx context.Context, v float64) error {
	if v <= 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return domain.NewValidationError("credits_per_usd", "must be a positive number")
	}
	raw, _ := json.Marshal(v)
	return s.repo.UpsertSetting(ctx, domain.SettingCreditsPerUSD, raw)
}

// ============================================================================
// Sell bindings
// ============================================================================

func (s *Service) UpsertTenantSellBinding(ctx context.Context, b domain.TenantSellBinding) (domain.TenantSellBinding, error) {
	if strings.TrimSpace(b.TenantID) == "" {
		return domain.TenantSellBinding{}, domain.NewValidationError("tenant_id", "tenant_id is required")
	}
	if strings.TrimSpace(b.PriceBookID) == "" {
		return domain.TenantSellBinding{}, domain.NewValidationError("price_book_id", "price_book_id is required")
	}
	if err := validateMultiplier("sell_multiplier", b.SellMultiplier); err != nil {
		return domain.TenantSellBinding{}, err
	}
	return s.repo.UpsertTenantSellBinding(ctx, b)
}

func (s *Service) GetTenantSellBinding(ctx context.Context, tenantID string) (domain.TenantSellBinding, error) {
	return s.repo.GetTenantSellBinding(ctx, tenantID)
}

func (s *Service) ListTenantSellBindings(ctx context.Context) ([]domain.TenantSellBinding, error) {
	return s.repo.ListTenantSellBindings(ctx)
}

func (s *Service) DeleteTenantSellBinding(ctx context.Context, tenantID string) error {
	return s.repo.DeleteTenantSellBinding(ctx, tenantID)
}

func (s *Service) UpsertUserSellBinding(ctx context.Context, b domain.UserSellBinding) (domain.UserSellBinding, error) {
	if strings.TrimSpace(b.TenantID) == "" {
		return domain.UserSellBinding{}, domain.NewValidationError("tenant_id", "tenant_id is required")
	}
	if err := validateMultiplier("user_multiplier", b.UserMultiplier); err != nil {
		return domain.UserSellBinding{}, err
	}
	return s.repo.UpsertUserSellBinding(ctx, b)
}

func (s *Service) GetUserSellBinding(ctx context.Context, tenantID string) (domain.UserSellBinding, error) {
	return s.repo.GetUserSellBinding(ctx, tenantID)
}

func (s *Service) DeleteUserSellBinding(ctx context.Context, tenantID string) error {
	return s.repo.DeleteUserSellBinding(ctx, tenantID)
}

// ============================================================================
// Effective prices (read-only, scope-resolved credit unit prices for display)
// ============================================================================

// perMillion scales per-token prices to per-1M for display.
const perMillion = 1_000_000.0

// EffectivePrices returns the credit unit prices in effect for a tenant
// (includeUser=false → platform→tenant prices) or its end users
// (includeUser=true → cascaded tenant→user prices). Returns ErrNotFound when the
// tenant has no sell binding.
func (s *Service) EffectivePrices(ctx context.Context, tenantID string, includeUser bool) ([]domain.EffectiveModelPrice, error) {
	tb, err := s.repo.GetTenantSellBinding(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	rate, err := s.GetCreditsPerUSD(ctx)
	if err != nil {
		return nil, err
	}
	mult := tb.SellMultiplier
	cacheEnabled := tb.CacheBillingEnabled
	if includeUser {
		ub, err := s.repo.GetUserSellBinding(ctx, tenantID)
		if err == nil {
			mult *= ub.UserMultiplier
			cacheEnabled = ub.CacheBillingEnabled
		} else if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		// no user binding → user_multiplier defaults to 1, cache off
	}

	entries, err := s.repo.ListEntries(ctx, tb.PriceBookID)
	if err != nil {
		return nil, err
	}

	// per-1M credits = per_token × 1e6 × mult × rate
	factor := perMillion * mult * rate
	out := make([]domain.EffectiveModelPrice, 0, len(entries))
	for _, e := range entries {
		cacheWrite := e.InputPerToken
		cacheRead := e.InputPerToken
		reasoning := e.OutputPerToken
		if cacheEnabled {
			cacheWrite = e.EffectiveCacheWritePerToken()
			cacheRead = e.EffectiveCacheReadPerToken()
			reasoning = e.EffectiveReasoningPerToken()
		}
		out = append(out, domain.EffectiveModelPrice{
			ModelCode:                 e.ModelCode,
			CapabilityType:            e.CapabilityType,
			InputPer1MCredits:         e.InputPerToken * factor,
			OutputPer1MCredits:        e.OutputPerToken * factor,
			CacheWritePer1MCredits:    cacheWrite * factor,
			CacheReadPer1MCredits:     cacheRead * factor,
			ReasoningPer1MCredits:     reasoning * factor,
			ImagePrices:               scaleResolutions(e.ImagePrices, mult*rate),
			VideoPrices:               scaleResolutions(e.VideoPrices, mult*rate),
			AudioTTSPer1MCharsCredits: e.AudioTTSPerChar * factor,
			AudioSTTPerMinuteCredits:  e.AudioSTTPerMinute * mult * rate,
		})
	}
	return out, nil
}

func scaleResolutions(rs []domain.ResolutionUSDPrice, factor float64) []domain.ResolutionCreditPriceF {
	if len(rs) == 0 {
		return nil
	}
	out := make([]domain.ResolutionCreditPriceF, 0, len(rs))
	for _, r := range rs {
		out = append(out, domain.ResolutionCreditPriceF{Resolution: r.Resolution, Price: r.Price * factor})
	}
	return out
}

// ============================================================================
// Validation
// ============================================================================

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
		{"input_per_token", e.InputPerToken},
		{"output_per_token", e.OutputPerToken},
		{"cache_write_per_token", e.CacheWritePerToken},
		{"cache_read_per_token", e.CacheReadPerToken},
		{"reasoning_per_token", e.ReasoningPerToken},
		{"audio_tts_per_char", e.AudioTTSPerChar},
		{"audio_stt_per_minute", e.AudioSTTPerMinute},
	}
	for _, sc := range scalars {
		if err := validatePrice(sc.field, sc.value); err != nil {
			return err
		}
	}
	if err := validateResolutions("image_prices", e.ImagePrices); err != nil {
		return err
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
	for i, r := range rs {
		res := strings.TrimSpace(r.Resolution)
		if res == "" {
			return domain.NewValidationError(field, "resolution is required")
		}
		if _, dup := seen[res]; dup {
			return domain.NewValidationError(field, "duplicate resolution "+res)
		}
		seen[res] = struct{}{}
		if r.Price < 0 || math.IsInf(r.Price, 0) || math.IsNaN(r.Price) {
			return domain.NewValidationError(field, "price must be non-negative")
		}
		_ = i
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

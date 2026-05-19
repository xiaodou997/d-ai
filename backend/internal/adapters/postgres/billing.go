package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/serving"
)

// PriceResolver resolves the effective model price for a request using
// three-tier lookup: user override → tenant override → base model price.
type PriceResolver struct {
	q *dbgen.Queries
}

func NewPriceResolver(q *dbgen.Queries) *PriceResolver {
	return &PriceResolver{q: q}
}

func (r *PriceResolver) ResolvePricing(ctx context.Context, req *serving.Request) (domain.ModelPricing, error) {
	if req.APIKey == nil || req.Candidate == nil {
		return domain.ModelPricing{}, errors.New("billing: missing api key or route candidate")
	}
	tenantID := req.APIKey.TenantID
	userID := req.APIKey.UserID
	modelID := mustParseUUID(req.Candidate.ModelID)

	if userID != "" {
		row, err := r.q.GetTenantUserPriceForRuntime(ctx, dbgen.GetTenantUserPriceForRuntimeParams{
			TenantID: tenantID,
			ModelID:  modelID,
		})
		if err == nil {
			return buildModelPricing(row.InputPricePer1m, row.OutputPricePer1m,
				row.CacheWritePricePer1m, row.CacheReadPricePer1m, row.ReasoningPricePer1m,
				row.ImagePrices, row.VideoPrices), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return domain.ModelPricing{}, err
		}
	}

	tenantRow, err := r.q.GetTenantModelPriceOverrideForRuntime(ctx, dbgen.GetTenantModelPriceOverrideForRuntimeParams{
		TenantID: tenantID,
		ModelID:  modelID,
	})
	if err == nil {
		return buildModelPricing(tenantRow.InputPricePer1m, tenantRow.OutputPricePer1m,
			tenantRow.CacheWritePricePer1m, tenantRow.CacheReadPricePer1m, tenantRow.ReasoningPricePer1m,
			tenantRow.ImagePrices, tenantRow.VideoPrices), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.ModelPricing{}, err
	}

	baseRow, err := r.q.GetActiveModelPrice(ctx, modelID)
	if err != nil {
		return domain.ModelPricing{}, err
	}
	return buildModelPricing(baseRow.InputPricePer1m, baseRow.OutputPricePer1m,
		baseRow.CacheWritePricePer1m, baseRow.CacheReadPricePer1m, baseRow.ReasoningPricePer1m,
		baseRow.ImagePrices, baseRow.VideoPrices), nil
}

func buildModelPricing(input, output, cacheWrite, cacheRead, reasoning int64, imagePricesJSON, videoPricesJSON []byte) domain.ModelPricing {
	return domain.ModelPricing{
		InputPer1M:      input,
		OutputPer1M:     output,
		CacheWritePer1M: cacheWrite,
		CacheReadPer1M:  cacheRead,
		ReasoningPer1M:  reasoning,
		ImagePrices:     decodeResolutionCreditPrices(imagePricesJSON),
		VideoPrices:     decodeResolutionCreditPrices(videoPricesJSON),
	}
}

func decodeResolutionCreditPrices(raw []byte) []domain.ResolutionCreditPrice {
	if len(raw) == 0 {
		return nil
	}
	var prices []domain.ResolutionCreditPrice
	if err := json.Unmarshal(raw, &prices); err != nil {
		return nil
	}
	return prices
}

// CalculateBilling computes all cost tiers from usage and pricing.
// For token-based models, cache tokens are billed at input price to create margin.
// For image models, cost = price-per-image × count (resolved by resolution).
// For video models, cost = price-per-second × duration (resolved by resolution).
func CalculateBilling(usage domain.TokenUsage, pricing domain.ModelPricing) domain.BillingResult {
	if usage.ImageCount > 0 {
		cost := lookupCreditPrice(pricing.ImagePrices, usage.ImageResolution) * int64(usage.ImageCount)
		return domain.BillingResult{
			PlatformCost:     cost,
			UserCost:         cost,
			APIKeyQuotaCost:  cost,
			BillableUnits:    int64(usage.ImageCount),
			BillableUnitType: "image",
		}
	}

	if usage.VideoSeconds > 0 {
		pricePerSec := lookupCreditPrice(pricing.VideoPrices, usage.VideoResolution)
		cost := int64(usage.VideoSeconds * float64(pricePerSec))
		seconds := int64(usage.VideoSeconds)
		return domain.BillingResult{
			PlatformCost:     cost,
			UserCost:         cost,
			APIKeyQuotaCost:  cost,
			BillableUnits:    seconds,
			BillableUnitType: "second",
		}
	}

	const perM = int64(1_000_000)
	promptCost := int64(usage.PromptTokens) * pricing.InputPer1M / perM
	completionCost := int64(usage.CompletionTokens) * pricing.OutputPer1M / perM
	cacheWriteCost := int64(usage.CacheWriteTokens) * pricing.EffectiveCacheWritePrice() / perM
	cacheReadCost := int64(usage.CacheReadTokens) * pricing.EffectiveCacheReadPrice() / perM
	reasoningCost := int64(usage.ReasoningTokens) * pricing.EffectiveReasoningPrice() / perM

	total := promptCost + completionCost + cacheWriteCost + cacheReadCost + reasoningCost
	totalTokens := int64(usage.TotalTokens())

	// Integer division truncates sub-1 results to 0 for small token counts.
	// Apply a floor of 1 when there is real token usage and the model has non-zero pricing.
	if total == 0 && totalTokens > 0 && (pricing.InputPer1M > 0 || pricing.OutputPer1M > 0) {
		total = 1
	}

	return domain.BillingResult{
		PlatformCost:     total,
		UserCost:         total,
		APIKeyQuotaCost:  total,
		BillableUnits:    totalTokens,
		BillableUnitType: "token",
	}
}

// lookupCreditPrice returns the credit price matching resolution, or the lowest
// price in the list when no exact match is found.
func lookupCreditPrice(prices []domain.ResolutionCreditPrice, resolution string) int64 {
	if len(prices) == 0 {
		return 0
	}
	if resolution != "" {
		for _, p := range prices {
			if p.Resolution == resolution {
				return p.Price
			}
		}
	}
	min := prices[0].Price
	for _, p := range prices[1:] {
		if p.Price < min {
			min = p.Price
		}
	}
	return min
}

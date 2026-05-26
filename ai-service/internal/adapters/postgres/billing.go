package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	dbgen "xiaodou/uni-ai-api/internal/db/gen"
	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/serving"
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
	identity := req.RuntimeIdentity()
	if identity == nil || req.Candidate == nil {
		return domain.ModelPricing{}, errors.New("billing: missing runtime identity or route candidate")
	}
	tenantID := identity.TenantID
	userID := identity.UserID
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

// CalculateBilling computes all cost tiers from usage and pricing. All output
// fields are in micro-credits (1 credit = 10000 micro). Pricing inputs are
// micro-credits per 1M tokens (or per image / per second of video).
//
// Precision: with micro-credit units, a 300-token request at 1 credit/M tokens
// produces 300 × 10000 / 1_000_000 = 3 micro-credits (= 0.0003 credit), no
// longer rounded up to a full integer credit.
//
// For token-based models, only PromptTokens and CompletionTokens are billed
// (at input and output price respectively). Cache and reasoning tokens are
// NOT billed separately because they are subsets of PromptTokens and
// CompletionTokens per upstream API semantics (OpenAI, Anthropic, Gemini).
// For image models, cost = price-per-image × count (resolved by resolution).
// For video models, cost = price-per-second × duration (resolved by resolution).
func CalculateBilling(usage domain.TokenUsage, pricing domain.ModelPricing) domain.BillingResult {
	if usage.ImageCount > 0 {
		cost := lookupCreditPrice(pricing.ImagePrices, usage.ImageResolution) * int64(usage.ImageCount)
		return domain.BillingResult{
			PlatformCostMicro:    cost,
			UserCostMicro:        cost,
			APIKeyQuotaCostMicro: cost,
			BillableUnits:        int64(usage.ImageCount),
			BillableUnitType:     "image",
		}
	}

	if usage.VideoSeconds > 0 {
		pricePerSec := lookupCreditPrice(pricing.VideoPrices, usage.VideoResolution)
		cost := int64(usage.VideoSeconds * float64(pricePerSec))
		seconds := int64(usage.VideoSeconds)
		return domain.BillingResult{
			PlatformCostMicro:    cost,
			UserCostMicro:        cost,
			APIKeyQuotaCostMicro: cost,
			BillableUnits:        seconds,
			BillableUnitType:     "second",
		}
	}

	const perM = int64(1_000_000)
	// CacheWriteTokens/CacheReadTokens are subsets of PromptTokens;
	// ReasoningTokens is a subset of CompletionTokens.
	// We bill only PromptTokens at input price and CompletionTokens at
	// output price — no separate cache/reasoning charges.
	promptCost := int64(usage.PromptTokens) * pricing.InputPer1M / perM
	completionCost := int64(usage.CompletionTokens) * pricing.OutputPer1M / perM

	total := promptCost + completionCost
	totalTokens := int64(usage.TotalTokens())

	return domain.BillingResult{
		PlatformCostMicro:    total,
		UserCostMicro:        total,
		APIKeyQuotaCostMicro: total,
		BillableUnits:        totalTokens,
		BillableUnitType:     "token",
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

package postgres

import (
	"context"
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
			return priceRowToModelPricing(row.InputPricePer1m, row.OutputPricePer1m,
				row.CacheWritePricePer1m, row.CacheReadPricePer1m, row.ReasoningPricePer1m), nil
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
		return priceRowToModelPricing(tenantRow.InputPricePer1m, tenantRow.OutputPricePer1m,
			tenantRow.CacheWritePricePer1m, tenantRow.CacheReadPricePer1m, tenantRow.ReasoningPricePer1m), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.ModelPricing{}, err
	}

	baseRow, err := r.q.GetActiveModelPrice(ctx, modelID)
	if err != nil {
		return domain.ModelPricing{}, err
	}
	return priceRowToModelPricing(baseRow.InputPricePer1m, baseRow.OutputPricePer1m,
		baseRow.CacheWritePricePer1m, baseRow.CacheReadPricePer1m, baseRow.ReasoningPricePer1m), nil
}

func priceRowToModelPricing(input, output, cacheWrite, cacheRead, reasoning int64) domain.ModelPricing {
	return domain.ModelPricing{
		InputPer1M:      input,
		OutputPer1M:     output,
		CacheWritePer1M: cacheWrite,
		CacheReadPer1M:  cacheRead,
		ReasoningPer1M:  reasoning,
	}
}

// CalculateBilling computes all cost tiers from token usage and pricing.
// Cache tokens are billed at input price, which creates margin when the
// provider charges less for cache reads.
func CalculateBilling(usage domain.TokenUsage, pricing domain.ModelPricing) domain.BillingResult {
	const perM = int64(1_000_000)

	promptCost := int64(usage.PromptTokens) * pricing.InputPer1M / perM
	completionCost := int64(usage.CompletionTokens) * pricing.OutputPer1M / perM
	cacheWriteCost := int64(usage.CacheWriteTokens) * pricing.EffectiveCacheWritePrice() / perM
	cacheReadCost := int64(usage.CacheReadTokens) * pricing.EffectiveCacheReadPrice() / perM
	reasoningCost := int64(usage.ReasoningTokens) * pricing.EffectiveReasoningPrice() / perM

	total := promptCost + completionCost + cacheWriteCost + cacheReadCost + reasoningCost
	totalTokens := int64(usage.TotalTokens())

	return domain.BillingResult{
		PlatformCost:     total,
		UserCost:         total,
		APIKeyQuotaCost:  total,
		BillableUnits:    totalTokens,
		BillableUnitType: "token",
	}
}

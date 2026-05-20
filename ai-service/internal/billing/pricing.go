// Package billing computes upstream CNY cost from a deployment's Pricing
// config and observed usage. The result is the platform-side cost (not the
// customer-facing price) — used for profit accounting, not quota deduction.
package billing

import "xiaodou/uni-ai-api/internal/domain"

// Usage carries the observable units of one upstream call.
type Usage struct {
	// Token counts for chat / embedding / completion calls.
	PromptTokens     int64
	CompletionTokens int64
	CacheWriteTokens int64
	CacheReadTokens  int64
	ReasoningTokens  int64

	// Image generation: number of images at the given resolution.
	ImageCount      int64
	ImageResolution string

	// Video generation: seconds produced at the given resolution.
	VideoSeconds    float64
	VideoResolution string
}

// CostForUsage computes total CNY for a single upstream call. Returns 0
// when pricing is nil or when no relevant fields are populated.
func CostForUsage(pricing *domain.Pricing, capability domain.CapabilityType, u Usage) float64 {
	if pricing == nil {
		return 0
	}

	total := pricing.RequestCost

	switch capability {
	case domain.CapabilityImage:
		total += imageCost(pricing.ImagePrices, u)
	case domain.CapabilityVideo:
		total += videoCost(pricing.VideoPrices, u)
	default:
		total += tokenCost(pricing.Tiers, u)
	}

	return total
}

// tokenCost picks the tier whose UpTo bound covers PromptTokens (last tier with
// UpTo == nil is unbounded), then applies per-token charges.
func tokenCost(tiers []domain.PricingTier, u Usage) float64 {
	if len(tiers) == 0 {
		return 0
	}
	tier := selectTier(tiers, u.PromptTokens)
	return perMillion(u.PromptTokens, tier.InputPer1M) +
		perMillion(u.CompletionTokens, tier.OutputPer1M) +
		perMillion(u.CacheWriteTokens, tier.CacheWritePer1M) +
		perMillion(u.CacheReadTokens, tier.CacheReadPer1M) +
		perMillion(u.ReasoningTokens, tier.ReasoningPer1M)
}

func selectTier(tiers []domain.PricingTier, promptTokens int64) domain.PricingTier {
	for _, t := range tiers {
		if t.UpTo == nil {
			return t
		}
		if promptTokens <= *t.UpTo {
			return t
		}
	}
	return tiers[len(tiers)-1]
}

func perMillion(tokens int64, pricePer1M float64) float64 {
	if tokens <= 0 || pricePer1M <= 0 {
		return 0
	}
	return float64(tokens) * pricePer1M / 1_000_000
}

func imageCost(prices []domain.ResolutionPrice, u Usage) float64 {
	if u.ImageCount <= 0 || len(prices) == 0 {
		return 0
	}
	return lookupResolution(prices, u.ImageResolution) * float64(u.ImageCount)
}

func videoCost(prices []domain.ResolutionPrice, u Usage) float64 {
	if u.VideoSeconds <= 0 || len(prices) == 0 {
		return 0
	}
	return lookupResolution(prices, u.VideoResolution) * u.VideoSeconds
}

// lookupResolution returns the price matching resolution, or the lowest price
// in the list when no exact match is found (mirrors UI "from X" rendering).
func lookupResolution(prices []domain.ResolutionPrice, resolution string) float64 {
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

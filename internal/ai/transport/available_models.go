package transport

import (
	"context"
	"sort"

	"xiaodou/dai/internal/ai/domain"
)

type availableModelAccumulator struct {
	dto                    userAvailableModelDTO
	imagePrices            map[string]resolutionUSDRangeDTO
	videoPrices            map[string]resolutionUSDRangeDTO
	tokenPricesInitialized bool
}

func listAvailableModelsForScope(ctx context.Context, reader ModelCatalogReader, tenantID, userID string) ([]userAvailableModelDTO, error) {
	rows, err := reader.ListAvailableModelPrices(ctx, domain.ModelCatalogScope{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, err
	}

	items := make(map[string]*availableModelAccumulator, len(rows))
	for _, row := range rows {
		key := row.ModelCode + "::" + row.CapabilityType
		acc, ok := items[key]
		if !ok {
			acc = &availableModelAccumulator{
				dto: userAvailableModelDTO{
					ModelCode:            row.ModelCode,
					ModelName:            row.ModelCode,
					CapabilityType:       row.CapabilityType,
					ImageDefaultPriceMin: row.ImageDefaultPrice,
					ImageDefaultPriceMax: row.ImageDefaultPrice,
					VideoDefaultPriceMin: row.VideoDefaultPrice,
					VideoDefaultPriceMax: row.VideoDefaultPrice,
				},
				imagePrices: make(map[string]resolutionUSDRangeDTO),
				videoPrices: make(map[string]resolutionUSDRangeDTO),
			}
			items[key] = acc
		} else {
			acc.dto.ImageDefaultPriceMin = minFloat64(acc.dto.ImageDefaultPriceMin, row.ImageDefaultPrice)
			acc.dto.ImageDefaultPriceMax = maxFloat64(acc.dto.ImageDefaultPriceMax, row.ImageDefaultPrice)
			acc.dto.VideoDefaultPriceMin = minFloat64(acc.dto.VideoDefaultPriceMin, row.VideoDefaultPrice)
			acc.dto.VideoDefaultPriceMax = maxFloat64(acc.dto.VideoDefaultPriceMax, row.VideoDefaultPrice)
		}
		mergeTokenPriceRanges(acc, row.TokenPriceTiers)
		mergeResolutionRanges(acc.imagePrices, row.ImagePrices)
		mergeResolutionRanges(acc.videoPrices, row.VideoPrices)
	}

	out := make([]userAvailableModelDTO, 0, len(items))
	for _, acc := range items {
		acc.dto.ImagePrices = sortedResolutionRanges(acc.imagePrices)
		acc.dto.VideoPrices = sortedResolutionRanges(acc.videoPrices)
		out = append(out, acc.dto)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ModelCode != out[j].ModelCode {
			return out[i].ModelCode < out[j].ModelCode
		}
		return out[i].CapabilityType < out[j].CapabilityType
	})
	return out, nil
}

func mergeTokenPriceRanges(acc *availableModelAccumulator, tiers []domain.TokenPriceTier) {
	if len(tiers) > 1 {
		acc.dto.HasContextTiers = true
	}
	for _, tier := range tiers {
		input := tier.InputPerToken * pricebookPerMillion
		output := tier.OutputPerToken * pricebookPerMillion
		cacheWrite := tier.CacheWritePerToken * pricebookPerMillion
		cacheRead := tier.CacheReadPerToken * pricebookPerMillion
		if !acc.tokenPricesInitialized {
			acc.dto.InputPer1MUSDMin, acc.dto.InputPer1MUSDMax = input, input
			acc.dto.OutputPer1MUSDMin, acc.dto.OutputPer1MUSDMax = output, output
			acc.dto.CacheWritePer1MUSDMin, acc.dto.CacheWritePer1MUSDMax = cacheWrite, cacheWrite
			acc.dto.CacheReadPer1MUSDMin, acc.dto.CacheReadPer1MUSDMax = cacheRead, cacheRead
			acc.tokenPricesInitialized = true
			continue
		}
		acc.dto.InputPer1MUSDMin = minFloat64(acc.dto.InputPer1MUSDMin, input)
		acc.dto.InputPer1MUSDMax = maxFloat64(acc.dto.InputPer1MUSDMax, input)
		acc.dto.OutputPer1MUSDMin = minFloat64(acc.dto.OutputPer1MUSDMin, output)
		acc.dto.OutputPer1MUSDMax = maxFloat64(acc.dto.OutputPer1MUSDMax, output)
		acc.dto.CacheWritePer1MUSDMin = minFloat64(acc.dto.CacheWritePer1MUSDMin, cacheWrite)
		acc.dto.CacheWritePer1MUSDMax = maxFloat64(acc.dto.CacheWritePer1MUSDMax, cacheWrite)
		acc.dto.CacheReadPer1MUSDMin = minFloat64(acc.dto.CacheReadPer1MUSDMin, cacheRead)
		acc.dto.CacheReadPer1MUSDMax = maxFloat64(acc.dto.CacheReadPer1MUSDMax, cacheRead)
	}
}

func mergeResolutionRanges(target map[string]resolutionUSDRangeDTO, prices []domain.ResolutionUSDPrice) {
	for _, price := range prices {
		item, ok := target[price.Resolution]
		if !ok {
			target[price.Resolution] = resolutionUSDRangeDTO{
				Resolution:  price.Resolution,
				PriceUSDMin: price.Price,
				PriceUSDMax: price.Price,
			}
			continue
		}
		item.PriceUSDMin = minFloat64(item.PriceUSDMin, price.Price)
		item.PriceUSDMax = maxFloat64(item.PriceUSDMax, price.Price)
		target[price.Resolution] = item
	}
}

func sortedResolutionRanges(items map[string]resolutionUSDRangeDTO) []resolutionUSDRangeDTO {
	if len(items) == 0 {
		return nil
	}
	out := make([]resolutionUSDRangeDTO, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Resolution < out[j].Resolution
	})
	return out
}

func minFloat64(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func maxFloat64(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

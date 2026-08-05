package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/domain"
)

type availableModelRow struct {
	ModelCode         string
	CapabilityType    string
	TokenPriceTiers   []byte
	ImageDefaultPrice float64
	VideoDefaultPrice float64
	ImagePrices       []byte
	VideoPrices       []byte
}

type availableModelAccumulator struct {
	dto                    userAvailableModelDTO
	imagePrices            map[string]resolutionUSDRangeDTO
	videoPrices            map[string]resolutionUSDRangeDTO
	tokenPricesInitialized bool
}

func listAvailableModelsForScope(ctx context.Context, pool *pgxpool.Pool, tenantID, userID string) ([]userAvailableModelDTO, error) {
	if pool == nil {
		return nil, fmt.Errorf("postgres pool is not configured")
	}
	rows, err := queryAvailableModelRows(ctx, pool, tenantID, userID)
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
		if err := mergeTokenPriceRanges(acc, row.TokenPriceTiers); err != nil {
			return nil, fmt.Errorf("decode token price tiers for %s: %w", row.ModelCode, err)
		}

		if err := mergeResolutionRanges(acc.imagePrices, row.ImagePrices); err != nil {
			return nil, fmt.Errorf("decode image prices for %s: %w", row.ModelCode, err)
		}
		if err := mergeResolutionRanges(acc.videoPrices, row.VideoPrices); err != nil {
			return nil, fmt.Errorf("decode video prices for %s: %w", row.ModelCode, err)
		}
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

func queryAvailableModelRows(ctx context.Context, pool *pgxpool.Pool, tenantID, userID string) ([]availableModelRow, error) {
	query := `
		SELECT
		  um.model_code,
		  um.capability_type,
			  e.token_price_tiers,
			  e.image_default_price::float8,
			  e.video_default_price::float8,
			  e.image_prices,
			  e.video_prices
		FROM ai_groups g
		JOIN ai_group_targets gt
		  ON gt.group_id = g.id AND gt.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.status = 'active'
		JOIN ai_price_book_entries e
		  ON e.price_book_id = g.retail_price_book_id
		 AND e.model_code = um.model_code
		 AND e.capability_type = um.capability_type
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		JOIN ai_price_book_entries account_e
		  ON account_e.price_book_id = COALESCE(a.price_book_id, cp.price_book_id)
		 AND account_e.model_code = um.model_code
		 AND account_e.capability_type = um.capability_type
	`
	args := []any{tenantID}
	if userID != "" {
		query += `
		LEFT JOIN ai_user_groups ug
		  ON ug.group_id = g.id AND ug.tenant_id = $1 AND ug.user_id = $2
		WHERE g.tenant_id = $1
		  AND g.status = 'active'
		  AND (g.user_default_visible OR ug.id IS NOT NULL)
		`
		args = append(args, userID)
	} else {
		query += `
		WHERE g.tenant_id = $1
		  AND g.status = 'active'
		`
	}
	query += `
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		  AND (
		    COALESCE(a.tenant_access_mode, cp.tenant_access_mode) = 'public'
		    OR EXISTS (
		      SELECT 1 FROM ai_upstream_resource_tenant_policies rg
		      WHERE rg.resource_kind = gt.target_kind AND rg.resource_id = gt.target_id AND rg.tenant_id = $1
		        AND rg.access_granted
		    )
		  )
		ORDER BY um.model_code ASC, um.capability_type ASC
	`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]availableModelRow, 0)
	for rows.Next() {
		var row availableModelRow
		if err := rows.Scan(
			&row.ModelCode,
			&row.CapabilityType,
			&row.TokenPriceTiers,
			&row.ImageDefaultPrice,
			&row.VideoDefaultPrice,
			&row.ImagePrices,
			&row.VideoPrices,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func mergeTokenPriceRanges(acc *availableModelAccumulator, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var tiers []domain.TokenPriceTier
	if err := json.Unmarshal(raw, &tiers); err != nil {
		return err
	}
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
	return nil
}

func mergeResolutionRanges(target map[string]resolutionUSDRangeDTO, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var prices []domain.ResolutionUSDPrice
	if err := json.Unmarshal(raw, &prices); err != nil {
		return err
	}
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
	return nil
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

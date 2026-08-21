package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/domain"
)

type ModelCatalogReader struct {
	pool *pgxpool.Pool
}

func NewModelCatalogReader(pool *pgxpool.Pool) *ModelCatalogReader {
	return &ModelCatalogReader{pool: pool}
}

func (r *ModelCatalogReader) ListAvailableModelPrices(ctx context.Context, scope domain.ModelCatalogScope) ([]domain.RoutedModelPrice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			um.model_code,
			um.capability_type,
			e.token_price_tiers,
			e.image_default_price::float8,
			e.video_default_price::float8,
			e.image_prices,
			e.video_prices,
			e.audio_tts_per_char::float8,
			e.audio_stt_per_minute::float8
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
		LEFT JOIN ai_user_groups ug
			ON ug.group_id = g.id
			AND ug.tenant_id = $1
			AND ug.user_id = NULLIF($2, '')
		WHERE g.tenant_id = $1
			AND g.status = 'active'
			AND ($2 = '' OR g.user_default_visible OR ug.id IS NOT NULL)
			AND (
				(gt.target_kind = 'direct_upstream' AND a.status = 'active')
				OR (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
			)
			AND (
				COALESCE(a.tenant_access_mode, cp.tenant_access_mode) = 'public'
				OR EXISTS (
					SELECT 1
					FROM ai_upstream_resource_tenant_policies rg
					WHERE rg.resource_kind = gt.target_kind
						AND rg.resource_id = gt.target_id
						AND rg.tenant_id = $1
						AND rg.access_granted
				)
			)
		ORDER BY um.model_code ASC, um.capability_type ASC
	`, scope.TenantID, scope.UserID)
	if err != nil {
		return nil, fmt.Errorf("list available model prices: %w", err)
	}
	defer rows.Close()

	items := make([]domain.RoutedModelPrice, 0)
	for rows.Next() {
		item, err := scanRoutedModelPrice(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan available model price: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate available model prices: %w", err)
	}
	return items, nil
}

func (r *ModelCatalogReader) ListRoutedGroupPrices(ctx context.Context, groupID string) ([]domain.RoutedModelPrice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT
			um.model_code,
			um.capability_type,
			e.token_price_tiers,
			e.image_default_price::float8,
			e.video_default_price::float8,
			e.image_prices,
			e.video_prices,
			e.audio_tts_per_char::float8,
			e.audio_stt_per_minute::float8
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
		WHERE g.id = $1::uuid
			AND g.status = 'active'
			AND (
				(gt.target_kind = 'direct_upstream' AND a.status = 'active')
				OR (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
			)
		ORDER BY um.model_code ASC, um.capability_type ASC
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list routed group prices: %w", err)
	}
	defer rows.Close()

	items := make([]domain.RoutedModelPrice, 0)
	for rows.Next() {
		item, err := scanRoutedModelPrice(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan routed group price: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routed group prices: %w", err)
	}
	return items, nil
}

func (r *ModelCatalogReader) ListTenantUpstreamResources(ctx context.Context, tenantID string) ([]domain.TenantUpstreamResource, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			r.id::text,
			r.resource_kind,
			r.tenant_display_name,
			COALESCE(tp.tenant_multiplier_override, r.tenant_multiplier, 1)::float8,
			COALESCE(pb.id::text, ''),
			COALESCE(pb.name, ''),
			COALESCE(pb.revision, 0),
			COALESCE(um.model_code, ''),
			COALESCE(um.capability_type, ''),
			COALESCE(um.api_format, ''),
			e.id IS NOT NULL,
			COALESCE(e.token_price_tiers, '[]'::jsonb),
			COALESCE(e.image_default_price, 0)::float8,
			COALESCE(e.video_default_price, 0)::float8,
			COALESCE(e.image_prices, '[]'::jsonb),
			COALESCE(e.video_prices, '[]'::jsonb),
			COALESCE(e.audio_tts_per_char, 0)::float8,
			COALESCE(e.audio_stt_per_minute, 0)::float8,
			COALESCE(e.source, ''),
			COALESCE(e.manually_edited, false)
		FROM ai_upstream_resources r
		LEFT JOIN ai_upstream_resource_tenant_policies tp
			ON tp.resource_kind = r.resource_kind
			AND tp.resource_id = r.id
			AND tp.tenant_id = $1
		LEFT JOIN ai_price_books pb
			ON pb.id = r.price_book_id AND pb.status = 'active'
		LEFT JOIN ai_upstream_models um
			ON um.upstream_kind = r.resource_kind
			AND um.upstream_id = r.id
			AND um.status = 'active'
		LEFT JOIN ai_price_book_entries e
			ON e.price_book_id = pb.id
			AND e.model_code = um.model_code
			AND e.capability_type = um.capability_type
		WHERE r.status = 'active'
			AND (r.tenant_access_mode = 'public' OR COALESCE(tp.access_granted, false))
		ORDER BY r.tenant_display_name, r.resource_kind, um.model_code, um.capability_type
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list tenant upstream resources: %w", err)
	}
	defer rows.Close()

	items := make([]domain.TenantUpstreamResource, 0)
	byID := make(map[string]int)
	for rows.Next() {
		var resource domain.TenantUpstreamResource
		var model domain.TenantUpstreamModel
		var hasPrice bool
		var tokenTiersRaw, imagePricesRaw, videoPricesRaw []byte
		var price domain.PriceBookEntry
		if err := rows.Scan(
			&resource.ID, &resource.Kind, &resource.Name, &resource.TenantMultiplier,
			&resource.PriceBookID, &resource.PriceBookName, &resource.PriceBookRevision,
			&model.ModelCode, &model.CapabilityType, &model.APIFormat, &hasPrice,
			&tokenTiersRaw, &price.ImageDefaultPrice, &price.VideoDefaultPrice,
			&imagePricesRaw, &videoPricesRaw, &price.AudioTTSPerChar,
			&price.AudioSTTPerMinute, &price.Source, &price.ManuallyEdited,
		); err != nil {
			return nil, fmt.Errorf("scan tenant upstream resource: %w", err)
		}
		resourceKey := string(resource.Kind) + ":" + resource.ID
		index, ok := byID[resourceKey]
		if !ok {
			resource.Models = []domain.TenantUpstreamModel{}
			items = append(items, resource)
			index = len(items) - 1
			byID[resourceKey] = index
		}
		if model.ModelCode == "" {
			continue
		}
		if hasPrice {
			price.PriceBookID = resource.PriceBookID
			price.ModelCode = model.ModelCode
			price.CapabilityType = model.CapabilityType
			if err := decodeCatalogPriceJSON(&price, tokenTiersRaw, imagePricesRaw, videoPricesRaw); err != nil {
				return nil, fmt.Errorf("decode tenant upstream model price for %s: %w", model.ModelCode, err)
			}
			model.Price = &price
		}
		items[index].Models = append(items[index].Models, model)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenant upstream resources: %w", err)
	}
	return items, nil
}

func scanRoutedModelPrice(scan func(dest ...any) error) (domain.RoutedModelPrice, error) {
	var item domain.RoutedModelPrice
	var tokenTiersRaw, imagePricesRaw, videoPricesRaw []byte
	if err := scan(
		&item.ModelCode, &item.CapabilityType, &tokenTiersRaw,
		&item.ImageDefaultPrice, &item.VideoDefaultPrice,
		&imagePricesRaw, &videoPricesRaw,
		&item.AudioTTSPerChar, &item.AudioSTTPerMinute,
	); err != nil {
		return domain.RoutedModelPrice{}, err
	}
	price := domain.PriceBookEntry{}
	if err := decodeCatalogPriceJSON(&price, tokenTiersRaw, imagePricesRaw, videoPricesRaw); err != nil {
		return domain.RoutedModelPrice{}, err
	}
	item.TokenPriceTiers = price.TokenPriceTiers
	item.ImagePrices = price.ImagePrices
	item.VideoPrices = price.VideoPrices
	return item, nil
}

func decodeCatalogPriceJSON(price *domain.PriceBookEntry, tokenTiersRaw, imagePricesRaw, videoPricesRaw []byte) error {
	if err := json.Unmarshal(tokenTiersRaw, &price.TokenPriceTiers); err != nil {
		return fmt.Errorf("decode token price tiers: %w", err)
	}
	if err := json.Unmarshal(imagePricesRaw, &price.ImagePrices); err != nil {
		return fmt.Errorf("decode image prices: %w", err)
	}
	if err := json.Unmarshal(videoPricesRaw, &price.VideoPrices); err != nil {
		return fmt.Errorf("decode video prices: %w", err)
	}
	return nil
}

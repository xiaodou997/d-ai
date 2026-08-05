package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

type tenantUpstreamModelDTO struct {
	ModelCode      string             `json:"model_code"`
	CapabilityType string             `json:"capability_type"`
	APIFormat      string             `json:"api_format"`
	Availability   string             `json:"availability" enum:"available,no_price_configured"`
	Price          *priceBookEntryDTO `json:"price,omitempty"`
}

type tenantUpstreamResourceDTO struct {
	ID                string                   `json:"id"`
	ResourceKind      string                   `json:"resource_kind" enum:"direct_upstream,oauth_pool"`
	Name              string                   `json:"name"`
	TenantMultiplier  float64                  `json:"tenant_multiplier"`
	PriceBookID       string                   `json:"price_book_id,omitempty"`
	PriceBookName     string                   `json:"price_book_name,omitempty"`
	PriceBookRevision int64                    `json:"price_book_revision,omitempty"`
	Models            []tenantUpstreamModelDTO `json:"models"`
}

type tenantUpstreamResourcesOutput struct {
	Body struct {
		Items []tenantUpstreamResourceDTO `json:"items"`
		Total int                         `json:"total"`
	}
}

func registerTenantUpstreamCatalog(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-upstream-resources",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/upstream-resources",
		Summary:     "租户可选上游资源",
		Description: "返回平台开放的上游账号、结算倍率、公共模型与价格；不会返回地址、密钥、请求头、OAuth 凭据或内部模型名。",
		Tags:        []string{"upstream-resources"},
	}, func(ctx context.Context, _ *struct{}) (*tenantUpstreamResourcesOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres dependency is not configured")
		}
		tenantID := strings.TrimSpace(tenantIDFromContext(ctx))
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		items, err := loadTenantUpstreamResources(ctx, d, tenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &tenantUpstreamResourcesOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		return out, nil
	})
}

func loadTenantUpstreamResources(ctx context.Context, d AIDeps, tenantID string) ([]tenantUpstreamResourceDTO, error) {
	rows, err := d.Postgres.Query(ctx, `
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
		LEFT JOIN ai_price_books pb ON pb.id = r.price_book_id AND pb.status = 'active'
		LEFT JOIN ai_upstream_models um
		  ON um.upstream_kind = r.resource_kind AND um.upstream_id = r.id AND um.status = 'active'
		LEFT JOIN ai_price_book_entries e
		  ON e.price_book_id = pb.id
		 AND e.model_code = um.model_code
		 AND e.capability_type = um.capability_type
		WHERE r.status = 'active'
		  AND (
		    r.tenant_access_mode = 'public'
		    OR COALESCE(tp.access_granted, false)
		  )
		ORDER BY r.tenant_display_name, r.resource_kind, um.model_code, um.capability_type
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]tenantUpstreamResourceDTO, 0)
	byID := make(map[string]int)
	for rows.Next() {
		var resource tenantUpstreamResourceDTO
		var model tenantUpstreamModelDTO
		var hasPrice bool
		var tokenTiersRaw, imagePricesRaw, videoPricesRaw []byte
		var imageDefault, videoDefault, audioTTS, audioSTT float64
		var source string
		var manuallyEdited bool
		if err := rows.Scan(
			&resource.ID, &resource.ResourceKind, &resource.Name, &resource.TenantMultiplier,
			&resource.PriceBookID, &resource.PriceBookName, &resource.PriceBookRevision,
			&model.ModelCode, &model.CapabilityType, &model.APIFormat, &hasPrice,
			&tokenTiersRaw, &imageDefault, &videoDefault, &imagePricesRaw, &videoPricesRaw,
			&audioTTS, &audioSTT, &source, &manuallyEdited,
		); err != nil {
			return nil, err
		}
		resourceKey := resource.ResourceKind + ":" + resource.ID
		index, ok := byID[resourceKey]
		if !ok {
			resource.Models = []tenantUpstreamModelDTO{}
			items = append(items, resource)
			index = len(items) - 1
			byID[resourceKey] = index
		}
		if model.ModelCode == "" {
			continue
		}
		model.Availability = "no_price_configured"
		if hasPrice {
			entry := domain.PriceBookEntry{
				ModelCode: model.ModelCode, CapabilityType: model.CapabilityType,
				ImageDefaultPrice: imageDefault, VideoDefaultPrice: videoDefault,
				AudioTTSPerChar: audioTTS, AudioSTTPerMinute: audioSTT,
				Source: source, ManuallyEdited: manuallyEdited,
			}
			if err := json.Unmarshal(tokenTiersRaw, &entry.TokenPriceTiers); err != nil {
				return nil, err
			}
			if err := json.Unmarshal(imagePricesRaw, &entry.ImagePrices); err != nil {
				return nil, err
			}
			if err := json.Unmarshal(videoPricesRaw, &entry.VideoPrices); err != nil {
				return nil, err
			}
			price := priceBookEntryToDTO(entry)
			model.Price = &price
			model.Availability = "available"
		}
		items[index].Models = append(items[index].Models, model)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

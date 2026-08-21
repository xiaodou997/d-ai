package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/libs/go/httpx"
)

// registerTenantSelfPricing exposes tenant-scoped model and retail price
// helpers. The price multiplier is the group's user multiplier only; upstream
// account multipliers belong to the independent tenant-charge ledger.
func registerTenantSelfPricing(api huma.API, d TenantCatalogHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-available-models",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/available-models",
		Summary:     "租户可用模型列表",
		Tags:        []string{"model-grants"},
	}, func(ctx context.Context, _ *struct{}) (*userAvailableModelsOutput, error) {
		if d.ModelCatalog == nil {
			return nil, httpx.ErrUnavailable.WithDetail("model catalog reader is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		items, err := listAvailableModelsForScope(ctx, d.ModelCatalog, tenantID, "")
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &userAvailableModelsOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-group-effective-prices",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/groups/{groupID}/effective-prices",
		Summary:     "分组默认用户价格",
		Description: "返回零售价格表 USD 单价乘分组默认用户倍率后的 USD 价格。",
		Tags:        []string{"groups"},
	}, func(ctx context.Context, in *tenantSelfGroupIDInput) (*tenantGroupEffectivePricesOutput, error) {
		if d.Groups == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		group, err := d.Groups.GetGroup(ctx, commercial.TenantGroupScope{TenantID: tenantID, GroupID: in.GroupID})
		if err != nil {
			return nil, mapServiceError(err)
		}
		entries, err := listRoutedGroupPriceEntries(ctx, d.ModelCatalog, group.ID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		factor := group.DefaultUserMultiplier
		out := &tenantGroupEffectivePricesOutput{}
		out.Body.GroupID = group.ID
		out.Body.RetailPriceBookID = group.RetailPriceBookID
		out.Body.EffectiveUserMultiplier = group.DefaultUserMultiplier
		out.Body.Items = make([]tenantGroupEffectivePriceDTO, 0, len(entries))
		for _, entry := range entries {
			out.Body.Items = append(out.Body.Items, tenantGroupEffectivePriceDTO{
				ModelCode: entry.ModelCode, CapabilityType: entry.CapabilityType,
				TokenPriceTiers:       effectiveTokenPriceTiers(entry.TokenPriceTiers, factor),
				ImageDefaultPriceUSD:  entry.ImageDefaultPrice * factor,
				VideoDefaultPriceUSD:  entry.VideoDefaultPrice * factor,
				ImagePrices:           resolutionUSDPrices(entry.ImagePrices, factor),
				VideoPrices:           resolutionUSDPrices(entry.VideoPrices, factor),
				AudioTTSPer1MCharsUSD: entry.AudioTTSPerChar * pricebookPerMillion * factor,
				AudioSTTPerMinuteUSD:  entry.AudioSTTPerMinute * factor,
			})
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

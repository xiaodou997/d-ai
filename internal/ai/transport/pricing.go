package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/libs/go/httpx"
)

// 售价绑定已迁移到分组体系（groups.go：租户/用户分组绑定 + 分组对外价格表/倍率）。
// 本文件仅保留全局 USD→积分 汇率管理。

type creditsPerUSDOutput struct {
	Body struct {
		CreditsPerUSD float64 `json:"credits_per_usd" doc:"1 USD 对应积分数"`
	}
}

type updateCreditsPerUSDInput struct {
	Body updateCreditsPerUSDRequest
}

type updateCreditsPerUSDRequest struct {
	CreditsPerUSD float64 `json:"credits_per_usd" minimum:"0.0001" maximum:"1000000" doc:"1 USD 对应积分数"`
}

func registerPricingRead(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-get-credits-per-usd",
		Method:      http.MethodGet,
		Path:        "/api/v1/pricing/credits-per-usd",
		Summary:     "USD 到积分汇率",
		Tags:        []string{"pricing"},
	}, func(ctx context.Context, _ *struct{}) (*creditsPerUSDOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		value, err := d.PriceBookSvc.GetCreditsPerUSD(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &creditsPerUSDOutput{}
		out.Body.CreditsPerUSD = value
		return out, nil
	})
}

func registerPricingWrite(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-set-credits-per-usd",
		Method:      http.MethodPut,
		Path:        "/api/v1/pricing/credits-per-usd",
		Summary:     "设置 USD 到积分汇率",
		Tags:        []string{"pricing"},
	}, func(ctx context.Context, in *updateCreditsPerUSDInput) (*creditsPerUSDOutput, error) {
		if d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("price book service is not configured")
		}
		if err := d.PriceBookSvc.SetCreditsPerUSD(ctx, in.Body.CreditsPerUSD); err != nil {
			return nil, mapServiceError(err)
		}
		out := &creditsPerUSDOutput{}
		out.Body.CreditsPerUSD = in.Body.CreditsPerUSD
		return out, nil
	})
}

// registerTenantSelfPricing exposes tenant-scoped model and retail price
// helpers. The price multiplier is the group's user multiplier only; upstream
// account multipliers belong to the independent tenant-charge ledger.
func registerTenantSelfPricing(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-available-models",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/available-models",
		Summary:     "租户可用模型列表",
		Tags:        []string{"model-grants"},
	}, func(ctx context.Context, _ *struct{}) (*userAvailableModelsOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres dependency is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		items, err := listAvailableModelsForScope(ctx, d.Postgres, tenantID, "")
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
		Description: "返回零售价格表 USD 单价乘分组默认用户倍率和积分汇率后的价格。",
		Tags:        []string{"groups"},
	}, func(ctx context.Context, in *tenantSelfGroupIDInput) (*tenantGroupEffectivePricesOutput, error) {
		if d.CommercialSvc == nil || d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial/pricebook service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		group, err := d.CommercialSvc.GetGroup(ctx, commercial.TenantGroupScope{TenantID: tenantID, GroupID: in.GroupID})
		if err != nil {
			return nil, mapServiceError(err)
		}
		rate, err := d.PriceBookSvc.GetCreditsPerUSD(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		entries, err := listRoutedGroupPriceEntries(ctx, d, group.ID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		factor := group.DefaultUserMultiplier * rate
		out := &tenantGroupEffectivePricesOutput{}
		out.Body.GroupID = group.ID
		out.Body.RetailPriceBookID = group.RetailPriceBookID
		out.Body.EffectiveUserMultiplier = group.DefaultUserMultiplier
		out.Body.CreditsPerUSD = rate
		out.Body.Items = make([]tenantGroupEffectivePriceDTO, 0, len(entries))
		for _, entry := range entries {
			out.Body.Items = append(out.Body.Items, tenantGroupEffectivePriceDTO{
				ModelCode: entry.ModelCode, CapabilityType: entry.CapabilityType,
				TokenPriceTiers:           effectiveTokenPriceTiers(entry.TokenPriceTiers, factor),
				ImageDefaultPriceCredits:  entry.ImageDefaultPrice * factor,
				VideoDefaultPriceCredits:  entry.VideoDefaultPrice * factor,
				ImagePrices:               resolutionCreditPrices(entry.ImagePrices, factor),
				VideoPrices:               resolutionCreditPrices(entry.VideoPrices, factor),
				AudioTTSPer1MCharsCredits: entry.AudioTTSPerChar * pricebookPerMillion * factor,
				AudioSTTPerMinuteCredits:  entry.AudioSTTPerMinute * factor,
			})
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

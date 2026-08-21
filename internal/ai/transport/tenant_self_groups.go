package transport

// Shared DTOs and pricing helpers for tenant-owned groups.

import (
	"context"
	"encoding/json"
	"sort"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

type userVisibleGroupDTO struct {
	ID                      string  `json:"id" doc:"分组 ID"`
	Name                    string  `json:"name" doc:"分组名称"`
	Description             string  `json:"description" doc:"分组描述"`
	EffectiveUserMultiplier float64 `json:"effective_user_multiplier" doc:"终端用户最终零售倍率（用户覆盖优先于分组默认）"`
	Status                  string  `json:"status" doc:"分组状态"`
}

type userVisibleGroupsOutput struct {
	Body struct {
		Items []userVisibleGroupDTO `json:"items"`
		Total int                   `json:"total"`
	}
}

type tenantSelfGroupIDInput struct {
	GroupID string `path:"groupID" doc:"分组 ID"`
}

type tenantGroupEffectivePriceDTO struct {
	ModelCode             string                        `json:"model_code" doc:"模型编码"`
	CapabilityType        string                        `json:"capability_type" doc:"能力类型"`
	TokenPriceTiers       []effectiveTokenPriceTierDTO  `json:"token_price_tiers" doc:"逐上下文档位的生效 USD 价格"`
	ImageDefaultPriceUSD  float64                       `json:"image_default_price_usd" doc:"图片默认生效售价（USD/张）"`
	VideoDefaultPriceUSD  float64                       `json:"video_default_price_usd" doc:"视频默认生效售价（USD/秒）"`
	ImagePrices           []tenantResolutionUSDPriceDTO `json:"image_prices,omitempty" doc:"图片尺寸档位覆盖生效售价（USD/张）"`
	VideoPrices           []tenantResolutionUSDPriceDTO `json:"video_prices,omitempty" doc:"视频规格覆盖生效售价（USD/秒）"`
	AudioTTSPer1MCharsUSD float64                       `json:"audio_tts_per_1m_chars_usd" doc:"语音合成每 100 万字符的生效售价（USD）"`
	AudioSTTPerMinuteUSD  float64                       `json:"audio_stt_per_minute_usd" doc:"语音识别每分钟的生效售价（USD）"`
}

type effectiveTokenPriceTierDTO struct {
	UpToInputTokens    *int    `json:"up_to_input_tokens"`
	InputPer1MUSD      float64 `json:"input_per_1m_usd"`
	OutputPer1MUSD     float64 `json:"output_per_1m_usd"`
	CacheWritePer1MUSD float64 `json:"cache_write_per_1m_usd"`
	CacheReadPer1MUSD  float64 `json:"cache_read_per_1m_usd"`
}

type tenantResolutionUSDPriceDTO struct {
	Resolution string  `json:"resolution" doc:"分辨率/规格"`
	Price      float64 `json:"price" doc:"该规格的生效售价（USD）"`
}

type tenantGroupEffectivePricesOutput struct {
	Body struct {
		GroupID                 string                         `json:"group_id"`
		RetailPriceBookID       string                         `json:"retail_price_book_id"`
		EffectiveUserMultiplier float64                        `json:"effective_user_multiplier"`
		Items                   []tenantGroupEffectivePriceDTO `json:"items"`
		Total                   int                            `json:"total"`
	}
}

func listRoutedGroupPriceEntries(ctx context.Context, reader ModelCatalogReader, groupID string) ([]domain.RoutedModelPrice, error) {
	if reader == nil {
		return nil, httpx.ErrUnavailable.WithDetail("model catalog reader is not configured")
	}
	return reader.ListRoutedGroupPrices(ctx, groupID)
}

func userVisibleGroupToDTO(item commercial.AccessibleGroup) userVisibleGroupDTO {
	return userVisibleGroupDTO{
		ID:                      item.Group.ID,
		Name:                    item.Group.Name,
		Description:             item.Group.Description,
		EffectiveUserMultiplier: item.EffectiveUserMultiplier,
		Status:                  string(item.Group.Status),
	}
}

func effectiveTokenPriceTiers(tiers []domain.TokenPriceTier, factor float64) []effectiveTokenPriceTierDTO {
	out := make([]effectiveTokenPriceTierDTO, 0, len(tiers))
	for _, tier := range tiers {
		out = append(out, effectiveTokenPriceTierDTO{
			UpToInputTokens:    tier.UpToInputTokens,
			InputPer1MUSD:      tier.InputPerToken * pricebookPerMillion * factor,
			OutputPer1MUSD:     tier.OutputPerToken * pricebookPerMillion * factor,
			CacheWritePer1MUSD: tier.CacheWritePerToken * pricebookPerMillion * factor,
			CacheReadPer1MUSD:  tier.CacheReadPerToken * pricebookPerMillion * factor,
		})
	}
	return out
}

func resolutionUSDPrices(prices []domain.ResolutionUSDPrice, factor float64) []tenantResolutionUSDPriceDTO {
	if len(prices) == 0 {
		return nil
	}
	out := make([]tenantResolutionUSDPriceDTO, 0, len(prices))
	for _, price := range prices {
		out = append(out, tenantResolutionUSDPriceDTO{
			Resolution: price.Resolution,
			Price:      price.Price * factor,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Resolution < out[j].Resolution
	})
	return out
}

func decodeUSDResolutionsInto(raw []byte, target *[]domain.ResolutionUSDPrice) error {
	if len(raw) == 0 {
		*target = nil
		return nil
	}
	var prices []domain.ResolutionUSDPrice
	if err := json.Unmarshal(raw, &prices); err != nil {
		return err
	}
	*target = prices
	return nil
}

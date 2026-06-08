package domain

import "time"

// ============================================================================
// Price Book 统一定价（详见 docs/PRICING_REFACTOR_PLAN.md）
//
// 一套 USD 价格表，上游成本与对外售价共用同一目录；各处引用时设单倍率。
// 单价单位：USD per token（与 LiteLLM 原生 input_cost_per_token 一致）；
// 音频为 USD/字符、USD/分钟；图片/视频档位为 USD（每图 / 每秒）。
//
// 这些字段用 float64 承载：USD 单价精度远高于最终 int64 微积分粒度
// （1 微积分 ≈ 0.0001 积分），所有计费在写入微积分时统一 floor，误差可忽略。
// 计费引擎（PriceBookBiller）直接消费这些 USD 价并在写入微积分时 floor。
// ============================================================================

// SettingCreditsPerUSD 是 ai_settings 中 USD→积分汇率的 key。
const SettingCreditsPerUSD = "credits_per_usd"

// DefaultCreditsPerUSD 是汇率缺省值（1 USD = 7 积分），与 init.sql 种子一致。
const DefaultCreditsPerUSD = 7.0

// PriceBook 是一张命名的 USD 价格表（如「标准价」「中转便宜价」）。
type PriceBook struct {
	ID          string
	Name        string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ResolutionUSDPrice 是按分辨率的 USD 价（图片每张 / 视频每秒）。
type ResolutionUSDPrice struct {
	Resolution string  `json:"resolution"`
	Price      float64 `json:"price"`
}

// PriceBookEntry 是价格表里某个模型的一行 USD 定价。
// Cache/Reasoning 为 0 表示「按 input/output 计」，由 Effective* 方法解析。
type PriceBookEntry struct {
	ID                 string
	PriceBookID        string
	ModelCode          string
	CapabilityType     string
	InputPerToken      float64
	OutputPerToken     float64
	CacheWritePerToken float64 // 0 → 按 InputPerToken
	CacheReadPerToken  float64 // 0 → 按 InputPerToken
	ReasoningPerToken  float64 // 0 → 按 OutputPerToken
	ImagePrices        []ResolutionUSDPrice
	VideoPrices        []ResolutionUSDPrice
	AudioTTSPerChar    float64
	AudioSTTPerMinute  float64
	Source             string // "manual" | "litellm"
	ManuallyEdited     bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// EffectiveCacheWritePerToken 返回缓存写入单价；0 时回退到 input。
func (e PriceBookEntry) EffectiveCacheWritePerToken() float64 {
	if e.CacheWritePerToken > 0 {
		return e.CacheWritePerToken
	}
	return e.InputPerToken
}

// EffectiveCacheReadPerToken 返回缓存读取单价；0 时回退到 input。
func (e PriceBookEntry) EffectiveCacheReadPerToken() float64 {
	if e.CacheReadPerToken > 0 {
		return e.CacheReadPerToken
	}
	return e.InputPerToken
}

// EffectiveReasoningPerToken 返回推理单价；0 时回退到 output。
func (e PriceBookEntry) EffectiveReasoningPerToken() float64 {
	if e.ReasoningPerToken > 0 {
		return e.ReasoningPerToken
	}
	return e.OutputPerToken
}

// TenantSellBinding 是平台→租户售价绑定（一租户一倍率，覆盖该价格表所有模型）。
// 租户售价(积分/token) = entry × SellMultiplier × credits_per_usd。
type TenantSellBinding struct {
	ID                  string
	TenantID            string
	PriceBookID         string
	SellMultiplier      float64
	CacheBillingEnabled bool
	CreatedAt           time.Time
	UpdatedAt           time.Time

	// PriceBookName 仅在列表查询（join ai_price_books）时填充。
	PriceBookName string
}

// UserSellBinding 是租户→用户售价绑定（级联：基准=平台给该租户的售价，再×UserMultiplier）。
// 租户不另选价格表，跟随平台为其选定的价格表。
type UserSellBinding struct {
	ID                  string
	TenantID            string
	UserMultiplier      float64
	CacheBillingEnabled bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ResolutionCreditPriceF is a per-resolution price in (fractional) credits, used
// for read-only effective-price display (image per image / video per second).
type ResolutionCreditPriceF struct {
	Resolution string  `json:"resolution"`
	Price      float64 `json:"price"`
}

// EffectiveModelPrice is a read-only, scope-resolved per-model price in credits,
// for tenant/user self-service display. Token/char fields are per 1M.
type EffectiveModelPrice struct {
	ModelCode                 string
	CapabilityType            string
	InputPer1MCredits         float64
	OutputPer1MCredits        float64
	CacheWritePer1MCredits    float64
	CacheReadPer1MCredits     float64
	ReasoningPer1MCredits     float64
	ImagePrices               []ResolutionCreditPriceF
	VideoPrices               []ResolutionCreditPriceF
	AudioTTSPer1MCharsCredits float64
	AudioSTTPerMinuteCredits  float64
}

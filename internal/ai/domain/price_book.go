package domain

import (
	"time"

	corebilling "xiaodou/dai/internal/ai/core/billing"
)

// ============================================================================
// Price Book 统一定价（详见 docs/PRICING_REFACTOR_PLAN.md）
//
// 一套 USD 价格表，上游成本与对外售价共用同一目录；各处引用时设单倍率。
// 单价单位：USD per token（与 LiteLLM 原生 input_cost_per_token 一致）；
// 音频为 USD/字符、USD/分钟；图片/视频档位为 USD（每图 / 每秒）。
//
// 控制面暂以 float64 承载这些字段；计费引擎会先通过十进制文本恢复为精确
// 有理数，最终每个计费侧只做一次 round-half-up 到 int64 微积分。浮点值只用于
// DTO 兼容和展示，不直接参与最终取整。
// ============================================================================

// SettingCreditsPerUSD 是 ai_settings 中 USD→积分汇率的 key。
const SettingCreditsPerUSD = "credits_per_usd"

// DefaultCreditsPerUSD 是汇率缺省值（1 USD = 100 积分），与 init.sql 种子一致。
// 仅当读取 ai_settings.credits_per_usd 失败时作为兜底；正常运行以 DB 值为准。
const DefaultCreditsPerUSD = 100.0

type PriceBookOwnerType string

const (
	PriceBookOwnerPlatform PriceBookOwnerType = "platform"
	PriceBookOwnerTenant   PriceBookOwnerType = "tenant"
)

// PriceBook 是一张平台公共或租户私有的 USD 价格表。
type PriceBook struct {
	ID            string
	OwnerType     PriceBookOwnerType
	OwnerTenantID string
	Name          string
	Description   string
	Status        string
	Revision      int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ResolutionUSDPrice 是按分辨率的 USD 价（图片每张 / 视频每秒）。
type ResolutionUSDPrice struct {
	Resolution string  `json:"resolution"`
	Price      float64 `json:"price"`
}

// TokenPriceTier is shared with the rebuilt billing core so validation and
// selection rules cannot diverge between legacy runtime and vNext code.
type TokenPriceTier = corebilling.TokenPriceTier

// PriceBookEntry 是价格表里某个模型的一行 USD 定价。
type PriceBookEntry struct {
	ID                string
	PriceBookID       string
	ModelCode         string
	CapabilityType    string
	TokenPriceTiers   []TokenPriceTier
	ImageDefaultPrice float64
	VideoDefaultPrice float64
	ImagePrices       []ResolutionUSDPrice
	VideoPrices       []ResolutionUSDPrice
	AudioTTSPerChar   float64
	AudioSTTPerMinute float64
	Source            string // "manual" | "litellm"
	ManuallyEdited    bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TenantSellBinding 是平台→租户售价绑定（一租户一倍率，覆盖该价格表所有模型）。
// 租户售价(积分/token) = entry × SellMultiplier × credits_per_usd。
type TenantSellBinding struct {
	ID             string
	TenantID       string
	PriceBookID    string
	SellMultiplier float64
	CreatedAt      time.Time
	UpdatedAt      time.Time

	// PriceBookName 仅在列表查询（join ai_price_books）时填充。
	PriceBookName string
}

// UserSellBinding 是租户→用户售价绑定（级联：基准=平台给该租户的售价，再×UserMultiplierOverride）。
// 租户不另选价格表，跟随平台为其选定的价格表。
type UserSellBinding struct {
	ID                     string
	TenantID               string
	UserMultiplierOverride float64
	CreatedAt              time.Time
	UpdatedAt              time.Time
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
	TokenPriceTiers           []EffectiveTokenPriceTier
	ImageDefaultPriceCredits  float64
	VideoDefaultPriceCredits  float64
	ImagePrices               []ResolutionCreditPriceF
	VideoPrices               []ResolutionCreditPriceF
	AudioTTSPer1MCharsCredits float64
	AudioSTTPerMinuteCredits  float64
}

type EffectiveTokenPriceTier struct {
	UpToInputTokens        *int    `json:"up_to_input_tokens"`
	InputPer1MCredits      float64 `json:"input_per_1m_credits"`
	OutputPer1MCredits     float64 `json:"output_per_1m_credits"`
	CacheWritePer1MCredits float64 `json:"cache_write_per_1m_credits"`
	CacheReadPer1MCredits  float64 `json:"cache_read_per_1m_credits"`
}

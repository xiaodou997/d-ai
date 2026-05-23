// Package domain contains pure business types with no external dependencies.
package domain

import "time"

// CapabilityType represents what an AI model can do.
type CapabilityType string

const (
	CapabilityChat      CapabilityType = "chat"
	CapabilityImage     CapabilityType = "image"
	CapabilityVideo     CapabilityType = "video"
	CapabilityEmbedding CapabilityType = "embedding"
	CapabilityAudioTTS  CapabilityType = "audio_tts"
	CapabilityAudioSTT  CapabilityType = "audio_stt"
	CapabilityRerank    CapabilityType = "rerank"
)

// EndpointProtocol identifies the vendor API style of an endpoint, used for
// model discovery (GET /v1/models) and default protocol assignment.
type EndpointProtocol string

const (
	EndpointProtocolOpenAICompatible EndpointProtocol = "openai_compatible"
	EndpointProtocolAnthropic        EndpointProtocol = "anthropic"
	EndpointProtocolGemini           EndpointProtocol = "gemini"
)

// UpstreamProtocol identifies which wire protocol to use when calling an upstream.
type UpstreamProtocol string

const (
	ProtocolOpenAIChat        UpstreamProtocol = "openai_chat"
	ProtocolOpenAIResponses   UpstreamProtocol = "openai_responses"
	ProtocolOpenAICompletions UpstreamProtocol = "openai_completions"
	ProtocolOpenAIEmbeddings  UpstreamProtocol = "openai_embeddings"
	ProtocolOpenAIImages      UpstreamProtocol = "openai_images"
	ProtocolAnthropicMessages UpstreamProtocol = "anthropic_messages"
	ProtocolGeminiGenerate    UpstreamProtocol = "gemini_generate"
	ProtocolGeminiEmbeddings  UpstreamProtocol = "gemini_embeddings"
)

// HealthStatus tracks upstream deployment health.
type HealthStatus string

const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// OwnerType describes who owns an API key.
type OwnerType string

const (
	OwnerTenant OwnerType = "tenant"
	OwnerUser   OwnerType = "user"
)

// BillingStatus describes the state of a usage log's billing.
type BillingStatus string

const (
	BillingPending   BillingStatus = "pending"
	BillingFrozen    BillingStatus = "frozen"
	BillingConfirmed BillingStatus = "confirmed"
	BillingCancelled BillingStatus = "cancelled"
	BillingFree      BillingStatus = "free"
	// Phase 2 (分账层) 新状态：
	// PendingSettle = 已写入本地账本聚合表 pending_micro，等待结算聚合
	// Settled       = 已被一次聚合 Consume 调用扣款，settled_event_id 已回填
	BillingPendingSettle BillingStatus = "pending_settle"
	BillingSettled       BillingStatus = "settled"
)

// RequestStatus describes the outcome of an AI request.
type RequestStatus string

const (
	RequestSuccess RequestStatus = "success"
	RequestFailed  RequestStatus = "failed"
)

// TaskStatus describes the lifecycle of an async task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

// ============================================================================
// Auth context — populated from API key or JWT
// ============================================================================

type RuntimeAuthMethod string

const (
	AuthMethodAPIKey RuntimeAuthMethod = "api_key"
	AuthMethodJWT    RuntimeAuthMethod = "jwt"
)

type RequestSource string

const (
	RequestSourceAPIKey  RequestSource = "api_key"
	RequestSourceWebChat RequestSource = "web_chat"
)

// APIKeyAuth holds everything resolved from an API key DB lookup.
type APIKeyAuth struct {
	KeyID         string
	OwnerType     OwnerType
	TenantID      string
	UserID        string // empty when OwnerType == OwnerTenant
	AllowedModels []string
	QuotaLimit    *int64
	QuotaUsed     int64
	QuotaReserved int64
}

// QuotaAvailable returns the remaining quota, or -1 if unlimited.
func (a APIKeyAuth) QuotaAvailable() int64 {
	if a.QuotaLimit == nil {
		return -1
	}
	return *a.QuotaLimit - a.QuotaUsed - a.QuotaReserved
}

// RuntimeIdentity is the normalized caller identity for every runtime request.
// API-key calls populate APIKeyID, AllowedModels, and quota fields. JWT web
// calls leave those fields empty and are governed by tenant/user authorization.
type RuntimeIdentity struct {
	AuthMethod    RuntimeAuthMethod
	RequestSource RequestSource
	OwnerType     OwnerType
	TenantID      string
	UserID        string
	APIKeyID      string
	AllowedModels []string
	QuotaLimit    *int64
	QuotaUsed     int64
	QuotaReserved int64
}

func IdentityFromAPIKey(key APIKeyAuth) *RuntimeIdentity {
	return &RuntimeIdentity{
		AuthMethod:    AuthMethodAPIKey,
		RequestSource: RequestSourceAPIKey,
		OwnerType:     key.OwnerType,
		TenantID:      key.TenantID,
		UserID:        key.UserID,
		APIKeyID:      key.KeyID,
		AllowedModels: key.AllowedModels,
		QuotaLimit:    key.QuotaLimit,
		QuotaUsed:     key.QuotaUsed,
		QuotaReserved: key.QuotaReserved,
	}
}

func (i RuntimeIdentity) QuotaAvailable() int64 {
	if i.QuotaLimit == nil {
		return -1
	}
	return *i.QuotaLimit - i.QuotaUsed - i.QuotaReserved
}

func (i RuntimeIdentity) UsesAPIKeyQuota() bool {
	return i.AuthMethod == AuthMethodAPIKey && i.APIKeyID != ""
}

// StickyKey returns a stable per-caller key used for sticky-routing bindings
// and upstream session derivation. API-key calls key on the key ID; JWT web
// calls key on request source + owner + tenant + user so that distinct web
// callers never share a binding. Both tenant and user IDs are always included
// to keep the key unambiguous.
func (i RuntimeIdentity) StickyKey() string {
	if i.APIKeyID != "" {
		return i.APIKeyID
	}
	return string(i.RequestSource) + ":" + string(i.OwnerType) + ":" + i.TenantID + ":" + i.UserID
}

// JWTClaims holds parsed claims from a URM-issued JWT.
type JWTClaims struct {
	UserID   string
	Username string
	UserType int // 1=platform, 2=admin, other=tenant user
	TenantID string
	AppKey   string
}

func (c JWTClaims) IsPlatformAdmin() bool { return c.UserType == 1 || c.UserType == 2 }

// ============================================================================
// Model resolution
// ============================================================================

// Model is the public-facing AI model definition.
type Model struct {
	ID                     string
	ModelCode              string
	CapabilityType         CapabilityType
	ContextWindow          *int
	DefaultMaxOutputTokens int
	MaxOutputTokens        *int
}

// RouteCandidate represents one resolved upstream route for a model.
// It carries everything needed to execute an AI request against an upstream.
//
// Two mutually exclusive target types:
//   - Deployment-based (API Key): DeploymentID + EndpointID + APIKeyCiphertext are set.
//   - Pool-based (OAuth Fixed Provider): PoolID + PoolUpstreamModel are set; BaseURL and
//     Protocol are derived from FixedProviderBaseURL / FixedProviderProtocol at selection time.
type RouteCandidate struct {
	RouteID        string
	Priority       int
	Weight         int
	SupportsStream bool
	ModelID        string // ai_models.id — used for price lookup
	CapabilityType CapabilityType

	// Deployment-based route fields (API Key type)
	DeploymentID       string
	UpstreamModel      string
	Protocol           UpstreamProtocol
	RequestPath        string
	UpstreamParameters map[string]any
	HealthStatus       HealthStatus
	EndpointID         string
	BaseURL            string
	APIKeyCiphertext   string // decrypted; empty for pool routes
	ExtraHeaders       map[string]string
	Timeouts           RouteTimeouts // 三段式超时，已按 route>model>global 解析完毕
	ProviderCode       string

	// Pool-based route fields (OAuth Fixed Provider)
	PoolID            string            // ai_credential_pools.id
	PoolUpstreamModel string            // model name to send to the upstream
	FixedProviderType FixedProviderType // "codex" | "claude_oauth" | "gemini_cli" | "antigravity"
	OAuthStrategy     string            // "round_robin" | "weighted"

	// P3: scoring hints loaded from ai_model_routes
	CostPer1kTokens      float64            // 0 for free/pool routes → scorer treats as very cheap
	ScoreWeightsOverride map[string]float64 // nil = use global weights

	// Upstream cost price (decoded from ai_upstream_deployments.pricing JSONB).
	// nil for pool routes or when deployment has no pricing configured.
	Pricing *Pricing
}

// ============================================================================
// Upstream cost pricing
// ============================================================================

// Pricing carries everything needed to compute the upstream cost of one call.
// All numeric values are CNY in原值 (no scaling). Token prices are per 1,000,000
// tokens; image/video prices are absolute per-unit charges. Stored in JSONB so
// decimals (e.g. ¥0.525/M) survive round-trip without precision loss.
type Pricing struct {
	Tiers       []PricingTier     `json:"tiers,omitempty"`
	RequestCost float64           `json:"request_cost,omitempty"`
	ImagePrices []ResolutionPrice `json:"image_prices,omitempty"`
	VideoPrices []ResolutionPrice `json:"video_prices,omitempty"`
}

// PricingTier describes one band of token-volume-based pricing. The tier is
// selected by prompt_tokens (input). The last tier must use UpTo == nil to
// mean "no upper bound".
type PricingTier struct {
	UpTo            *int64  `json:"up_to"`
	InputPer1M      float64 `json:"input_per_1m"`
	OutputPer1M     float64 `json:"output_per_1m"`
	CacheWritePer1M float64 `json:"cache_write_per_1m,omitempty"`
	CacheReadPer1M  float64 `json:"cache_read_per_1m,omitempty"`
	ReasoningPer1M  float64 `json:"reasoning_per_1m,omitempty"`
}

// ResolutionPrice expresses a per-resolution image or per-resolution-per-second
// video price. For images, Price is per generated image. For videos, Price is
// per second at that resolution.
type ResolutionPrice struct {
	Resolution string  `json:"resolution"`
	Price      float64 `json:"price"`
}

// IsPoolRoute returns true when this route targets a CredentialPool (OAuth Fixed Provider).
func (r *RouteCandidate) IsPoolRoute() bool { return r.PoolID != "" }

// RouteTimeouts is the resolved 三段式 timeout budget for one route, already
// flattened through the route > model > global config chain. All four values
// are concrete (no zero/nil "inherit" sentinels) by the time a RouteCandidate
// carries them.
type RouteTimeouts struct {
	Connect     time.Duration // 发出请求 → 收到响应头
	FirstByte   time.Duration // 响应头 → 首个 body 字节
	Idle        time.Duration // 流式相邻 chunk 间隔
	MaxDuration time.Duration // 单次响应总时长上限
}

// ============================================================================
// OAuth credentials
// ============================================================================

// FixedProviderType identifies built-in providers with hardcoded base URLs.
type FixedProviderType string

const (
	FixedProviderCodex       FixedProviderType = "codex"
	FixedProviderClaudeOAuth FixedProviderType = "claude_oauth"
	FixedProviderGeminiCLI   FixedProviderType = "gemini_cli"
	FixedProviderAntigravity FixedProviderType = "antigravity"
)

// FixedProviderBaseURL returns the hardcoded upstream base URL.
func FixedProviderBaseURL(t FixedProviderType) string {
	switch t {
	case FixedProviderCodex:
		return "https://chatgpt.com/backend-api/codex"
	case FixedProviderClaudeOAuth:
		return "https://api.anthropic.com"
	case FixedProviderGeminiCLI, FixedProviderAntigravity:
		return "https://cloudcode-pa.googleapis.com"
	default:
		return ""
	}
}

// FixedProviderProtocol returns the upstream protocol for a fixed provider.
func FixedProviderProtocol(t FixedProviderType) UpstreamProtocol {
	switch t {
	case FixedProviderCodex:
		return ProtocolOpenAIResponses
	case FixedProviderClaudeOAuth:
		return ProtocolAnthropicMessages
	case FixedProviderGeminiCLI, FixedProviderAntigravity:
		return ProtocolGeminiGenerate
	default:
		return ProtocolOpenAIChat
	}
}

// OAuthCredentialStatus tracks whether an OAuth credential is usable.
type OAuthCredentialStatus string

const (
	OAuthCredentialActive   OAuthCredentialStatus = "active"
	OAuthCredentialInvalid  OAuthCredentialStatus = "invalid"
	OAuthCredentialDisabled OAuthCredentialStatus = "disabled"
)

// CredentialPool represents an OAuth account pool for a fixed provider.
type CredentialPool struct {
	ID                string
	Name              string
	FixedProviderType FixedProviderType
	OAuthStrategy     string
	Notes             string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// OAuthCredential represents a single OAuth token entry in a credential pool.
// AccessToken and RefreshToken are decrypted values ready for use.
type OAuthCredential struct {
	ID           string
	PoolID       string
	Name         string
	ProviderType FixedProviderType
	Email        string
	AccessToken  string // decrypted
	RefreshToken string // decrypted, may be empty
	TokenType    string
	ExpiresAt    *time.Time
	AuthMetadata map[string]any // raw metadata from import JSON
	Weight       int
	Status       OAuthCredentialStatus
}

// AccountID extracts the account ID from AuthMetadata (Codex-specific).
func (c *OAuthCredential) AccountID() string {
	if c.AuthMetadata == nil {
		return ""
	}
	v, _ := c.AuthMetadata["account_id"].(string)
	return v
}

// ============================================================================
// Billing
// ============================================================================

// TokenUsage records actual usage from a completed upstream response.
// For chat/embedding, only the token fields are populated.
// For image, ImageCount + ImageResolution carry the billing inputs.
// For video, VideoSeconds + VideoResolution carry the billing inputs.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	CacheWriteTokens int // tokens written to provider cache (billed at input price)
	CacheReadTokens  int // tokens read from provider cache (billed at input price, margin opportunity)
	ReasoningTokens  int // extended thinking / reasoning tokens

	// Image generation billing fields (populated from request, not upstream response).
	ImageCount      int    // number of images generated
	ImageResolution string // e.g. "1024x1024"

	// Video generation billing fields (populated from request, not upstream response).
	VideoSeconds    float64 // duration of video generated
	VideoResolution string  // e.g. "1920x1080"
}

func (u TokenUsage) TotalTokens() int {
	return u.PromptTokens + u.CompletionTokens +
		u.CacheWriteTokens + u.CacheReadTokens + u.ReasoningTokens
}

// MicroCreditsPerCredit is the precision unit for internal billing math.
// 1 积分 (credit, == 1 分人民币) = 10000 micro-credits.
//
// All in-memory price and amount fields in this package use micro-credit units
// to avoid the "300 tokens of a cheap model rounds to 0 (or 1) credit" loss
// that integer credit math suffered from. Conversion to integer credits happens
// only at the URM boundary (floor for actual deduction, ceil for pre-auth holds)
// and at the public DTO boundary (micro/10000.0 → float credits for display).
const MicroCreditsPerCredit int64 = 10000

// MicroToCreditsFloor truncates a micro-credit amount to whole credits, dropping
// any fractional remainder. Used when actually deducting from URM.
func MicroToCreditsFloor(micro int64) int64 {
	return micro / MicroCreditsPerCredit
}

// MicroToCreditsCeil rounds a micro-credit amount up to whole credits. Used
// when pre-authorizing (freezing) credits to ensure we hold enough.
func MicroToCreditsCeil(micro int64) int64 {
	if micro <= 0 {
		return 0
	}
	return (micro + MicroCreditsPerCredit - 1) / MicroCreditsPerCredit
}

// MicroToCreditsFloat returns the micro amount as fractional credits, suitable
// for human-facing display (e.g. "0.03 积分").
func MicroToCreditsFloat(micro int64) float64 {
	return float64(micro) / float64(MicroCreditsPerCredit)
}

// CreditsToMicro converts integer credits back into micro-credits.
func CreditsToMicro(credits int64) int64 {
	return credits * MicroCreditsPerCredit
}

// ResolutionCreditPrice is a per-resolution price in micro-credits.
// Used for image (per image) and video (per second) sales pricing.
type ResolutionCreditPrice struct {
	Resolution string `json:"resolution"`
	Price      int64  `json:"price"` // micro-credits
}

// ModelPricing holds per-1M-token prices in micro-credits.
// Zero values mean "use the corresponding base price" — see Effective* methods.
type ModelPricing struct {
	InputPer1M      int64 // micro-credits per 1M input tokens
	OutputPer1M     int64 // micro-credits per 1M output tokens
	CacheWritePer1M int64 // 0 = bill at InputPer1M
	CacheReadPer1M  int64 // 0 = bill at InputPer1M (profit: provider charges less)
	ReasoningPer1M  int64 // 0 = bill at OutputPer1M
	ImagePrices     []ResolutionCreditPrice
	VideoPrices     []ResolutionCreditPrice
}

func (p ModelPricing) EffectiveCacheWritePrice() int64 {
	if p.CacheWritePer1M > 0 {
		return p.CacheWritePer1M
	}
	return p.InputPer1M
}

func (p ModelPricing) EffectiveCacheReadPrice() int64 {
	if p.CacheReadPer1M > 0 {
		return p.CacheReadPer1M
	}
	return p.InputPer1M
}

func (p ModelPricing) EffectiveReasoningPrice() int64 {
	if p.ReasoningPer1M > 0 {
		return p.ReasoningPer1M
	}
	return p.OutputPer1M
}

// BillingResult is the output of cost calculation. *CostMicro 字段统一使用
// micro-credit 精度（1 积分 = 10000 micro-credit）。settle worker 在调 URM
// Consume 前 floor 到整数积分；DTO 层折成 _credits float 展示。
type BillingResult struct {
	ProviderCostMicro     int64 // 平台付给上游
	PlatformCostMicro     int64 // 租户付给平台（→ URM TenantAmount，floor 整数积分）
	UserCostMicro         int64 // 用户付给租户（→ URM UserAmount；tenant-owned key 为 0）
	APIKeyQuotaCostMicro  int64 // 扣减 API key 本地配额计数
	BillableUnits         int64
	BillableUnitType      string
}

// ============================================================================
// Rate limiting
// ============================================================================

// LimitPolicy is a resolved rate limit rule for a scope.
type LimitPolicy struct {
	RPMLimit         *int
	TPMLimit         *int
	ConcurrencyLimit *int
}

// ============================================================================
// Async task (Phase 2)
// ============================================================================

// AsyncTask tracks a long-running AI job (video generation, batch inference).
type AsyncTask struct {
	ID            string
	TaskType      string
	TenantID      string
	UserID        string
	APIKeyID      string
	ModelCode     string
	Status        TaskStatus
	EstimatedCost int64
	ActualCost    int64
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
	ExpiresAt     *time.Time
}

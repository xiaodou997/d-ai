// Package domain contains pure business types with no external dependencies.
package domain

import (
	"time"

	"xiaodou/dai/internal/money"
)

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
	BillingFree    BillingStatus = "free"
	BillingPending BillingStatus = "pending"
	BillingSettled BillingStatus = "settled"
	BillingFailed  BillingStatus = "failed"
)

// RequestStatus describes the outcome of an AI request.
type RequestStatus string

const (
	RequestSuccess RequestStatus = "success"
	RequestFailed  RequestStatus = "failed"
)

type ServiceTier string

const (
	ServiceTierStandard ServiceTier = "standard"
	ServiceTierFast     ServiceTier = "fast"
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
	RequestSourceAPIKey   RequestSource = "api_key"
	RequestSourceWebChat  RequestSource = "web_chat"
	RequestSourceWebImage RequestSource = "web_image"
)

const (
	ImageStreamModeAuto        = "auto"
	ImageStreamModeForceStream = "force_stream"
	ImageStreamModeForceSync   = "force_sync"

	ImageResponseFormatURL = "url"
	ImageResponseFormatB64 = "b64_json"

	ImageEditTransportJSON      = "application/json"
	ImageEditTransportMultipart = "multipart/form-data"

	DefaultImageOutputCount = 1
	MaxImageOutputCount     = 10
)

// JWTClaims holds parsed claims from a D-AI JWT.
type JWTClaims struct {
	UserID   string
	Username string
	UserType int // 1=platform, 2=admin, other=tenant user
	TenantID string
}

func (c JWTClaims) IsPlatformAdmin() bool { return c.UserType == 1 || c.UserType == 2 }

// ============================================================================
// Model resolution
// ============================================================================

// Model is the public-facing AI model definition. 当前对外模型目录由分组可访问
// 的 active 上游显式模型绑定推导；价格表只负责售价与计费校验。
// Model 仅保留 code+capability，不再承载上游元数据。
type Model struct {
	ModelCode      string
	CapabilityType CapabilityType
}

// RouteCandidate represents one resolved upstream target for a model (账号级路由).
// It carries everything needed to execute an AI request against an upstream.
//
// Two mutually exclusive target types (由 ai_group_targets 关联):
//   - Account-based (API Key): EndpointID + APIKeyCiphertext + BaseURL + Protocol are set.
//   - Pool-based (OAuth Fixed Provider): PoolID + PoolUpstreamModel are set; BaseURL and
//     Protocol are derived from FixedProviderBaseURL / FixedProviderProtocol at selection time.
//
// RouteID = ai_group_targets.id（绑定行，用于日志/统计/sticky 校验）。
// 目标身份（健康/sticky）：账号取 EndpointID，池取 PoolID。
type RouteCandidate struct {
	RouteID                      string
	GroupRank                    int
	Priority                     int
	SupportsStream               bool
	ModelCode                    string // 映射后的逻辑模型，用于售价价格表查找
	CapabilityType               CapabilityType
	RequestedModel               string
	MatchedDispatchRuleID        string
	MatchedDispatchRuleSummary   string
	ResolvedProviderFamily       string
	GroupAllowProtocolConversion bool

	// Account-based route fields (API Key type)
	UpstreamModel               string // 显式上游模型绑定中的真实模型名
	Protocol                    UpstreamProtocol
	RequestPath                 string
	UpstreamParameters          map[string]any
	ImageStreamMode             string
	ImageEditTransport          string
	ImageUpstreamResponseFormat string
	ImageMaxOutputCount         int
	ImageEditMaxOutputCount     int
	HealthStatus                HealthStatus
	EndpointID                  string // ai_upstream_accounts.id（账号目标身份）
	BaseURL                     string
	APIKeyCiphertext            string // decrypted; empty for pool routes
	ExtraHeaders                map[string]string
	Timeouts                    RouteTimeouts // 系统统一的响应头、首字节、空闲及总时长限制
	ProviderCode                string
	UpstreamConcurrencyLimit    *int // direct account requests allowed in flight at once; nil = unlimited

	// Pool-based route fields (OAuth Fixed Provider)
	PoolID            string            // ai_credential_pools.id
	PoolUpstreamModel string            // model name to send to the upstream
	FixedProviderType FixedProviderType // "codex" | "claude_oauth" | "gemini_cli" | "antigravity"
	OAuthStrategy     string            // "round_robin" | "weighted"

	// ConversionBucket 是协议转换偏好桶（0 同格式零转换 > 1 同子类型 > 2 同家族 >
	// 3 跨家族），由绑定解析按 client↔provider 落差计算。pickCandidate 在相同分组、
	// 相同目标 priority 内按桶分层，实现「主备优先级不被转换偏好打乱，同层零转换优先」。
	ConversionBucket int

	// 评分提示：CostPer1kTokens 供 scorer 计算成本分项（1/cost；0 = 免费路由，
	// scorer 以 costCapFree 兜底）。来自售价表 first-tier input+output × 1000 × tenant_multiplier。
	CostPer1kTokens float64 // 0 for free/pool routes → scorer treats as very cheap

	// 租户结算绑定。按对外 ModelCode + CapabilityType 查价，不暴露或依赖上游真实模型名。
	AccountPriceBookID string
	TenantMultiplier   float64

	// 用户零售绑定。用户覆盖倍率直接替换分组默认倍率。
	GroupID                    string
	GroupName                  string
	RetailPriceBookID          string
	GroupDefaultUserMultiplier float64
}

// IsPoolRoute returns true when this route targets a CredentialPool (OAuth Fixed Provider).
func (r *RouteCandidate) IsPoolRoute() bool { return r.PoolID != "" }

func (r *RouteCandidate) UpstreamTargetType() string {
	if r == nil {
		return ""
	}
	if r.IsPoolRoute() {
		return "pool"
	}
	if r.EndpointID != "" {
		return "account"
	}
	return ""
}

func (r *RouteCandidate) EffectiveUpstreamModel() string {
	if r == nil {
		return ""
	}
	if r.IsPoolRoute() {
		return r.PoolUpstreamModel
	}
	return r.UpstreamModel
}

// RouteTimeouts is the system-wide timeout budget for one upstream attempt.
type RouteTimeouts struct {
	ResponseHeader time.Duration // 发出请求 → 收到响应头
	FirstByte      time.Duration // 响应头 → 首个 body 字节
	Idle           time.Duration // 流式或生图响应的相邻 chunk 间隔
	MaxDuration    time.Duration // 单次响应总时长上限
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

// ProtocolFamilyMembers expands a provider protocol family into the concrete
// request protocols used by protocol-routing tests and conversion decisions.
// Account and pool model reachability still comes from explicit bindings.
func ProtocolFamilyMembers(family string) []UpstreamProtocol {
	switch EndpointProtocol(family) {
	case EndpointProtocolAnthropic:
		return []UpstreamProtocol{ProtocolAnthropicMessages}
	case EndpointProtocolGemini:
		return []UpstreamProtocol{ProtocolGeminiGenerate, ProtocolGeminiEmbeddings}
	default:
		return []UpstreamProtocol{
			ProtocolOpenAIChat,
			ProtocolOpenAIResponses,
			ProtocolOpenAIEmbeddings,
			ProtocolOpenAIImages,
		}
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
// 池亦为一等路由目标（经 ai_group_targets 被分组直连），与上游账号共享结算绑定。
type CredentialPool struct {
	ID                string
	Name              string
	TenantDisplayName string
	TenantAccessMode  string
	FixedProviderType FixedProviderType
	OAuthStrategy     string
	Notes             string
	Status            string
	PriceBookID       string
	TenantMultiplier  float64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// OAuthCredential represents a single OAuth token entry in a credential pool.
// AccessToken and RefreshToken are decrypted values ready for use.
type OAuthCredential struct {
	ID            string
	PoolID        string
	Name          string
	ProviderType  FixedProviderType
	Email         string
	AccessToken   string // decrypted
	RefreshToken  string // decrypted, may be empty
	TokenType     string
	ExpiresAt     *time.Time
	TokenVersion  int64
	AuthMetadata  map[string]any // raw metadata from import JSON
	Weight        int
	Status        OAuthCredentialStatus
	CooldownUntil *time.Time
}

// OAuthCredentialSummary is the non-secret read model exposed to management
// and transport layers. It deliberately omits access/refresh token
// ciphertexts while retaining the health and audit fields used by the pool
// credential list endpoint.
type OAuthCredentialSummary struct {
	ID                   string
	PoolID               string
	Name                 string
	ProviderType         string
	Email                string
	TokenType            string
	Scope                string
	ExpiresAt            *time.Time
	AuthMetadata         map[string]any
	Weight               int
	Status               string
	InvalidReason        string
	CooldownUntil        *time.Time
	LastUsedAt           *time.Time
	LastRefreshedAt      *time.Time
	LastFailedAt         *time.Time
	ConsecutiveFailCount int
	SuccessCount         int64
	FailCount            int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
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
	CacheWriteTokens int // subset of PromptTokens: tokens written to provider cache
	CacheReadTokens  int // subset of PromptTokens: tokens read from provider cache
	ReasoningTokens  int // subset of CompletionTokens: extended thinking / reasoning tokens

	// Image generation billing fields (populated from request, not upstream response).
	ImageCount      int    // number of images generated
	ImageResolution string // e.g. "1024x1024"

	// Video generation billing fields (populated from request, not upstream response).
	VideoSeconds    float64 // duration of video generated
	VideoResolution string  // e.g. "1920x1080"
}

const (
	// TokenUsageSourceUpstream means every billable token counter came from the
	// upstream provider's reported usage.
	TokenUsageSourceUpstream = "upstream"
	// TokenUsageSourceMixed means at least one reported token counter was
	// preserved and at least one missing counter was conservatively estimated.
	TokenUsageSourceMixed = "mixed"
	// TokenUsageSourceEstimated means no usable upstream token counters were
	// available, so all billable token counters are estimates.
	TokenUsageSourceEstimated = "estimated"
)

// TotalTokens returns the total number of billable tokens.
// Note: CacheWriteTokens/CacheReadTokens are subsets of PromptTokens,
// and ReasoningTokens is a subset of CompletionTokens (per upstream API
// semantics — OpenAI, Anthropic, Gemini all follow this convention).
// Therefore TotalTokens = PromptTokens + CompletionTokens to avoid
// double-counting.
func (u TokenUsage) TotalTokens() int {
	return u.PromptTokens + u.CompletionTokens
}

// MicroUSDPerUSD is the single precision unit used by pricing, balances,
// reservations, settlements and quotas. All persisted billing amounts are
// signed or unsigned int64 micro-USD; float64 is display-only.
const MicroUSDPerUSD int64 = money.MicrosPerUSD

// MaxWholeUSD is the largest whole-dollar amount that can be represented
// after conversion to micro-USD.
const MaxWholeUSD int64 = money.MaxWholeUSD

// MicroToUSD returns a micro-USD amount as dollars for human-facing display.
func MicroToUSD(micro int64) float64 {
	return money.MicrosToUSD(micro)
}

// WholeUSDToMicro converts whole dollars into micro-USD without overflow.
func WholeUSDToMicro(usd int64) (int64, bool) {
	micro, err := money.WholeUSDToMicros(usd)
	return micro, err == nil
}

// BillingResult uses microcredit precision end to end, including lease
// settlement and the remaining V2 strict-debit subscription path.
type BillingResult struct {
	CatalogBaseMicro           int64 // 目录基准价（倍率 1），谁都不付这个数，只作基数与参照
	TenantPayableMicro         int64 // 平台向租户应收：目录价 x 租户结算倍率
	RetailBaseMicro            int64 // 分组零售价格表原价；订阅套餐额度计量基数
	UserPayableMicro           int64 // 用户零售应收：零售原价 x 有效用户倍率
	UserChargedMicro           int64 // 用户实际扣费；订阅覆盖时为 0
	APIKeyQuotaCostMicro       int64 // 扣减 API key 本地配额计数
	ServiceTier                ServiceTier
	BillingBreakdownJSON       []byte
	BillableUnits              int64
	BillableUnitType           string
	GroupNameSnapshot          string
	GroupDefaultUserMultiplier float64
	UserMultiplierOverride     *float64
	EffectiveUserMultiplier    float64
	BillingGroupLabel          string
}

// BillingSnapshot contains every mutable pricing input required to price one
// candidate. It is resolved before the first upstream byte is sent; completion
// applies actual usage without reading control-plane tables again.
type BillingSnapshot struct {
	RetailEntry                PriceBookEntry
	AccountEntry               PriceBookEntry
	GroupName                  string
	GroupDefaultUserMultiplier float64
	UserMultiplierOverride     *float64
	EffectiveUserMultiplier    float64
	ServiceTier                ServiceTier
}

// ============================================================================
// Rate limiting
// ============================================================================

// LimitPolicy is a resolved rate limit rule for a scope.
type LimitPolicy struct {
	ConcurrencyLimit *int
}

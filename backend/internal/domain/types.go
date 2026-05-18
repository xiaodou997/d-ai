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
	DisplayName            string
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
	TimeoutMs          int
	ProviderCode       string

	// Pool-based route fields (OAuth Fixed Provider)
	PoolID             string            // ai_credential_pools.id
	PoolUpstreamModel  string            // model name to send to the upstream
	FixedProviderType  FixedProviderType // "codex" | "claude_oauth" | "gemini_cli" | "antigravity"
	OAuthStrategy      string            // "round_robin" | "weighted"

	// P3: scoring hints loaded from ai_model_routes
	CostPer1kTokens    float64                // 0 for free/pool routes → scorer treats as very cheap
	ScoreWeightsOverride map[string]float64   // nil = use global weights
}

// IsPoolRoute returns true when this route targets a CredentialPool (OAuth Fixed Provider).
func (r *RouteCandidate) IsPoolRoute() bool { return r.PoolID != "" }

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

// TokenUsage records actual token counts from a completed upstream response.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	CacheWriteTokens int // tokens written to provider cache (billed at input price)
	CacheReadTokens  int // tokens read from provider cache (billed at input price, margin opportunity)
	ReasoningTokens  int // extended thinking / reasoning tokens
}

func (u TokenUsage) TotalTokens() int {
	return u.PromptTokens + u.CompletionTokens +
		u.CacheWriteTokens + u.CacheReadTokens + u.ReasoningTokens
}

// ModelPricing holds per-1M-token prices in integer credits.
// Zero values mean "use the corresponding base price" — see Effective* methods.
type ModelPricing struct {
	InputPer1M       int64
	OutputPer1M      int64
	CacheWritePer1M  int64 // 0 = bill at InputPer1M
	CacheReadPer1M   int64 // 0 = bill at InputPer1M (profit: provider charges less)
	ReasoningPer1M   int64 // 0 = bill at OutputPer1M
	ImageSizePrices  map[string]int64
	VideoPricePerSec int64
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

// BillingResult is the output of cost calculation.
type BillingResult struct {
	ProviderCost     int64  // what the platform pays the upstream
	PlatformCost     int64  // what the tenant pays the platform (→ URM TenantAmount)
	UserCost         int64  // what the user pays the tenant (→ URM UserAmount; 0 for tenant-owned keys)
	APIKeyQuotaCost  int64  // deducted from the API key's local quota counter
	BillableUnits    int64
	BillableUnitType string
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

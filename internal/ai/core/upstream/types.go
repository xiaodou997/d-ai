package upstream

import (
	"time"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
)

// ProviderFamily mirrors catalog provider families at the upstream layer.
type ProviderFamily string

const (
	ProviderFamilyOpenAICompatible ProviderFamily = "openai_compatible"
	ProviderFamilyAnthropic        ProviderFamily = "anthropic"
	ProviderFamilyGoogle           ProviderFamily = "google"
	ProviderFamilyOther            ProviderFamily = "other"
)

// AccessMode differentiates direct upstreams from pool-backed upstreams.
type AccessMode string

const (
	AccessModeDirect    AccessMode = "direct"
	AccessModeOAuthPool AccessMode = "oauth_pool"
)

// Status tracks lifecycle for upstream resources.
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusInvalid  Status = "invalid"
)

// Upstream is the top-level vendor endpoint resource.
type Upstream struct {
	ID               string
	Code             string
	Name             string
	ProviderFamily   ProviderFamily
	AccessMode       AccessMode
	BaseURL          string
	Headers          map[string]string
	ConcurrencyLimit *int
	Status           Status
	Notes            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CredentialKind describes how a direct upstream is authenticated.
type CredentialKind string

const (
	CredentialKindAPIKey CredentialKind = "api_key"
	CredentialKindBearer CredentialKind = "bearer"
	CredentialKindBasic  CredentialKind = "basic"
	CredentialKindNone   CredentialKind = "none"
)

// Credential stores one direct credential for an upstream.
type Credential struct {
	ID               string
	UpstreamID       string
	Name             string
	CredentialKind   CredentialKind
	HeaderName       string
	SecretCiphertext string
	ExtraAuth        map[string]any
	Weight           int
	Status           Status
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// FixedProviderType identifies an OAuth-backed provider pool.
type FixedProviderType string

const (
	FixedProviderCodex       FixedProviderType = "codex"
	FixedProviderClaudeOAuth FixedProviderType = "claude_oauth"
	FixedProviderGeminiCLI   FixedProviderType = "gemini_cli"
	FixedProviderAntigravity FixedProviderType = "antigravity"
	FixedProviderCustom      FixedProviderType = "custom"
)

// SelectionStrategy chooses how credentials are selected inside a pool.
type SelectionStrategy string

const (
	SelectionRoundRobin SelectionStrategy = "round_robin"
	SelectionWeighted   SelectionStrategy = "weighted"
)

// StickyScope controls whether sticky routing is bound to a credential or pool.
type StickyScope string

const (
	StickyScopeCredential StickyScope = "credential"
	StickyScopePool       StickyScope = "pool"
)

// OAuthPool stores a fixed-provider credential pool.
type OAuthPool struct {
	ID                string
	Code              string
	Name              string
	FixedProviderType FixedProviderType
	SelectionStrategy SelectionStrategy
	StickyScope       StickyScope
	Status            Status
	Notes             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// OAuthPoolCredential stores one credential inside a fixed-provider pool.
type OAuthPoolCredential struct {
	ID                     string
	PoolID                 string
	Name                   string
	Email                  string
	AccessTokenCiphertext  string
	RefreshTokenCiphertext string
	TokenType              string
	Scope                  string
	ExpiresAt              *time.Time
	AuthMetadata           map[string]any
	Weight                 int
	Status                 Status
	InvalidReason          string
	LastUsedAt             *time.Time
	LastRefreshedAt        *time.Time
	ConsecutiveFailCount   int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// ModelBinding explicitly maps an internal model to one upstream model over
// concrete request/response surfaces.
type ModelBinding struct {
	ID                string
	UpstreamKind      AccessMode
	UpstreamID        string
	ModelID           string
	Capability        catalog.Capability
	RequestSurface    surface.ID
	ResponseSurface   surface.ID
	UpstreamModelName string
	Priority          int
	Status            Status
	Config            map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

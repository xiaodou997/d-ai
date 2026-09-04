package domain

import "time"

const (
	UpstreamAccountStatusActive   = "active"
	UpstreamAccountStatusInvalid  = "invalid"
	UpstreamAccountStatusDisabled = "disabled"
)

// UpstreamAccount is one concrete API-key supplier account. Its supported
// request formats and transport URLs are declared by Endpoints.
// The encrypted API key is intentionally NOT part of this type: it never leaves
// the repository/secret boundary. Some queries (create/list) do not return every
// field; absent fields are left zero.
type UpstreamAccount struct {
	ID                string
	Name              string
	TenantDisplayName string
	TenantAccessMode  string
	Endpoints         []UpstreamAccountEndpoint
	ConcurrencyLimit  *int
	// 账号级上游成本绑定。PriceBookID 空 = 未绑定；
	// TenantMultiplier nil = 未设置（计费时 COALESCE 到 1）。
	PriceBookID      string
	TenantMultiplier *float64
	Status           string
	InvalidReason    string
	InvalidAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

const (
	EndpointStatusActive   = "active"
	EndpointStatusDisabled = "disabled"

	EndpointAuthFormatDefault   = "format_default"
	EndpointAuthBearer          = "bearer"
	EndpointAuthAnthropicAPIKey = "anthropic_api_key"
	EndpointAuthGeminiAPIKey    = "gemini_api_key"
	EndpointAuthCustomHeader    = "custom_header"
)

// UpstreamAccountEndpoint declares one exact API format exposed by a direct
// upstream account. An account may have several formats, but at most one
// endpoint for each format.
type UpstreamAccountEndpoint struct {
	ID            string
	AccountID     string
	APIFormat     UpstreamProtocol
	BaseURL       string
	PathOverride  string
	AuthScheme    string
	AuthHeader    string
	ExtraHeaders  []byte
	Status        string
	HealthStatus  HealthStatus
	LastError     string
	LastCheckedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UpstreamAccountEndpointWrite struct {
	APIFormat    UpstreamProtocol
	BaseURL      string
	PathOverride string
	AuthScheme   string
	AuthHeader   string
	ExtraHeaders []byte
	Status       string
}

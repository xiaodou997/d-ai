package identity

import "time"

// Scope identifies who owns or executes an AI action in the rebuilt model.
type Scope string

const (
	ScopePlatform Scope = "platform"
	ScopeTenant   Scope = "tenant"
	ScopeUser     Scope = "user"
)

// AuthMethod describes how a runtime subject authenticated.
type AuthMethod string

const (
	AuthMethodAPIKey    AuthMethod = "api_key"
	AuthMethodJWT       AuthMethod = "jwt"
	AuthMethodInvokeKey AuthMethod = "invoke_key"
)

// RequestSource distinguishes the user-facing entrypoint that triggered a
// runtime request.
type RequestSource string

const (
	RequestSourceAPIKey     RequestSource = "api_key"
	RequestSourceWebChat    RequestSource = "web_chat"
	RequestSourceWebImage   RequestSource = "web_image"
	RequestSourceAppPreview RequestSource = "app_preview"
	RequestSourceInvokeKey  RequestSource = "invoke_key"
)

// APIKey is the rebuilt AI gateway key model.
type APIKey struct {
	ID              string
	OwnerScope      Scope
	TenantID        string
	UserID          string
	GroupID         string
	KeyHash         string
	LastFour        string
	Name            string
	QuotaLimitMicro *int64
	QuotaUsedMicro  int64
	AllowedModelIDs []string
	Status          string
	ExpiresAt       *time.Time
	LastUsedAt      *time.Time
	CreatedBy       string
	UpdatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Subject is the normalized runtime actor after auth expansion.
type Subject struct {
	AuthMethod    AuthMethod
	RequestSource RequestSource
	Scope         Scope
	TenantID      string
	UserID        string
	APIKeyID      string
	InvokeKeyID   string
	GroupID       string
	AllowedModels []string
	QuotaLimit    *int64
	QuotaUsed     int64

	// ForcedGroupID 强制本次请求只使用该分组。仅智能应用调用路径设置；
	// 租户身份可使用本租户活跃分组，用户身份仍在每次执行时复核公开/专属授权，
	// 计费按调用者在该分组的当前有效倍率解析。
	ForcedGroupID string
	// AppID / AppName 是本次请求经由的智能应用快照,写入使用日志,
	// 供使用记录展示与对非应用所有者的脱敏。
	AppID            string
	AppName          string
	AppOwnerType     string
	AppOwnerTenantID string
	AppOwnerUserID   string
}

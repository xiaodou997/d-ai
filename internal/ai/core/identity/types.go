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
	AuthMethodAPIKey AuthMethod = "api_key"
	AuthMethodJWT    AuthMethod = "jwt"
)

// RequestSource distinguishes the user-facing entrypoint that triggered a
// runtime request.
type RequestSource string

const (
	RequestSourceAPIKey   RequestSource = "api_key"
	RequestSourceWebChat  RequestSource = "web_chat"
	RequestSourceWebImage RequestSource = "web_image"
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
	GroupID       string
	AllowedModels []string
	QuotaLimit    *int64
	QuotaUsed     int64

	// ForcedGroupID constrains an explicitly selected runtime request to one group.
	ForcedGroupID string
}

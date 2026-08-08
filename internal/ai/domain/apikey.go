package domain

import "time"

// APIKey is the management-domain representation of an API key. Quota fields
// are micro-USD; the HTTP layer converts them to/from display USD.
type APIKey struct {
	ID              string
	OwnerType       OwnerType
	TenantID        string
	UserID          string // empty for tenant-owned keys
	GroupID         string
	LastFour        string
	Name            string
	QuotaLimitMicro *int64 // nil = unlimited
	QuotaUsedMicro  int64
	AllowedModels   []string
	Status          string
	ExpiresAt       *time.Time
	LastUsedAt      *time.Time
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const (
	// APIKeyStatusActive is the default lifecycle status for a freshly created key.
	APIKeyStatusActive = "active"
	// APIKeyStatusDisabled means the key is retained but rejected by auth/runtime.
	APIKeyStatusDisabled = "disabled"
)

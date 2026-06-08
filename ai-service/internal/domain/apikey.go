package domain

import "time"

// APIKey is the management-domain representation of an API key. It is distinct
// from APIKeyAuth, the slim projection used by the runtime auth path. Quota
// fields are micro-credits (the internal precision unit, see
// MicroCreditsPerCredit); the HTTP layer converts to/from display credits.
type APIKey struct {
	ID                 string
	OwnerType          OwnerType
	TenantID           string
	UserID             string // empty for tenant-owned keys
	LastFour           string
	Name               string
	QuotaLimitMicro    *int64 // nil = unlimited
	QuotaUsedMicro     int64
	QuotaReservedMicro int64
	AllowedModels      []string
	Status             string
	ExpiresAt          *time.Time
	LastUsedAt         *time.Time
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// APIKeyStatusActive is the default lifecycle status for a freshly created key.
const APIKeyStatusActive = "active"

package domain

import "time"

// ManagedModel is the management-domain view of a row in ai_models (the console
// CRUD projection). It is distinct from Model, the slimmer runtime projection
// used by the serving pipeline. The table has NO display_name column;
// DefaultMaxOutputTokens is a non-null int32 (DB default 2048); ContextWindow
// and MaxOutputTokens are nullable (nil = NULL column).
type ManagedModel struct {
	ID                     string
	ModelCode              string
	CapabilityType         string
	ContextWindow          *int32
	DefaultMaxOutputTokens int32
	MaxOutputTokens        *int32
	Status                 string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

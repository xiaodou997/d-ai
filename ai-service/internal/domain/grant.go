package domain

import "time"

// TenantModelGrant authorises a tenant to use a model. ModelCode/CapabilityType
// are populated only by list queries (which join ai_models); create/update
// return them empty. The grants table has no updated_at column.
type TenantModelGrant struct {
	ID             string
	TenantID       string
	ModelID        string
	ModelCode      string
	CapabilityType string
	Status         string
	CreatedBy      string
	CreatedAt      time.Time
}

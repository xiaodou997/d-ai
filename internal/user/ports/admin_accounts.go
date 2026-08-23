package ports

import (
	"context"
	"time"
)

// AdminAccountRow is the non-secret account projection consumed by admin
// management handlers.
type AdminAccountRow struct {
	UserID          string
	Username        string
	Email           *string
	Status          string
	CredentialState string
	CreatedAt       time.Time
}

// AdminAccountPage is the pagination contract shared by the application
// boundary and persistence adapter.
type AdminAccountPage struct {
	Records []AdminAccountRow
	Total   int64
	Page    int
	Size    int
}

// AdminAccountReader exposes only the read projections needed by management
// list endpoints; SQL and persistence details remain in the adapter.
type AdminAccountReader interface {
	ListSystemAdmins(ctx context.Context, keyword string, page, size int) (AdminAccountPage, error)
	ListTenantUsers(ctx context.Context, tenantID, keyword string, page, size int) (AdminAccountPage, error)
}

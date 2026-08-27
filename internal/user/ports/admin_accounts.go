package ports

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrTenantNotFound means an account mutation referenced a tenant that is
	// no longer present. The account adapter exposes this persistence boundary
	// error without importing the tenant domain package.
	ErrTenantNotFound = errors.New("tenant not found")
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

// AdminAccountCreate contains normalized account fields and one-time
// activation material prepared by the authentication application service.
type AdminAccountCreate struct {
	UserID              string
	TenantID            string
	Username            string
	Email               string
	PasswordHash        string
	ActivationTokenHash []byte
	ActivationExpiresAt time.Time
}

// AdminAccountUpdate is the mutable profile/state projection for an existing
// system or tenant account.
type AdminAccountUpdate struct {
	UserID string
	Email  string
	Status string
}

// AdminAccountMutationResult preserves the legacy distinction between a
// missing target and an attempt to mutate the super administrator.
type AdminAccountMutationResult struct {
	Updated   bool
	Forbidden bool
}

// AdminAccountReader exposes only the read projections needed by management
// list endpoints; SQL and persistence details remain in the adapter.
type AdminAccountReader interface {
	ListSystemAdmins(ctx context.Context, keyword string, page, size int) (AdminAccountPage, error)
	ListTenantUsers(ctx context.Context, tenantID, keyword string, page, size int) (AdminAccountPage, error)
}

// AdminAccountWriter owns lifecycle mutations for system administrators and
// tenant users. Blacklist/session effects are coordinated by the separate
// auth/ports.AccountSecurityWriter application port.
type AdminAccountWriter interface {
	CreateSystemAdmin(ctx context.Context, input AdminAccountCreate) error
	CreateTenantUser(ctx context.Context, input AdminAccountCreate) error
	UpdateSystemAdmin(ctx context.Context, input AdminAccountUpdate) (AdminAccountMutationResult, error)
	UpdateTenantUser(ctx context.Context, input AdminAccountUpdate) (bool, error)
	UpdateTenantUserStatus(ctx context.Context, userID, status string) (bool, error)
	ResetSystemAdminPassword(ctx context.Context, userID string) (ActivationCredentialResult, error)
	ResetTenantUserPassword(ctx context.Context, userID string) (ActivationCredentialResult, error)
	DeleteSystemAdmin(ctx context.Context, userID string) (AdminAccountMutationResult, error)
}

package ports

import (
	"context"
	"time"
)

// AdminTenantStatusResult describes the tenant cascade committed by the
// management status endpoint.
type AdminTenantStatusResult struct {
	Updated         bool
	RestoredUserIDs []string
}

// AdminTenantStatusWriter owns the tenant status transaction and account
// cascade. Redis blacklist synchronization remains outside persistence.
type AdminTenantStatusWriter interface {
	UpdateStatus(ctx context.Context, tenantID, status string) (AdminTenantStatusResult, error)
}

// TenantInitialUserCreate is the optional pending-activation organization
// user created together with a tenant.
type TenantInitialUserCreate struct {
	UserID              string
	Username            string
	Email               string
	PasswordHash        string
	ActivationTokenHash []byte
	ActivationExpiresAt time.Time
}

// TenantCreateCommand contains the complete atomic tenant creation input.
type TenantCreateCommand struct {
	TenantID      string
	TenantName    string
	ContactPerson string
	ContactEmail  string
	Status        string
	InitialUser   *TenantInitialUserCreate
}

// TenantUpdateCommand contains mutable tenant profile and access state.
type TenantUpdateCommand struct {
	TenantID      string
	TenantName    string
	ContactPerson string
	ContactEmail  string
	Status        string
}

// AdminTenantWriter owns tenant lifecycle persistence. Account activation is
// part of CreateTenant's transaction when an initial user is requested.
type AdminTenantWriter interface {
	CreateTenant(ctx context.Context, input TenantCreateCommand) error
	UpdateTenant(ctx context.Context, input TenantUpdateCommand) (bool, error)
	DeleteTenant(ctx context.Context, tenantID string) (bool, error)
}

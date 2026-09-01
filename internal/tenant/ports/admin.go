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

// AdminTenantSecurityError marks a committed tenant status transition whose
// post-commit ban projection failed. The transition itself is durable and the
// security command can be retried independently.
type AdminTenantSecurityError struct {
	Cause error
}

func (e *AdminTenantSecurityError) Error() string { return "admin tenant security sync failed" }
func (e *AdminTenantSecurityError) Unwrap() error { return e.Cause }

// AdminTenantStatusWriter owns the tenant status transaction and account
// cascade. Redis blacklist synchronization is coordinated by the separate
// auth security command after the transaction commits.
type AdminTenantStatusWriter interface {
	UpdateStatus(ctx context.Context, tenantID, status string) (AdminTenantStatusResult, error)
}

// AdminTenantLifecycle coordinates tenant persistence with the security
// projection after a status transition commits.
type AdminTenantLifecycle interface {
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

type TenantDeletionJob struct {
	JobID        string     `json:"jobId"`
	TenantID     string     `json:"tenantId"`
	Status       string     `json:"status"`
	RequestedAt  time.Time  `json:"requestedAt"`
	ExecuteAfter time.Time  `json:"executeAfter"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	LastError    string     `json:"lastError,omitempty"`
}

type TenantDeletionService interface {
	Request(ctx context.Context, tenantID, requestedBy string) (TenantDeletionJob, error)
	Cancel(ctx context.Context, tenantID string) (bool, error)
	Get(ctx context.Context, tenantID string) (TenantDeletionJob, error)
}

type TenantDeletionStore interface {
	RequestDeletion(ctx context.Context, tenantID, requestedBy string, executeAfter time.Time) (TenantDeletionJob, error)
	CancelDeletion(ctx context.Context, tenantID string) (bool, error)
	GetDeletion(ctx context.Context, tenantID string) (TenantDeletionJob, error)
	RunDeletion(ctx context.Context, jobID, tenantID string) error
	GetDueDeletion(ctx context.Context) (TenantDeletionJob, error)
}

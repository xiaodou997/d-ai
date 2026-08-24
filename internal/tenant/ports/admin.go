package ports

import "context"

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

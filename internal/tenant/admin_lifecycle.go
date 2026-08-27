package tenant

import (
	"context"
	"errors"

	authports "xiaodou/dai/internal/auth/ports"
	tenantports "xiaodou/dai/internal/tenant/ports"
)

// AdminTenantLifecycleService owns the application-level sequence for tenant
// status changes: the tenant writer commits its cascade first, then the auth
// security writer projects the committed status and restored user set.
type AdminTenantLifecycleService struct {
	writer   tenantports.AdminTenantStatusWriter
	security authports.AccountSecurityWriter
}

var _ tenantports.AdminTenantLifecycle = (*AdminTenantLifecycleService)(nil)

func NewAdminTenantLifecycleService(writer tenantports.AdminTenantStatusWriter, security authports.AccountSecurityWriter) *AdminTenantLifecycleService {
	return &AdminTenantLifecycleService{writer: writer, security: security}
}

func (s *AdminTenantLifecycleService) UpdateStatus(ctx context.Context, tenantID, status string) (tenantports.AdminTenantStatusResult, error) {
	if s == nil || s.writer == nil {
		return tenantports.AdminTenantStatusResult{}, errors.New("admin tenant lifecycle writer is not configured")
	}
	result, err := s.writer.UpdateStatus(ctx, tenantID, status)
	if err != nil || !result.Updated || s.security == nil {
		return result, err
	}
	if err := s.security.SyncTenantStatus(ctx, tenantID, status, result.RestoredUserIDs); err != nil {
		return result, &tenantports.AdminTenantSecurityError{Cause: err}
	}
	return result, nil
}

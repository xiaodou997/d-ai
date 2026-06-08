// Package grant holds the business logic for tenant model-grant management
// (the console management plane). Service owns validation and default filling;
// persistence is reached through Repository, defined on the consumer side.
package grant

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the grant service.
type Repository interface {
	GrantToTenant(ctx context.Context, w GrantWrite) (domain.TenantModelGrant, error)
	ListForTenant(ctx context.Context, tenantID string) ([]domain.TenantModelGrant, error)
	UpdateStatus(ctx context.Context, tenantID, modelID, status string) (domain.TenantModelGrant, error)
}

// GrantWrite is the persistence-level payload for granting a model to a tenant.
type GrantWrite struct {
	TenantID  string
	ModelID   string
	Status    string
	CreatedBy string
}

package grant

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Service implements tenant model-grant management business logic.
type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// GrantInput is the decoded grant request.
type GrantInput struct {
	TenantID  string
	ModelID   string
	Status    string
	CreatedBy string
}

// GrantToTenant validates input and persists the grant.
func (s *Service) GrantToTenant(ctx context.Context, in GrantInput) (domain.TenantModelGrant, error) {
	if in.ModelID == "" {
		return domain.TenantModelGrant{}, domain.NewValidationError("model_id", "model_id is required")
	}
	status := in.Status
	if status == "" {
		status = domain.APIKeyStatusActive
	}
	return s.repo.GrantToTenant(ctx, GrantWrite{
		TenantID:  in.TenantID,
		ModelID:   in.ModelID,
		Status:    status,
		CreatedBy: in.CreatedBy,
	})
}

// ListForTenant returns all model grants for a tenant.
func (s *Service) ListForTenant(ctx context.Context, tenantID string) ([]domain.TenantModelGrant, error) {
	return s.repo.ListForTenant(ctx, tenantID)
}

// UpdateStatus changes a grant's lifecycle status.
func (s *Service) UpdateStatus(ctx context.Context, tenantID, modelID, status string) (domain.TenantModelGrant, error) {
	if status == "" {
		return domain.TenantModelGrant{}, domain.NewValidationError("status", "status is required")
	}
	return s.repo.UpdateStatus(ctx, tenantID, modelID, status)
}

package postgres

import (
	"context"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svcgrant "xiaodou/unihub/ai-service/internal/service/grant"
)

// GrantRepo implements service/grant.Repository on top of sqlc.
type GrantRepo struct {
	q *dbgen.Queries
}

func NewGrantRepo(q *dbgen.Queries) *GrantRepo {
	return &GrantRepo{q: q}
}

var _ svcgrant.Repository = (*GrantRepo)(nil)

func (r *GrantRepo) GrantToTenant(ctx context.Context, w svcgrant.GrantWrite) (domain.TenantModelGrant, error) {
	modelID, err := akUUID(w.ModelID)
	if err != nil {
		return domain.TenantModelGrant{}, domain.NewValidationError("model_id", "invalid model_id")
	}
	row, err := r.q.GrantModelToTenant(ctx, dbgen.GrantModelToTenantParams{
		TenantID:  w.TenantID,
		ModelID:   modelID,
		Status:    w.Status,
		CreatedBy: akText(w.CreatedBy),
	})
	if err != nil {
		return domain.TenantModelGrant{}, err
	}
	return grantFromRow(row), nil
}

func (r *GrantRepo) ListForTenant(ctx context.Context, tenantID string) ([]domain.TenantModelGrant, error) {
	rows, err := r.q.ListTenantModelGrants(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.TenantModelGrant, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.TenantModelGrant{
			ID:             uuidToString(row.ID),
			TenantID:       row.TenantID,
			ModelID:        uuidToString(row.ModelID),
			ModelCode:      row.ModelCode,
			CapabilityType: row.CapabilityType,
			Status:         row.Status,
			CreatedBy:      row.CreatedBy.String,
			CreatedAt:      row.CreatedAt.Time,
		})
	}
	return out, nil
}

func (r *GrantRepo) UpdateStatus(ctx context.Context, tenantID, modelID, status string) (domain.TenantModelGrant, error) {
	mid, err := akUUID(modelID)
	if err != nil {
		return domain.TenantModelGrant{}, domain.NewValidationError("model_id", "invalid model_id")
	}
	row, err := r.q.UpdateTenantModelGrantStatus(ctx, dbgen.UpdateTenantModelGrantStatusParams{
		TenantID: tenantID,
		ModelID:  mid,
		Status:   status,
	})
	if err != nil {
		return domain.TenantModelGrant{}, err
	}
	return grantFromRow(row), nil
}

func grantFromRow(row dbgen.AiTenantModelGrant) domain.TenantModelGrant {
	return domain.TenantModelGrant{
		ID:        uuidToString(row.ID),
		TenantID:  row.TenantID,
		ModelID:   uuidToString(row.ModelID),
		Status:    row.Status,
		CreatedBy: row.CreatedBy.String,
		CreatedAt: row.CreatedAt.Time,
	}
}

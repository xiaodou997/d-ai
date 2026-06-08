package postgres

import (
	"context"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svcmodel "xiaodou/unihub/ai-service/internal/service/model"
)

// ModelRepo implements service/model.Repository on top of sqlc. The ai_models
// table has no display_name column; default_max_output_tokens is a non-null
// column.
type ModelRepo struct {
	q *dbgen.Queries
}

func NewModelRepo(q *dbgen.Queries) *ModelRepo {
	return &ModelRepo{q: q}
}

var _ svcmodel.Repository = (*ModelRepo)(nil)

func (r *ModelRepo) Create(ctx context.Context, w svcmodel.ModelWrite) (domain.ManagedModel, error) {
	row, err := r.q.CreateModel(ctx, dbgen.CreateModelParams{
		ModelCode:              w.ModelCode,
		CapabilityType:         w.CapabilityType,
		ContextWindow:          akInt4Ptr(w.ContextWindow),
		DefaultMaxOutputTokens: w.DefaultMaxOutputTokens,
		MaxOutputTokens:        akInt4Ptr(w.MaxOutputTokens),
		Status:                 w.Status,
	})
	if err != nil {
		return domain.ManagedModel{}, err
	}
	return domain.ManagedModel{
		ID:                     uuidToString(row.ID),
		ModelCode:              row.ModelCode,
		CapabilityType:         row.CapabilityType,
		ContextWindow:          akInt4StrPtr(row.ContextWindow),
		DefaultMaxOutputTokens: row.DefaultMaxOutputTokens,
		MaxOutputTokens:        akInt4StrPtr(row.MaxOutputTokens),
		Status:                 row.Status,
		CreatedAt:              row.CreatedAt.Time,
		UpdatedAt:              row.UpdatedAt.Time,
	}, nil
}

func (r *ModelRepo) List(ctx context.Context) ([]domain.ManagedModel, error) {
	rows, err := r.q.ListAdminModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ManagedModel, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.ManagedModel{
			ID:                     uuidToString(row.ID),
			ModelCode:              row.ModelCode,
			CapabilityType:         row.CapabilityType,
			ContextWindow:          akInt4StrPtr(row.ContextWindow),
			DefaultMaxOutputTokens: row.DefaultMaxOutputTokens,
			MaxOutputTokens:        akInt4StrPtr(row.MaxOutputTokens),
			Status:                 row.Status,
			CreatedAt:              row.CreatedAt.Time,
			UpdatedAt:              row.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (r *ModelRepo) Update(ctx context.Context, id string, w svcmodel.ModelWrite) (domain.ManagedModel, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.ManagedModel{}, err
	}
	row, err := r.q.UpdateModel(ctx, dbgen.UpdateModelParams{
		ID:                     uid,
		ModelCode:              w.ModelCode,
		CapabilityType:         w.CapabilityType,
		ContextWindow:          akInt4Ptr(w.ContextWindow),
		DefaultMaxOutputTokens: w.DefaultMaxOutputTokens,
		MaxOutputTokens:        akInt4Ptr(w.MaxOutputTokens),
		Status:                 w.Status,
	})
	if err != nil {
		return domain.ManagedModel{}, err
	}
	return domain.ManagedModel{
		ID:                     uuidToString(row.ID),
		ModelCode:              row.ModelCode,
		CapabilityType:         row.CapabilityType,
		ContextWindow:          akInt4StrPtr(row.ContextWindow),
		DefaultMaxOutputTokens: row.DefaultMaxOutputTokens,
		MaxOutputTokens:        akInt4StrPtr(row.MaxOutputTokens),
		Status:                 row.Status,
		CreatedAt:              row.CreatedAt.Time,
		UpdatedAt:              row.UpdatedAt.Time,
	}, nil
}

func (r *ModelRepo) UpdateStatus(ctx context.Context, id, status string) (domain.ManagedModel, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.ManagedModel{}, err
	}
	row, err := r.q.UpdateModelStatus(ctx, dbgen.UpdateModelStatusParams{
		ID:     uid,
		Status: status,
	})
	if err != nil {
		return domain.ManagedModel{}, err
	}
	return domain.ManagedModel{
		ID:                     uuidToString(row.ID),
		ModelCode:              row.ModelCode,
		CapabilityType:         row.CapabilityType,
		ContextWindow:          akInt4StrPtr(row.ContextWindow),
		DefaultMaxOutputTokens: row.DefaultMaxOutputTokens,
		MaxOutputTokens:        akInt4StrPtr(row.MaxOutputTokens),
		Status:                 row.Status,
		CreatedAt:              row.CreatedAt.Time,
		UpdatedAt:              row.UpdatedAt.Time,
	}, nil
}

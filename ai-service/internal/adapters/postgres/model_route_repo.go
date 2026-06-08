package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svcmodel "xiaodou/unihub/ai-service/internal/service/model"
)

// ModelRouteRepo implements service/model.RouteRepository on top of sqlc. The
// table has no Delete query, so Delete uses a direct pool.Exec (as the old
// handler did).
type ModelRouteRepo struct {
	q    *dbgen.Queries
	pool *pgxpool.Pool
}

func NewModelRouteRepo(q *dbgen.Queries, pool *pgxpool.Pool) *ModelRouteRepo {
	return &ModelRouteRepo{q: q, pool: pool}
}

var _ svcmodel.RouteRepository = (*ModelRouteRepo)(nil)

func (r *ModelRouteRepo) Create(ctx context.Context, w svcmodel.RouteWrite) (domain.ModelRoute, error) {
	modelID, err := akUUID(w.ModelID)
	if err != nil {
		return domain.ModelRoute{}, err
	}
	deploymentID, err := akUUID(w.UpstreamDeploymentID)
	if err != nil {
		return domain.ModelRoute{}, domain.NewValidationError("upstream_deployment_id", "invalid upstream_deployment_id")
	}
	row, err := r.q.CreateModelRoute(ctx, dbgen.CreateModelRouteParams{
		ModelID:              modelID,
		UpstreamDeploymentID: deploymentID,
		Priority:             w.Priority,
		Weight:               w.Weight,
		SupportsStream:       w.SupportsStream,
		Status:               w.Status,
	})
	if err != nil {
		return domain.ModelRoute{}, err
	}
	return modelRouteFromRow(row), nil
}

func (r *ModelRouteRepo) List(ctx context.Context, modelID string) ([]domain.ModelRouteListItem, error) {
	mid, err := akUUID(modelID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListModelRoutes(ctx, mid)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ModelRouteListItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.ModelRouteListItem{
			ModelRoute: domain.ModelRoute{
				ID:                   uuidToString(row.ID),
				ModelID:              uuidToString(row.ModelID),
				UpstreamDeploymentID: uuidToString(row.UpstreamDeploymentID),
				Priority:             row.Priority,
				Weight:               row.Weight,
				SupportsStream:       row.SupportsStream,
				Status:               row.Status,
				CreatedAt:            row.CreatedAt.Time,
				UpdatedAt:            row.UpdatedAt.Time,
			},
			UpstreamDeploymentName: row.UpstreamDeploymentName,
			UpstreamModel:          row.UpstreamModel,
			CapabilityType:         row.CapabilityType,
			UpstreamProtocol:       row.UpstreamProtocol,
			HealthStatus:           row.HealthStatus,
			CredentialSource:       row.CredentialSource,
			EndpointID:             uuidToString(row.EndpointID),
			EndpointName:           row.EndpointName,
			BaseURL:                row.BaseUrl,
			ProviderID:             uuidToString(row.ProviderID),
			ProviderCode:           row.ProviderCode,
			ProviderName:           row.ProviderName,
			PoolID:                 uuidToString(row.PoolID),
			PoolName:               row.PoolName,
			FixedProviderType:      row.FixedProviderType,
		})
	}
	return out, nil
}

func (r *ModelRouteRepo) Get(ctx context.Context, routeID string) (domain.ModelRoute, error) {
	rid, err := akUUID(routeID)
	if err != nil {
		return domain.ModelRoute{}, err
	}
	row, err := r.q.GetModelRoute(ctx, rid)
	if err != nil {
		return domain.ModelRoute{}, err
	}
	return modelRouteFromRow(row), nil
}

func (r *ModelRouteRepo) Update(ctx context.Context, w svcmodel.RouteUpdate) (domain.ModelRoute, error) {
	modelID, err := akUUID(w.ModelID)
	if err != nil {
		return domain.ModelRoute{}, err
	}
	routeID, err := akUUID(w.RouteID)
	if err != nil {
		return domain.ModelRoute{}, err
	}
	deploymentID, err := akUUID(w.UpstreamDeploymentID)
	if err != nil {
		return domain.ModelRoute{}, domain.NewValidationError("upstream_deployment_id", "invalid upstream_deployment_id")
	}
	row, err := r.q.UpdateModelRoute(ctx, dbgen.UpdateModelRouteParams{
		ModelID:              modelID,
		ID:                   routeID,
		UpstreamDeploymentID: deploymentID,
		Priority:             w.Priority,
		Weight:               w.Weight,
		SupportsStream:       w.SupportsStream,
		Status:               w.Status,
	})
	if err != nil {
		return domain.ModelRoute{}, err
	}
	return modelRouteFromRow(row), nil
}

func (r *ModelRouteRepo) UpdateStatus(ctx context.Context, modelID, routeID, status string) (domain.ModelRoute, error) {
	mid, err := akUUID(modelID)
	if err != nil {
		return domain.ModelRoute{}, err
	}
	rid, err := akUUID(routeID)
	if err != nil {
		return domain.ModelRoute{}, err
	}
	row, err := r.q.UpdateModelRouteStatus(ctx, dbgen.UpdateModelRouteStatusParams{
		ModelID: mid,
		ID:      rid,
		Status:  status,
	})
	if err != nil {
		return domain.ModelRoute{}, err
	}
	return modelRouteFromRow(row), nil
}

func (r *ModelRouteRepo) Delete(ctx context.Context, modelID, routeID string) error {
	mid, err := akUUID(modelID)
	if err != nil {
		return err
	}
	rid, err := akUUID(routeID)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx,
		"DELETE FROM ai_model_routes WHERE model_id = $1 AND id = $2",
		mid, rid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func modelRouteFromRow(row dbgen.AiModelRoute) domain.ModelRoute {
	return domain.ModelRoute{
		ID:                   uuidToString(row.ID),
		ModelID:              uuidToString(row.ModelID),
		UpstreamDeploymentID: uuidToString(row.UpstreamDeploymentID),
		Priority:             row.Priority,
		Weight:               row.Weight,
		SupportsStream:       row.SupportsStream,
		Status:               row.Status,
		CreatedAt:            row.CreatedAt.Time,
		UpdatedAt:            row.UpdatedAt.Time,
	}
}

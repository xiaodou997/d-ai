package model

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Route default tuning, mirroring the previous handler-layer constants.
const (
	defaultRoutePriority int32 = 100
	defaultRouteWeight   int32 = 100
)

// RouteRepository is the persistence port for model-route management. Delete
// has no sqlc query (the handler used direct SQL), so the adapter implements it
// with a pool.Exec.
type RouteRepository interface {
	Create(ctx context.Context, w RouteWrite) (domain.ModelRoute, error)
	List(ctx context.Context, modelID string) ([]domain.ModelRouteListItem, error)
	Get(ctx context.Context, routeID string) (domain.ModelRoute, error)
	Update(ctx context.Context, w RouteUpdate) (domain.ModelRoute, error)
	UpdateStatus(ctx context.Context, modelID, routeID, status string) (domain.ModelRoute, error)
	// Delete removes a route scoped to its model, returning domain.ErrNotFound
	// when no row matched.
	Delete(ctx context.Context, modelID, routeID string) error
}

// RouteWrite is the persistence payload for creating a route.
type RouteWrite struct {
	ModelID              string
	UpstreamDeploymentID string
	Priority             int32
	Weight               int32
	SupportsStream       bool
	Status               string
}

// RouteUpdate is the persistence payload for updating a route.
type RouteUpdate struct {
	ModelID              string
	RouteID              string
	UpstreamDeploymentID string
	Priority             int32
	Weight               int32
	SupportsStream       bool
	Status               string
}

// RouteService implements model-route management business logic.
type RouteService struct {
	repo RouteRepository
}

func NewRouteService(repo RouteRepository) *RouteService {
	return &RouteService{repo: repo}
}

// RouteInput is the decoded create/update route request. Priority/Weight nil
// fall back to the defaults; SupportsStream nil defaults to true; an empty
// Status defaults to active.
type RouteInput struct {
	ModelID              string
	RouteID              string // set for Update only
	UpstreamDeploymentID string
	Priority             *int32
	Weight               *int32
	SupportsStream       *bool
	Status               string
}

func (s *RouteService) Create(ctx context.Context, in RouteInput) (domain.ModelRoute, error) {
	if in.UpstreamDeploymentID == "" {
		return domain.ModelRoute{}, domain.NewValidationError("upstream_deployment_id", "upstream_deployment_id is required")
	}
	return s.repo.Create(ctx, RouteWrite{
		ModelID:              in.ModelID,
		UpstreamDeploymentID: in.UpstreamDeploymentID,
		Priority:             int32OrDefault(in.Priority, defaultRoutePriority),
		Weight:               int32OrDefault(in.Weight, defaultRouteWeight),
		SupportsStream:       boolOrDefault(in.SupportsStream, true),
		Status:               routeStatusOrDefault(in.Status),
	})
}

func (s *RouteService) List(ctx context.Context, modelID string) ([]domain.ModelRouteListItem, error) {
	return s.repo.List(ctx, modelID)
}

func (s *RouteService) Get(ctx context.Context, routeID string) (domain.ModelRoute, error) {
	return s.repo.Get(ctx, routeID)
}

func (s *RouteService) Update(ctx context.Context, in RouteInput) (domain.ModelRoute, error) {
	if in.UpstreamDeploymentID == "" {
		return domain.ModelRoute{}, domain.NewValidationError("upstream_deployment_id", "upstream_deployment_id is required")
	}
	return s.repo.Update(ctx, RouteUpdate{
		ModelID:              in.ModelID,
		RouteID:              in.RouteID,
		UpstreamDeploymentID: in.UpstreamDeploymentID,
		Priority:             int32OrDefault(in.Priority, defaultRoutePriority),
		Weight:               int32OrDefault(in.Weight, defaultRouteWeight),
		SupportsStream:       boolOrDefault(in.SupportsStream, true),
		Status:               routeStatusOrDefault(in.Status),
	})
}

func (s *RouteService) UpdateStatus(ctx context.Context, modelID, routeID, status string) (domain.ModelRoute, error) {
	if status == "" {
		return domain.ModelRoute{}, domain.NewValidationError("status", "status is required")
	}
	return s.repo.UpdateStatus(ctx, modelID, routeID, status)
}

func (s *RouteService) Delete(ctx context.Context, modelID, routeID string) error {
	return s.repo.Delete(ctx, modelID, routeID)
}

func int32OrDefault(v *int32, def int32) int32 {
	if v == nil {
		return def
	}
	return *v
}

func boolOrDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func routeStatusOrDefault(status string) string {
	if status == "" {
		return domain.APIKeyStatusActive
	}
	return status
}

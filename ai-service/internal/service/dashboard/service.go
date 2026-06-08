package dashboard

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Top-widget limit bounds, mirroring the previous handler-layer behaviour
// (default 10, cap 100).
const (
	defaultTopLimit int32 = 10
	maxTopLimit     int32 = 100
)

// Service implements dashboard read business logic.
type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// Summary returns the headline panel for the given scope.
func (s *Service) Summary(ctx context.Context, f domain.DashboardFilter) (domain.DashboardSummary, error) {
	return s.repo.Summary(ctx, f)
}

// TopModels returns the top models widget, with the limit clamped to (0, 100]
// (default 10).
func (s *Service) TopModels(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error) {
	return s.repo.TopModels(ctx, f, clampTopLimit(limit))
}

// TopTenants returns the top tenants widget, with the limit clamped.
func (s *Service) TopTenants(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error) {
	return s.repo.TopTenants(ctx, f, clampTopLimit(limit))
}

// RecentErrors returns the recent-errors widget, with the limit clamped.
func (s *Service) RecentErrors(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error) {
	return s.repo.RecentErrors(ctx, f, clampTopLimit(limit))
}

func clampTopLimit(limit int32) int32 {
	if limit <= 0 {
		return defaultTopLimit
	}
	if limit > maxTopLimit {
		return maxTopLimit
	}
	return limit
}

package observabilitycontrol

import (
	"context"

	"xiaodou/dai/internal/ai/domain"
)

const (
	defaultTopLimit int32 = 10
	maxTopLimit     int32 = 100
)

type DashboardRepository interface {
	Summary(ctx context.Context, f domain.DashboardFilter) (domain.DashboardSummary, error)
	TopModels(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error)
	TopTenants(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error)
	RecentErrors(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error)
}

type DashboardService struct {
	repo DashboardRepository
}

func NewDashboardService(repo DashboardRepository) *DashboardService {
	return &DashboardService{repo: repo}
}

func (s *DashboardService) Summary(ctx context.Context, f domain.DashboardFilter) (domain.DashboardSummary, error) {
	return s.repo.Summary(ctx, f)
}

func (s *DashboardService) TopModels(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error) {
	return s.repo.TopModels(ctx, f, clampTopLimit(limit))
}

func (s *DashboardService) TopTenants(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error) {
	return s.repo.TopTenants(ctx, f, clampTopLimit(limit))
}

func (s *DashboardService) RecentErrors(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error) {
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

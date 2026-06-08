// Package dashboard holds the business logic for the analytics dashboard read
// APIs (the console management plane). It is read-only. Service owns the
// top-widget limit normalization; persistence is reached through Repository,
// defined on the consumer side.
package dashboard

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the dashboard service. All
// methods are read-only.
type Repository interface {
	Summary(ctx context.Context, f domain.DashboardFilter) (domain.DashboardSummary, error)
	TopModels(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error)
	TopTenants(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error)
	RecentErrors(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error)
}

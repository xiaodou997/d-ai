// Package usage holds the business logic for usage-log and usage-analytics
// read APIs (the console management plane). It is read-only. Service owns
// pagination/limit normalization and filter pass-through; persistence is
// reached through Repository, defined on the consumer side.
package usage

import (
	"context"
	"time"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the usage service. All methods
// are read-only.
type Repository interface {
	// CountLogs returns the total rows matching the filter (ignores paging).
	CountLogs(ctx context.Context, f domain.UsageFilter) (int64, error)
	// StatsFor returns the aggregate panel computed over the filter.
	StatsFor(ctx context.Context, f domain.UsageFilter) (domain.UsageStats, error)
	// ListLogs returns one page of logs (newest first).
	ListLogs(ctx context.Context, f domain.UsageFilter, limit, offset int32) ([]domain.UsageLog, error)
	// Summary aggregates per model over the SummaryFilter (which uses an
	// optional Since rather than a date range).
	Summary(ctx context.Context, f SummaryFilter) ([]domain.UsageSummaryRow, error)
	UnitSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUnitSummaryRow, error)
	// UserSummary is the single-row summary for one tenant user.
	UserSummary(ctx context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error)
	// DailyTrend returns per-day rollups for the last n days.
	DailyTrend(ctx context.Context, days int) ([]domain.DailyTrendRow, error)
}

// SummaryFilter scopes the per-model / per-unit summary queries. Unlike the
// log-list filter, these queries take an optional Since instead of a date
// range, and every scope field is optional.
type SummaryFilter struct {
	TenantID      string
	UserID        string
	ModelCode     string
	RequestStatus string
	RequestSource string
	Since         *time.Time
}

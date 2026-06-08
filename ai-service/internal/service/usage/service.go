package usage

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Pagination bounds, mirroring the previous handler-layer behaviour.
const (
	defaultLogLimit  int32 = 20
	maxLogLimit      int32 = 100
	maxTrendDays           = 365
	defaultTrendDays       = 30
)

// Service implements usage read business logic.
type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// LogPage bundles a page of logs with the filter-wide aggregate stats and total.
type LogPage struct {
	Total   int64
	Stats   domain.UsageStats
	Records []domain.UsageLog
}

// ListLogs normalizes paging (limit<=0→20, >100→100, offset<0→0), then fetches
// count + stats + the page in one call. The same method backs both the admin
// and tenant log views; the HTTP layer projects domain.UsageLog into the
// appropriate DTO.
func (s *Service) ListLogs(ctx context.Context, f domain.UsageFilter, limit, offset int32) (LogPage, error) {
	if limit <= 0 {
		limit = defaultLogLimit
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}
	if offset < 0 {
		offset = 0
	}
	total, err := s.repo.CountLogs(ctx, f)
	if err != nil {
		return LogPage{}, err
	}
	stats, err := s.repo.StatsFor(ctx, f)
	if err != nil {
		return LogPage{}, err
	}
	records, err := s.repo.ListLogs(ctx, f, limit, offset)
	if err != nil {
		return LogPage{}, err
	}
	return LogPage{Total: total, Stats: stats, Records: records}, nil
}

// Summary returns per-model aggregates.
func (s *Service) Summary(ctx context.Context, f SummaryFilter) ([]domain.UsageSummaryRow, error) {
	return s.repo.Summary(ctx, f)
}

// UnitSummary returns per-billable-unit aggregates.
func (s *Service) UnitSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUnitSummaryRow, error) {
	return s.repo.UnitSummary(ctx, f)
}

// UserSummary returns the single-row summary for one tenant user.
func (s *Service) UserSummary(ctx context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error) {
	return s.repo.UserSummary(ctx, tenantID, userID, requestSource)
}

// DailyTrend clamps days to (0, 365] (default 30) and returns per-day rollups.
func (s *Service) DailyTrend(ctx context.Context, days int) ([]domain.DailyTrendRow, error) {
	if days <= 0 || days > maxTrendDays {
		days = defaultTrendDays
	}
	return s.repo.DailyTrend(ctx, days)
}

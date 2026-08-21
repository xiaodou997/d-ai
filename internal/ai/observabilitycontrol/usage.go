package observabilitycontrol

import (
	"context"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

const (
	defaultLogLimit     int32 = 20
	maxLogLimit         int32 = 100
	defaultRankingLimit int32 = 50
	maxRankingLimit     int32 = 100
)

type UsageRepository interface {
	CountLogs(ctx context.Context, f domain.UsageFilter) (int64, error)
	StatsFor(ctx context.Context, f domain.UsageFilter) (domain.UsageStats, error)
	ListLogs(ctx context.Context, f domain.UsageFilter, limit, offset int32) ([]domain.UsageLog, error)
	ListUserLogs(ctx context.Context, tenantID, userID, requestSource string, limit int32) ([]domain.UsageLog, error)
	GetLogDetail(ctx context.Context, requestID string) (domain.UsageLogDetail, error)
	Summary(ctx context.Context, f SummaryFilter) ([]domain.UsageSummaryRow, error)
	UnitSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUnitSummaryRow, error)
	UpstreamSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUpstreamSummaryRow, error)
	UserRanking(ctx context.Context, f SummaryFilter, limit int32) ([]domain.UsageUserRankingRow, error)
	UserSummary(ctx context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error)
	DailyTrend(ctx context.Context, dateFrom, dateTo *time.Time) ([]domain.DailyTrendRow, error)
}

type SummaryFilter struct {
	TenantID      string
	UserID        string
	ModelCode     string
	RequestStatus string
	RequestSource string
	DateFrom      *time.Time
	DateTo        *time.Time
}

type LogPage struct {
	Total   int64
	Stats   domain.UsageStats
	Records []domain.UsageLog
}

type UsageService struct {
	repo UsageRepository
}

func NewUsageService(repo UsageRepository) *UsageService {
	return &UsageService{repo: repo}
}

func (s *UsageService) ListLogs(ctx context.Context, f domain.UsageFilter, limit, offset int32) (LogPage, error) {
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

func (s *UsageService) GetLogDetail(ctx context.Context, requestID string) (domain.UsageLogDetail, error) {
	return s.repo.GetLogDetail(ctx, requestID)
}

func (s *UsageService) ListUserLogs(ctx context.Context, tenantID, userID, requestSource string, limit int32) ([]domain.UsageLog, error) {
	return s.repo.ListUserLogs(ctx, tenantID, userID, requestSource, limit)
}

func (s *UsageService) Summary(ctx context.Context, f SummaryFilter) ([]domain.UsageSummaryRow, error) {
	return s.repo.Summary(ctx, f)
}

func (s *UsageService) UnitSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUnitSummaryRow, error) {
	return s.repo.UnitSummary(ctx, f)
}

func (s *UsageService) UpstreamSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUpstreamSummaryRow, error) {
	return s.repo.UpstreamSummary(ctx, f)
}

func (s *UsageService) UserRanking(ctx context.Context, f SummaryFilter, limit int32) ([]domain.UsageUserRankingRow, error) {
	if limit <= 0 {
		limit = defaultRankingLimit
	}
	if limit > maxRankingLimit {
		limit = maxRankingLimit
	}
	return s.repo.UserRanking(ctx, f, limit)
}

func (s *UsageService) UserSummary(ctx context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error) {
	return s.repo.UserSummary(ctx, tenantID, userID, requestSource)
}

func (s *UsageService) DailyTrend(ctx context.Context, dateFrom, dateTo *time.Time) ([]domain.DailyTrendRow, error) {
	return s.repo.DailyTrend(ctx, dateFrom, dateTo)
}

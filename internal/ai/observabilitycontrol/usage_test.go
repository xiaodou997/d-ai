package observabilitycontrol

import (
	"context"
	"errors"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

type usageRepoStub struct {
	gotLimit     int32
	gotOffset    int32
	gotTenantID  string
	gotUserID    string
	gotSource    string
	gotDateFrom  *time.Time
	gotDateTo    *time.Time
	total        int64
	stats        domain.UsageStats
	logs         []domain.UsageLog
	rankingRows  []domain.UsageUserRankingRow
	upstreamRows []domain.UsageUpstreamSummaryRow
	countErr     error
	statsErr     error
	listErr      error
	err          error
}

func (m *usageRepoStub) CountLogs(ctx context.Context, f domain.UsageFilter) (int64, error) {
	return m.total, m.countErr
}
func (m *usageRepoStub) StatsFor(ctx context.Context, f domain.UsageFilter) (domain.UsageStats, error) {
	return m.stats, m.statsErr
}
func (m *usageRepoStub) ListLogs(ctx context.Context, f domain.UsageFilter, limit, offset int32) ([]domain.UsageLog, error) {
	m.gotLimit, m.gotOffset = limit, offset
	return m.logs, m.listErr
}
func (m *usageRepoStub) ListUserLogs(ctx context.Context, tenantID, userID, requestSource string, limit int32) ([]domain.UsageLog, error) {
	m.gotTenantID, m.gotUserID, m.gotSource, m.gotLimit = tenantID, userID, requestSource, limit
	return m.logs, m.listErr
}
func (m *usageRepoStub) GetLogDetail(ctx context.Context, requestID string) (domain.UsageLogDetail, error) {
	return domain.UsageLogDetail{}, m.err
}
func (m *usageRepoStub) Summary(ctx context.Context, f SummaryFilter) ([]domain.UsageSummaryRow, error) {
	return nil, m.err
}
func (m *usageRepoStub) UnitSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUnitSummaryRow, error) {
	return nil, m.err
}
func (m *usageRepoStub) UpstreamSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUpstreamSummaryRow, error) {
	return m.upstreamRows, m.err
}
func (m *usageRepoStub) UserRanking(ctx context.Context, f SummaryFilter, limit int32) ([]domain.UsageUserRankingRow, error) {
	m.gotLimit = limit
	return m.rankingRows, m.err
}
func (m *usageRepoStub) UserSummary(ctx context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error) {
	return domain.UserUsageSummary{}, m.err
}
func (m *usageRepoStub) DailyTrend(ctx context.Context, dateFrom, dateTo *time.Time) ([]domain.DailyTrendRow, error) {
	m.gotDateFrom = dateFrom
	m.gotDateTo = dateTo
	return nil, m.err
}

func TestUsageListLogsDefaultsAndCaps(t *testing.T) {
	repo := &usageRepoStub{}
	svc := NewUsageService(repo)
	if _, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 0, -5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultLogLimit {
		t.Fatalf("want default limit %d, got %d", defaultLogLimit, repo.gotLimit)
	}
	if repo.gotOffset != 0 {
		t.Fatalf("want offset 0, got %d", repo.gotOffset)
	}

	if _, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 9999, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != maxLogLimit {
		t.Fatalf("want capped limit %d, got %d", maxLogLimit, repo.gotLimit)
	}
	if repo.gotOffset != 10 {
		t.Fatalf("want offset 10, got %d", repo.gotOffset)
	}
}

func TestUsageListLogsAssemblesPage(t *testing.T) {
	repo := &usageRepoStub{
		total: 42,
		stats: domain.UsageStats{TotalRequests: 42, TotalUserChargedMicro: 1000},
		logs:  []domain.UsageLog{{ID: "l1"}},
	}
	svc := NewUsageService(repo)
	page, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 42 || page.Stats.TotalUserChargedMicro != 1000 || len(page.Records) != 1 {
		t.Fatalf("page not assembled: %+v", page)
	}
}

func TestUsageListLogsCountErrorPropagates(t *testing.T) {
	svc := NewUsageService(&usageRepoStub{countErr: errors.New("boom")})
	if _, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 20, 0); err == nil {
		t.Fatal("want count error")
	}
}

func TestUsageListLogsStatsErrorPropagates(t *testing.T) {
	svc := NewUsageService(&usageRepoStub{statsErr: errors.New("boom")})
	if _, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 20, 0); err == nil {
		t.Fatal("want stats error")
	}
}

func TestUsageListLogsListErrorPropagates(t *testing.T) {
	svc := NewUsageService(&usageRepoStub{listErr: errors.New("boom")})
	if _, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 20, 0); err == nil {
		t.Fatal("want list error")
	}
}

func TestUsageListUserLogsPassThroughsScope(t *testing.T) {
	repo := &usageRepoStub{logs: []domain.UsageLog{{ID: "log-1"}}}
	svc := NewUsageService(repo)

	logs, err := svc.ListUserLogs(context.Background(), "tenant-1", "user-1", "workspace", 37)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotTenantID != "tenant-1" || repo.gotUserID != "user-1" || repo.gotSource != "workspace" || repo.gotLimit != 37 {
		t.Fatalf("scope = tenant %q user %q source %q limit %d", repo.gotTenantID, repo.gotUserID, repo.gotSource, repo.gotLimit)
	}
	if len(logs) != 1 || logs[0].ID != "log-1" {
		t.Fatalf("logs = %+v, want one log", logs)
	}
}

func TestUsageDailyTrendPassThroughsWindow(t *testing.T) {
	repo := &usageRepoStub{}
	svc := NewUsageService(repo)
	startAt := time.Unix(100, 0).UTC()
	endAt := time.Unix(200, 0).UTC()
	if _, err := svc.DailyTrend(context.Background(), &startAt, &endAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotDateFrom != &startAt || repo.gotDateTo != &endAt {
		t.Fatalf("want exact window pointers passed through, got %p %p", repo.gotDateFrom, repo.gotDateTo)
	}
}

func TestUsageSummaryPassThroughs(t *testing.T) {
	svc := NewUsageService(&usageRepoStub{})
	if _, err := svc.Summary(context.Background(), SummaryFilter{}); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if _, err := svc.UnitSummary(context.Background(), SummaryFilter{}); err != nil {
		t.Fatalf("UnitSummary: %v", err)
	}
	if _, err := svc.UserSummary(context.Background(), "t1", "u1", ""); err != nil {
		t.Fatalf("UserSummary: %v", err)
	}
}

func TestUsageUserRankingDefaultsAndCaps(t *testing.T) {
	repo := &usageRepoStub{}
	svc := NewUsageService(repo)
	if _, err := svc.UserRanking(context.Background(), SummaryFilter{}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultRankingLimit {
		t.Fatalf("want default ranking limit %d, got %d", defaultRankingLimit, repo.gotLimit)
	}

	if _, err := svc.UserRanking(context.Background(), SummaryFilter{}, 9999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != maxRankingLimit {
		t.Fatalf("want capped ranking limit %d, got %d", maxRankingLimit, repo.gotLimit)
	}
}

func TestUsageUserRankingPassThroughsRows(t *testing.T) {
	repo := &usageRepoStub{
		rankingRows: []domain.UsageUserRankingRow{{TenantID: "t1", UserID: "u1", RequestCount: 3}},
	}
	svc := NewUsageService(repo)
	rows, err := svc.UserRanking(context.Background(), SummaryFilter{}, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || rows[0].UserID != "u1" {
		t.Fatalf("rows = %+v, want one ranking row", rows)
	}
}

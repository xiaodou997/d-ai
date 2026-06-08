package usage

import (
	"context"
	"errors"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

type mockRepo struct {
	gotLimit  int32
	gotOffset int32
	gotDays   int
	total     int64
	stats     domain.UsageStats
	logs      []domain.UsageLog
	countErr  error
	statsErr  error
	listErr   error
	err       error
}

func (m *mockRepo) CountLogs(ctx context.Context, f domain.UsageFilter) (int64, error) {
	return m.total, m.countErr
}
func (m *mockRepo) StatsFor(ctx context.Context, f domain.UsageFilter) (domain.UsageStats, error) {
	return m.stats, m.statsErr
}
func (m *mockRepo) ListLogs(ctx context.Context, f domain.UsageFilter, limit, offset int32) ([]domain.UsageLog, error) {
	m.gotLimit, m.gotOffset = limit, offset
	return m.logs, m.listErr
}
func (m *mockRepo) Summary(ctx context.Context, f SummaryFilter) ([]domain.UsageSummaryRow, error) {
	return nil, m.err
}
func (m *mockRepo) UnitSummary(ctx context.Context, f SummaryFilter) ([]domain.UsageUnitSummaryRow, error) {
	return nil, m.err
}
func (m *mockRepo) UserSummary(ctx context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error) {
	return domain.UserUsageSummary{}, m.err
}
func (m *mockRepo) DailyTrend(ctx context.Context, days int) ([]domain.DailyTrendRow, error) {
	m.gotDays = days
	return nil, m.err
}

func TestListLogs_DefaultsAndCaps(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
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

func TestListLogs_AssemblesPage(t *testing.T) {
	repo := &mockRepo{
		total: 42,
		stats: domain.UsageStats{TotalRequests: 42, TotalCostMicro: 1000},
		logs:  []domain.UsageLog{{ID: "l1"}},
	}
	svc := New(repo)
	page, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 42 || page.Stats.TotalCostMicro != 1000 || len(page.Records) != 1 {
		t.Fatalf("page not assembled: %+v", page)
	}
}

func TestListLogs_CountErrorPropagates(t *testing.T) {
	svc := New(&mockRepo{countErr: errors.New("boom")})
	if _, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 20, 0); err == nil {
		t.Fatal("want count error")
	}
}

func TestListLogs_StatsErrorPropagates(t *testing.T) {
	svc := New(&mockRepo{statsErr: errors.New("boom")})
	if _, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 20, 0); err == nil {
		t.Fatal("want stats error")
	}
}

func TestListLogs_ListErrorPropagates(t *testing.T) {
	svc := New(&mockRepo{listErr: errors.New("boom")})
	if _, err := svc.ListLogs(context.Background(), domain.UsageFilter{}, 20, 0); err == nil {
		t.Fatal("want list error")
	}
}

func TestDailyTrend_ClampsDays(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.DailyTrend(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotDays != defaultTrendDays {
		t.Fatalf("want default days %d, got %d", defaultTrendDays, repo.gotDays)
	}
	if _, err := svc.DailyTrend(context.Background(), 9999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotDays != defaultTrendDays {
		t.Fatalf("want clamped to default %d, got %d", defaultTrendDays, repo.gotDays)
	}
	if _, err := svc.DailyTrend(context.Background(), 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotDays != 7 {
		t.Fatalf("want days 7, got %d", repo.gotDays)
	}
}

func TestSummaryPassThroughs(t *testing.T) {
	svc := New(&mockRepo{})
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

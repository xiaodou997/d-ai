package observabilitycontrol

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type dashboardRepoStub struct {
	gotLimit int32
	summary  domain.DashboardSummary
	err      error
}

func (m *dashboardRepoStub) Summary(ctx context.Context, f domain.DashboardFilter) (domain.DashboardSummary, error) {
	return m.summary, m.err
}
func (m *dashboardRepoStub) TopModels(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error) {
	m.gotLimit = limit
	return nil, m.err
}
func (m *dashboardRepoStub) TopTenants(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error) {
	m.gotLimit = limit
	return nil, m.err
}
func (m *dashboardRepoStub) RecentErrors(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error) {
	m.gotLimit = limit
	return nil, m.err
}

func TestDashboardSummaryPassThrough(t *testing.T) {
	repo := &dashboardRepoStub{summary: domain.DashboardSummary{TotalRequests: 7}}
	svc := NewDashboardService(repo)
	s, err := svc.Summary(context.Background(), domain.DashboardFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.TotalRequests != 7 {
		t.Fatalf("want 7, got %d", s.TotalRequests)
	}
}

func TestDashboardSummaryErrorPropagates(t *testing.T) {
	svc := NewDashboardService(&dashboardRepoStub{err: errors.New("boom")})
	if _, err := svc.Summary(context.Background(), domain.DashboardFilter{}); err == nil {
		t.Fatal("want error")
	}
}

func TestDashboardTopModelsClampsLimit(t *testing.T) {
	repo := &dashboardRepoStub{}
	svc := NewDashboardService(repo)
	if _, err := svc.TopModels(context.Background(), domain.DashboardFilter{}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultTopLimit {
		t.Fatalf("want default %d, got %d", defaultTopLimit, repo.gotLimit)
	}
	if _, err := svc.TopModels(context.Background(), domain.DashboardFilter{}, 9999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != maxTopLimit {
		t.Fatalf("want capped %d, got %d", maxTopLimit, repo.gotLimit)
	}
	if _, err := svc.TopModels(context.Background(), domain.DashboardFilter{}, 25); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != 25 {
		t.Fatalf("want 25, got %d", repo.gotLimit)
	}
}

func TestDashboardTopTenantsClampsLimit(t *testing.T) {
	repo := &dashboardRepoStub{}
	svc := NewDashboardService(repo)
	if _, err := svc.TopTenants(context.Background(), domain.DashboardFilter{}, -1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultTopLimit {
		t.Fatalf("want default %d, got %d", defaultTopLimit, repo.gotLimit)
	}
}

func TestDashboardRecentErrorsClampsLimit(t *testing.T) {
	repo := &dashboardRepoStub{}
	svc := NewDashboardService(repo)
	if _, err := svc.RecentErrors(context.Background(), domain.DashboardFilter{}, 500); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != maxTopLimit {
		t.Fatalf("want capped %d, got %d", maxTopLimit, repo.gotLimit)
	}
}

func TestDashboardRecentErrorsErrorPropagates(t *testing.T) {
	svc := NewDashboardService(&dashboardRepoStub{err: errors.New("boom")})
	if _, err := svc.RecentErrors(context.Background(), domain.DashboardFilter{}, 10); err == nil {
		t.Fatal("want error")
	}
}

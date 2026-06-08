package dashboard

import (
	"context"
	"errors"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

type mockRepo struct {
	gotLimit int32
	summary  domain.DashboardSummary
	err      error
}

func (m *mockRepo) Summary(ctx context.Context, f domain.DashboardFilter) (domain.DashboardSummary, error) {
	return m.summary, m.err
}
func (m *mockRepo) TopModels(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error) {
	m.gotLimit = limit
	return nil, m.err
}
func (m *mockRepo) TopTenants(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error) {
	m.gotLimit = limit
	return nil, m.err
}
func (m *mockRepo) RecentErrors(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error) {
	m.gotLimit = limit
	return nil, m.err
}

func TestSummary_PassThrough(t *testing.T) {
	repo := &mockRepo{summary: domain.DashboardSummary{TotalRequests: 7}}
	svc := New(repo)
	s, err := svc.Summary(context.Background(), domain.DashboardFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.TotalRequests != 7 {
		t.Fatalf("want 7, got %d", s.TotalRequests)
	}
}

func TestSummary_ErrorPropagates(t *testing.T) {
	svc := New(&mockRepo{err: errors.New("boom")})
	if _, err := svc.Summary(context.Background(), domain.DashboardFilter{}); err == nil {
		t.Fatal("want error")
	}
}

func TestTopModels_ClampsLimit(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
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

func TestTopTenants_ClampsLimit(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.TopTenants(context.Background(), domain.DashboardFilter{}, -1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultTopLimit {
		t.Fatalf("want default %d, got %d", defaultTopLimit, repo.gotLimit)
	}
}

func TestRecentErrors_ClampsLimit(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.RecentErrors(context.Background(), domain.DashboardFilter{}, 500); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != maxTopLimit {
		t.Fatalf("want capped %d, got %d", maxTopLimit, repo.gotLimit)
	}
}

func TestRecentErrors_ErrorPropagates(t *testing.T) {
	svc := New(&mockRepo{err: errors.New("boom")})
	if _, err := svc.RecentErrors(context.Background(), domain.DashboardFilter{}, 10); err == nil {
		t.Fatal("want error")
	}
}

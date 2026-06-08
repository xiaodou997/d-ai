package audit

import (
	"context"
	"errors"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

type mockRepo struct {
	gotLimit int32
	items    []domain.AuditLog
	err      error
}

func (m *mockRepo) List(ctx context.Context, limit int32) ([]domain.AuditLog, error) {
	m.gotLimit = limit
	return m.items, m.err
}

func TestList_DefaultsLimitWhenZero(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.List(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultLimit {
		t.Fatalf("want default limit %d, got %d", defaultLimit, repo.gotLimit)
	}
}

func TestList_DefaultsLimitWhenNegative(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.List(context.Background(), -3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultLimit {
		t.Fatalf("want default limit %d, got %d", defaultLimit, repo.gotLimit)
	}
}

func TestList_CapsLimitAtMax(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.List(context.Background(), 9999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != maxLimit {
		t.Fatalf("want capped limit %d, got %d", maxLimit, repo.gotLimit)
	}
}

func TestList_PassesValidLimitThrough(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.List(context.Background(), 25); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != 25 {
		t.Fatalf("want limit 25, got %d", repo.gotLimit)
	}
}

func TestList_PropagatesError(t *testing.T) {
	svc := New(&mockRepo{err: errors.New("boom")})
	if _, err := svc.List(context.Background(), 10); err == nil {
		t.Fatal("want repo error")
	}
}

func TestList_ReturnsItems(t *testing.T) {
	repo := &mockRepo{items: []domain.AuditLog{{ID: "a1", Action: "create"}}}
	svc := New(repo)
	items, err := svc.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "a1" {
		t.Fatalf("items not returned: %+v", items)
	}
}

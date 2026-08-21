package observabilitycontrol

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type auditRepoStub struct {
	gotLimit int32
	gotEvent domain.AdminAuditEvent
	items    []domain.AuditLog
	err      error
}

func (m *auditRepoStub) List(ctx context.Context, limit int32) ([]domain.AuditLog, error) {
	m.gotLimit = limit
	return m.items, m.err
}

func (m *auditRepoStub) Record(ctx context.Context, event domain.AdminAuditEvent) error {
	m.gotEvent = event
	return m.err
}

func TestAuditListDefaultsLimitWhenZero(t *testing.T) {
	repo := &auditRepoStub{}
	svc := NewAuditService(repo)
	if _, err := svc.List(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultAuditLimit {
		t.Fatalf("want default limit %d, got %d", defaultAuditLimit, repo.gotLimit)
	}
}

func TestAuditListDefaultsLimitWhenNegative(t *testing.T) {
	repo := &auditRepoStub{}
	svc := NewAuditService(repo)
	if _, err := svc.List(context.Background(), -3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != defaultAuditLimit {
		t.Fatalf("want default limit %d, got %d", defaultAuditLimit, repo.gotLimit)
	}
}

func TestAuditListCapsLimitAtMax(t *testing.T) {
	repo := &auditRepoStub{}
	svc := NewAuditService(repo)
	if _, err := svc.List(context.Background(), 9999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != maxAuditLimit {
		t.Fatalf("want capped limit %d, got %d", maxAuditLimit, repo.gotLimit)
	}
}

func TestAuditListPassesValidLimitThrough(t *testing.T) {
	repo := &auditRepoStub{}
	svc := NewAuditService(repo)
	if _, err := svc.List(context.Background(), 25); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotLimit != 25 {
		t.Fatalf("want limit 25, got %d", repo.gotLimit)
	}
}

func TestAuditListPropagatesError(t *testing.T) {
	svc := NewAuditService(&auditRepoStub{err: errors.New("boom")})
	if _, err := svc.List(context.Background(), 10); err == nil {
		t.Fatal("want repo error")
	}
}

func TestAuditListReturnsItems(t *testing.T) {
	repo := &auditRepoStub{items: []domain.AuditLog{{ID: "a1", Action: "create"}}}
	svc := NewAuditService(repo)
	items, err := svc.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "a1" {
		t.Fatalf("items not returned: %+v", items)
	}
}

func TestAuditRecordPassesEventThrough(t *testing.T) {
	status := int32(201)
	repo := &auditRepoStub{}
	svc := NewAuditService(repo)
	event := domain.AdminAuditEvent{
		Actor:          "admin-1",
		Action:         "groups.import",
		ObjectType:     "group_config_bundle",
		ObjectID:       "bundle-1",
		RequestSummary: []byte(`{"group_count":2}`),
		Result:         "success",
		HttpStatus:     &status,
	}

	if err := svc.Record(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotEvent.Actor != event.Actor || repo.gotEvent.Action != event.Action || repo.gotEvent.ObjectID != event.ObjectID {
		t.Fatalf("event not preserved: %+v", repo.gotEvent)
	}
	if repo.gotEvent.HttpStatus == nil || *repo.gotEvent.HttpStatus != status || string(repo.gotEvent.RequestSummary) != string(event.RequestSummary) {
		t.Fatalf("event payload not preserved: %+v", repo.gotEvent)
	}
}

func TestAuditRecordPropagatesError(t *testing.T) {
	svc := NewAuditService(&auditRepoStub{err: errors.New("boom")})
	if err := svc.Record(context.Background(), domain.AdminAuditEvent{Action: "groups.import"}); err == nil {
		t.Fatal("want repo error")
	}
}

package transport

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/auth"
)

type adminAuditRecorderStub struct {
	events []domain.AdminAuditEvent
	err    error
}

func (s *adminAuditRecorderStub) Record(_ context.Context, event domain.AdminAuditEvent) error {
	s.events = append(s.events, event)
	return s.err
}

func TestVoidAdminAuditBuildsDomainEvent(t *testing.T) {
	t.Parallel()

	recorder := &adminAuditRecorderStub{}
	ctx := context.WithValue(context.Background(), authClaimsContextKey{}, &auth.Claims{UserID: "admin-1"})
	voidAdminAudit(ctx, AIDeps{OperationsDeps: OperationsDeps{AdminAudit: recorder}}, "groups.import", "group_config_bundle", "bundle-1", map[string]any{
		"group_count": 2,
	}, "success", 201)

	if len(recorder.events) != 1 {
		t.Fatalf("events len = %d, want 1", len(recorder.events))
	}
	got := recorder.events[0]
	if got.Actor != "admin-1" || got.Action != "groups.import" || got.ObjectType != "group_config_bundle" || got.ObjectID != "bundle-1" || got.Result != "success" {
		t.Fatalf("event fields = %+v", got)
	}
	if got.HttpStatus == nil || *got.HttpStatus != 201 {
		t.Fatalf("http status = %v", got.HttpStatus)
	}
	var summary map[string]int
	if err := json.Unmarshal(got.RequestSummary, &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if summary["group_count"] != 2 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestVoidAdminAuditRemainsBestEffort(t *testing.T) {
	t.Parallel()

	recorder := &adminAuditRecorderStub{err: errors.New("database unavailable")}
	voidAdminAudit(context.Background(), AIDeps{OperationsDeps: OperationsDeps{AdminAudit: recorder}}, "accounts.export", "", "", map[string]any{
		"invalid": func() {},
	}, "failed", 0)

	if len(recorder.events) != 1 {
		t.Fatalf("events len = %d, want 1", len(recorder.events))
	}
	if string(recorder.events[0].RequestSummary) != "{}" || recorder.events[0].HttpStatus != nil {
		t.Fatalf("fallback event = %+v", recorder.events[0])
	}
}

func TestVoidAdminAuditSkipsMissingRecorder(t *testing.T) {
	t.Parallel()
	voidAdminAudit(context.Background(), AIDeps{}, "accounts.export", "", "", nil, "success", 200)
}

package console

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

func TestPortalTaskDTOEnforcesTenantReadOnlyUserTasks(t *testing.T) {
	now := time.Now()
	viewer := coreidentity.Subject{Scope: coreidentity.ScopeTenant, TenantID: "tenant-a"}
	view := asynctask.TaskView{
		ID: "task-user", Type: "app.images.generation", Status: domain.TaskRunning,
		Subject:   asynctask.SubjectRef{TenantID: "tenant-a", UserID: "user-a", InvokeKeyID: "invoke-key"},
		Output:    json.RawMessage(`{"data":[{"url":"https://example.com/one.png"}]}`),
		CreatedAt: now,
	}

	dto := portalTaskDTOFromView(view, viewer, true)
	if !dto.Permissions.ReadOnly || dto.Permissions.CanCancel || dto.Permissions.CanDelete {
		t.Fatalf("permissions = %+v, want read-only", dto.Permissions)
	}
	if dto.Result != nil {
		t.Fatalf("tenant received raw user result: %s", dto.Result)
	}
	if !dto.ResultAvailable || dto.ResultSummary == nil || dto.ResultSummary.ImageCount != 1 {
		t.Fatalf("result summary = %+v, available=%v", dto.ResultSummary, dto.ResultAvailable)
	}
	if dto.Owner.Scope != string(coreidentity.ScopeUser) || dto.Owner.UserID != "user-a" {
		t.Fatalf("owner = %+v", dto.Owner)
	}
}

func TestPortalTaskDTOAllowsOwnerToManageTerminalTask(t *testing.T) {
	now := time.Now()
	viewer := coreidentity.Subject{Scope: coreidentity.ScopeUser, TenantID: "tenant-a", UserID: "user-a"}
	view := asynctask.TaskView{
		ID: "task-user", Type: "api.chat.completions", Status: domain.TaskCompleted,
		Subject:      asynctask.SubjectRef{TenantID: "tenant-a", UserID: "user-a", APIKeyID: "api-key"},
		Output:       json.RawMessage(`{"choices":[{"finish_reason":"stop"}]}`),
		CallerCharge: 15_000, CreatedAt: now, CompletedAt: &now,
	}

	dto := portalTaskDTOFromView(view, viewer, true)
	if dto.Permissions.ReadOnly || dto.Permissions.CanCancel || !dto.Permissions.CanDelete {
		t.Fatalf("permissions = %+v, want terminal owner delete", dto.Permissions)
	}
	if dto.Result == nil || dto.ResultSummary == nil || dto.ResultSummary.ChoiceCount != 1 {
		t.Fatalf("result = %s summary=%+v", dto.Result, dto.ResultSummary)
	}
	if dto.Usage == nil || dto.Usage.CostCredits != 1.5 {
		t.Fatalf("usage = %+v", dto.Usage)
	}
}

func TestDecodePortalTaskListFilter(t *testing.T) {
	req := httptest.NewRequest("GET", "/runtime/v1/tasks?status=failed&type=images.generation&owner_scope=user&user_id=user-a&limit=50", nil)
	filter, err := decodePortalTaskListFilter(req)
	if err != nil {
		t.Fatalf("decode filter: %v", err)
	}
	if filter.Status != domain.TaskFailed || filter.OwnerScope != coreidentity.ScopeUser || filter.OwnerUserID != "user-a" || filter.Limit != 50 {
		t.Fatalf("filter = %+v", filter)
	}
	if len(filter.Types) != 3 {
		t.Fatalf("types = %v, want console/api/app image generation", filter.Types)
	}
}

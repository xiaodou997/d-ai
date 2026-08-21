package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/libs/go/server"
)

var _ AdminAuditLogReader = (*observabilitycontrol.AuditService)(nil)

type adminAuditLogReaderStub struct {
	limit int32
	logs  []domain.AuditLog
}

func (s *adminAuditLogReaderStub) List(_ context.Context, limit int32) ([]domain.AuditLog, error) {
	s.limit = limit
	return s.logs, nil
}

func TestAuditRouteUsesAdminAuditLogReader(t *testing.T) {
	status := int32(http.StatusCreated)
	createdAt := time.Date(2026, time.August, 21, 3, 4, 5, 0, time.UTC)
	reader := &adminAuditLogReaderStub{logs: []domain.AuditLog{{
		ID:             "audit-1",
		Actor:          "admin-1",
		Action:         "groups.import",
		ObjectType:     "group_config_bundle",
		ObjectID:       "bundle-1",
		RequestSummary: []byte(`{"groups":2}`),
		Result:         "success",
		HttpStatus:     &status,
		CreatedAt:      createdAt,
	}}}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerAudit(api, AIDeps{OperationsDeps: OperationsDeps{AuditLogs: reader}})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?limit=999", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if reader.limit != 500 {
		t.Fatalf("reader limit = %d, want 500", reader.limit)
	}
	var response struct {
		Items []auditLogDTO `json:"items"`
		Total int           `json:"total"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Total != 1 || len(response.Items) != 1 {
		t.Fatalf("response = %#v", response)
	}
	item := response.Items[0]
	if item.ID != "audit-1" || item.Actor == nil || *item.Actor != "admin-1" || item.HTTPStatus == nil || *item.HTTPStatus != status {
		t.Fatalf("item identity = %#v", item)
	}
	if item.CreatedAt == nil || *item.CreatedAt != createdAt.UnixMilli() || string(item.RequestSummary) != `{"groups":2}` {
		t.Fatalf("item projection = %#v", item)
	}
}

func TestAuditRouteRequiresAdminAuditLogReader(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerAudit(api, AIDeps{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

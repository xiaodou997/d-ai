package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

var _ GroupTransferManager = (*commercial.GroupTransferService)(nil)

type groupTransferManagerStub struct {
	exportTenantID  string
	exportGroupIDs  []string
	previewTenantID string
	previewRequest  commercial.GroupImportRequest
	importTenantID  string
	importRequest   commercial.GroupImportRequest
	bundle          commercial.GroupTransferBundle
	preview         commercial.GroupImportPreview
	result          commercial.GroupImportResult
}

func (s *groupTransferManagerStub) Export(_ context.Context, tenantID string, groupIDs []string) (commercial.GroupTransferBundle, error) {
	s.exportTenantID = tenantID
	s.exportGroupIDs = append([]string(nil), groupIDs...)
	return s.bundle, nil
}

func (s *groupTransferManagerStub) Preview(_ context.Context, tenantID string, request commercial.GroupImportRequest) (commercial.GroupImportPreview, error) {
	s.previewTenantID = tenantID
	s.previewRequest = request
	return s.preview, nil
}

func (s *groupTransferManagerStub) Import(_ context.Context, tenantID string, request commercial.GroupImportRequest) (commercial.GroupImportResult, error) {
	s.importTenantID = tenantID
	s.importRequest = request
	return s.result, nil
}

func TestGroupTransferRoutesUseManagerPort(t *testing.T) {
	manager := &groupTransferManagerStub{
		bundle: commercial.GroupTransferBundle{
			SchemaVersion: commercial.GroupTransferSchemaVersion,
			BundleID:      "export-bundle",
			ExportedAt:    "2026-08-21T00:00:00Z",
			Groups:        []commercial.GroupTransferGroup{{Name: "Exported group"}},
		},
		preview: commercial.GroupImportPreview{
			BundleID: "import-bundle",
			Summary:  commercial.GroupImportPreviewSummary{Create: 1},
			Warnings: []string{},
		},
		result: commercial.GroupImportResult{
			BundleID: "import-bundle",
			Summary:  commercial.GroupImportResultSummary{Success: 1},
		},
	}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerGroupTransfer(api, TenantGroupManagementHTTPDeps{GroupTransfer: manager})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), authClaimsContextKey{}, &auth.Claims{TenantID: "tenant-1", UserID: "user-1"})
		router.ServeHTTP(w, r.WithContext(ctx))
	})

	exportRecorder := performGroupTransferRequest(t, handler, http.MethodPost, "/api/v1/tenants/me/groups/export", `{"group_ids":["group-1","group-2"]}`)
	if exportRecorder.Code != http.StatusOK {
		t.Fatalf("export status = %d, body = %s", exportRecorder.Code, exportRecorder.Body.String())
	}
	if manager.exportTenantID != "tenant-1" || len(manager.exportGroupIDs) != 2 || manager.exportGroupIDs[1] != "group-2" {
		t.Fatalf("Export tenant = %q groups = %v", manager.exportTenantID, manager.exportGroupIDs)
	}
	var exported commercial.GroupTransferBundle
	if err := json.NewDecoder(exportRecorder.Body).Decode(&exported); err != nil {
		t.Fatalf("decode export response: %v", err)
	}
	if exported.BundleID != "export-bundle" || len(exported.Groups) != 1 {
		t.Fatalf("export response = %#v", exported)
	}

	requestBody := `{"bundle":{"schema_version":2,"bundle_id":"import-bundle","exported_at":"2026-08-21T00:00:00Z","groups":[]},"choices":[]}`
	previewRecorder := performGroupTransferRequest(t, handler, http.MethodPost, "/api/v1/tenants/me/groups/import/preview", requestBody)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", previewRecorder.Code, previewRecorder.Body.String())
	}
	if manager.previewTenantID != "tenant-1" || manager.previewRequest.Bundle.BundleID != "import-bundle" {
		t.Fatalf("Preview tenant = %q request = %#v", manager.previewTenantID, manager.previewRequest)
	}

	importRecorder := performGroupTransferRequest(t, handler, http.MethodPost, "/api/v1/tenants/me/groups/import", requestBody)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", importRecorder.Code, importRecorder.Body.String())
	}
	if manager.importTenantID != "tenant-1" || manager.importRequest.Bundle.BundleID != "import-bundle" {
		t.Fatalf("Import tenant = %q request = %#v", manager.importTenantID, manager.importRequest)
	}
	var imported commercial.GroupImportResult
	if err := json.NewDecoder(importRecorder.Body).Decode(&imported); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if imported.BundleID != "import-bundle" || imported.Summary.Success != 1 {
		t.Fatalf("import response = %#v", imported)
	}
}

func TestGroupTransferRoutesRequireManagerPort(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerGroupTransfer(api, TenantGroupManagementHTTPDeps{})

	recorder := performGroupTransferRequest(t, router, http.MethodPost, "/api/v1/tenants/me/groups/export", `{"group_ids":["group-1"]}`)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("export without manager status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func performGroupTransferRequest(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

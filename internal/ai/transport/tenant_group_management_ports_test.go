package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestTenantGroupManagementRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tenants/me/groups"},
		{http.MethodPost, "/api/v1/tenants/me/groups"},
		{http.MethodGet, "/api/v1/tenants/me/groups/group-1"},
		{http.MethodPatch, "/api/v1/tenants/me/groups/group-1"},
		{http.MethodPatch, "/api/v1/tenants/me/groups/group-1/route-policy"},
		{http.MethodPatch, "/api/v1/tenants/me/groups/group-1/status"},
		{http.MethodGet, "/api/v1/tenants/me/groups/group-1/client-surface-policy"},
		{http.MethodPut, "/api/v1/tenants/me/groups/group-1/client-surface-policy"},
		{http.MethodDelete, "/api/v1/tenants/me/groups/group-1"},
		{http.MethodGet, "/api/v1/tenants/me/groups/group-1/dispatch-rules"},
		{http.MethodPost, "/api/v1/tenants/me/groups/group-1/dispatch-rules/preview"},
		{http.MethodPost, "/api/v1/tenants/me/groups/group-1/dispatch-rules"},
		{http.MethodPatch, "/api/v1/tenants/me/groups/group-1/dispatch-rules/rule-1"},
		{http.MethodPatch, "/api/v1/tenants/me/groups/group-1/dispatch-rules/rule-1/status"},
		{http.MethodGet, "/api/v1/tenants/me/groups/group-1/dispatch-models"},
		{http.MethodDelete, "/api/v1/tenants/me/groups/group-1/dispatch-rules/rule-1"},
		{http.MethodGet, "/api/v1/tenants/me/groups/group-1/targets"},
		{http.MethodPut, "/api/v1/tenants/me/groups/group-1/targets"},
		{http.MethodPost, "/api/v1/tenants/me/groups/group-1/targets"},
		{http.MethodPatch, "/api/v1/tenants/me/groups/group-1/targets/binding-1"},
		{http.MethodDelete, "/api/v1/tenants/me/groups/group-1/targets/binding-1"},
		{http.MethodGet, "/api/v1/tenants/me/users/user-1/groups"},
		{http.MethodPut, "/api/v1/tenants/me/users/user-1/groups/group-1"},
		{http.MethodDelete, "/api/v1/tenants/me/users/user-1/groups/group-1"},
		{http.MethodPost, "/api/v1/tenants/me/groups/export"},
		{http.MethodPost, "/api/v1/tenants/me/groups/import/preview"},
		{http.MethodPost, "/api/v1/tenants/me/groups/import"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, CoreHTTPDeps{})
	for _, route := range routes {
		recorder := performTenantGroupManagementRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI tenant group route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	groupRouter, groupAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterTenantGroupManagement(groupAPI, TenantGroupManagementHTTPDeps{})
	for _, route := range routes {
		recorder := performTenantGroupManagementRequest(groupRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent tenant group route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performTenantGroupManagementRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

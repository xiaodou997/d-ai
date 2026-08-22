package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestWorkspaceRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tenants/me/workspace/overview"},
		{http.MethodGet, "/api/v1/tenants/me/workspace/chat/models"},
		{http.MethodGet, "/api/v1/tenants/me/workspace/chat/sessions"},
		{http.MethodPost, "/api/v1/tenants/me/workspace/chat/sessions"},
		{http.MethodGet, "/api/v1/tenants/me/workspace/chat/sessions/session-1"},
		{http.MethodDelete, "/api/v1/tenants/me/workspace/chat/sessions/session-1"},
		{http.MethodGet, "/api/v1/tenants/me/workspace/image/jobs"},
		{http.MethodGet, "/api/v1/users/me/workspace/overview"},
		{http.MethodGet, "/api/v1/users/me/workspace/chat/models"},
		{http.MethodGet, "/api/v1/users/me/workspace/chat/sessions"},
		{http.MethodPost, "/api/v1/users/me/workspace/chat/sessions"},
		{http.MethodGet, "/api/v1/users/me/workspace/chat/sessions/session-1"},
		{http.MethodDelete, "/api/v1/users/me/workspace/chat/sessions/session-1"},
		{http.MethodGet, "/api/v1/users/me/workspace/image/jobs"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, CoreHTTPDeps{})
	for _, route := range routes {
		recorder := performWorkspaceContractRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI workspace route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	workspaceRouter, workspaceAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterWorkspace(workspaceAPI, WorkspaceHTTPDeps{})
	for _, route := range routes {
		recorder := performWorkspaceContractRequest(workspaceRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent workspace route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performWorkspaceContractRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

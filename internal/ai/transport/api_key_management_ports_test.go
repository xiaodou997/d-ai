package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestAPIKeyManagementRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tenants/tenant-1/api-keys"},
		{http.MethodPost, "/api/v1/tenants/tenant-1/api-keys"},
		{http.MethodGet, "/api/v1/tenants/tenant-1/users/user-1/api-keys"},
		{http.MethodPost, "/api/v1/tenants/tenant-1/users/user-1/api-keys"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, CoreHTTPDeps{})
	for _, route := range routes {
		recorder := performAPIKeyManagementRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI API-key route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	managementRouter, managementAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAPIKeyManagement(managementAPI, APIKeyManagementHTTPDeps{})
	for _, route := range routes {
		recorder := performAPIKeyManagementRequest(managementRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent API-key route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performAPIKeyManagementRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

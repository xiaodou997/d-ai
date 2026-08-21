package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestUpstreamAccessManagementRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tenants/tenant-1/upstream-access"},
		{http.MethodPut, "/api/v1/tenants/tenant-1/upstream-access"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, route := range routes {
		recorder := performUpstreamAccessManagementRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI upstream-access route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	managementRouter, managementAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterUpstreamAccessManagement(managementAPI, UpstreamAccessManagementHTTPDeps{})
	for _, route := range routes {
		recorder := performUpstreamAccessManagementRequest(managementRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent upstream-access route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performUpstreamAccessManagementRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

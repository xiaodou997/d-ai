package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestUpstreamDiagnosticsRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/upstream-accounts/account-1/upstream-models"},
		{method: http.MethodPost, path: "/api/v1/upstream-accounts/account-1/import-upstream-models"},
		{method: http.MethodGet, path: "/api/v1/model-capability/infer"},
		{method: http.MethodPost, path: "/api/v1/upstream-accounts/account-1/test"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, route := range routes {
		recorder := performUpstreamDiagnosticsRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI upstream diagnostics route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	diagnosticsRouter, diagnosticsAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterUpstreamDiagnostics(diagnosticsAPI, UpstreamDiagnosticsHTTPDeps{})
	for _, route := range routes {
		recorder := performUpstreamDiagnosticsRequest(diagnosticsRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent upstream diagnostics route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performUpstreamDiagnosticsRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

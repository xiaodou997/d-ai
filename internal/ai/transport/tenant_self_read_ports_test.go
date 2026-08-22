package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestTenantSelfReadRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []string{
		"/api/v1/tenants/me/dashboard/summary",
		"/api/v1/tenants/me/dashboard/top-models",
		"/api/v1/tenants/me/dashboard/recent-errors",
		"/api/v1/tenants/me/usage-logs",
		"/api/v1/tenants/me/usage-summary",
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, path := range routes {
		recorder := performTenantSelfReadRequest(coreRouter, path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI tenant self-read route %s status = %d, want %d", path, recorder.Code, http.StatusNotFound)
		}
	}

	readRouter, readAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterTenantSelfRead(readAPI, TenantSelfReadHTTPDeps{})
	for _, path := range routes {
		recorder := performTenantSelfReadRequest(readRouter, path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent tenant self-read route %s status = %d, want %d", path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performTenantSelfReadRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

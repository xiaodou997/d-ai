package transport

import (
	"net/http"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestUpstreamAccountManagementRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/upstream-accounts"},
		{method: http.MethodPost, path: "/api/v1/upstream-accounts"},
		{method: http.MethodPatch, path: "/api/v1/upstream-accounts/account-1"},
		{method: http.MethodPatch, path: "/api/v1/upstream-accounts/account-1/status"},
		{method: http.MethodDelete, path: "/api/v1/upstream-accounts/account-1"},
		{method: http.MethodPost, path: "/api/v1/upstream-accounts/export"},
		{method: http.MethodPost, path: "/api/v1/upstream-accounts/import/preview"},
		{method: http.MethodPost, path: "/api/v1/upstream-accounts/import"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, route := range routes {
		recorder := performUpstreamAccountRequest(coreRouter, route.method, route.path, "")
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI upstream-account route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	managementRouter, managementAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterUpstreamAccountManagement(managementAPI, UpstreamAccountManagementHTTPDeps{})
	for _, route := range routes {
		recorder := performUpstreamAccountRequest(managementRouter, route.method, route.path, "")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent upstream-account route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

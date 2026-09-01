package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestComponentProbeStatus(t *testing.T) {
	tests := []struct {
		name       string
		probe      ComponentHealthProbe
		wantStatus string
		wantError  string
	}{
		{name: "disabled", wantStatus: "disabled"},
		{name: "healthy", probe: componentHealthProbeStub{}, wantStatus: "ok"},
		{name: "unhealthy", probe: componentHealthProbeStub{err: errors.New("redis unavailable")}, wantStatus: "error", wantError: "redis unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := componentProbeStatus(t.Context(), tt.probe)
			if got.Status != tt.wantStatus || got.Error != tt.wantError {
				t.Fatalf("status = %#v, want status %q error %q", got, tt.wantStatus, tt.wantError)
			}
		})
	}
}

func TestSystemRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	paths := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/system/status"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, CoreHTTPDeps{})
	for _, route := range paths {
		coreRequest := httptest.NewRequest(route.method, route.path, nil)
		coreRecorder := httptest.NewRecorder()
		coreRouter.ServeHTTP(coreRecorder, coreRequest)
		if coreRecorder.Code != http.StatusNotFound {
			t.Fatalf("core AI system route %s %s status = %d, want %d", route.method, route.path, coreRecorder.Code, http.StatusNotFound)
		}
	}

	systemRouter, systemAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterSystem(systemAPI, SystemHTTPDeps{})
	for _, route := range paths {
		systemRequest := httptest.NewRequest(route.method, route.path, nil)
		systemRecorder := httptest.NewRecorder()
		systemRouter.ServeHTTP(systemRecorder, systemRequest)
		if systemRecorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent system route %s %s status = %d, want %d", route.method, route.path, systemRecorder.Code, http.StatusUnauthorized)
		}
	}
}

type componentHealthProbeStub struct {
	err error
}

func (s componentHealthProbeStub) Check(context.Context) error { return s.err }

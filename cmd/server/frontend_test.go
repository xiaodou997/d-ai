package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPortalHandlerServesSPAAndStaticAssets(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><title>D-AI Portal</title>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('portal')")},
	}
	handler := newPortalHandler(fs.FS(dist))

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantContent string
	}{
		{
			name:        "frontend route falls back to index",
			path:        "/login",
			wantStatus:  http.StatusOK,
			wantContent: "D-AI Portal",
		},
		{
			name:        "static asset is served directly",
			path:        "/assets/app.js",
			wantStatus:  http.StatusOK,
			wantContent: "console.log('portal')",
		},
		{
			name:       "unknown API is not handled as a frontend route",
			path:       "/api/missing",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if tt.wantContent != "" && !strings.Contains(response.Body.String(), tt.wantContent) {
				t.Fatalf("body = %q, want content %q", response.Body.String(), tt.wantContent)
			}
		})
	}
}

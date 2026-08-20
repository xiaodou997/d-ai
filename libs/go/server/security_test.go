package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://example.test/", nil))

	checks := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Permissions-Policy":        "camera=(), geolocation=(), microphone=(), payment=(), usb=()",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	}
	for header, want := range checks {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Errorf("CSP = %q, want frame policy", got)
	}
}

func TestNewDisablesInteractiveDocsByDefault(t *testing.T) {
	r, _ := New(Options{Title: "test", Version: "dev"})
	var routes []string
	if err := chi.Walk(r, func(_ string, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, route)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range routes {
		if route == "/docs" || route == "/openapi.json" || route == "/openapi.yaml" {
			t.Fatalf("debug route %q is exposed by default", route)
		}
	}
}

func TestNoStoreAPIAllowsExplicitPublicAssetCache(t *testing.T) {
	handler := NoStoreAPI(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/public/tenant-brands/acme/favicon", nil))
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want explicit public override", got)
	}
}

func TestRequestBodyLimitRejectsDeclaredOversizeAndCapsChunkedBody(t *testing.T) {
	called := false
	handler := RequestBodyLimit(4)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, err := io.ReadAll(r.Body)
		if err == nil {
			t.Error("ReadAll accepted body over limit")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader("12345"))
	request.ContentLength = 5
	handler.ServeHTTP(rec, request)
	if rec.Code != http.StatusRequestEntityTooLarge || called {
		t.Fatalf("declared oversize status=%d called=%v", rec.Code, called)
	}

	called = false
	rec = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api", strings.NewReader("12345"))
	request.ContentLength = -1
	handler.ServeHTTP(rec, request)
	if rec.Code != http.StatusNoContent || !called {
		t.Fatalf("chunked body status=%d called=%v", rec.Code, called)
	}
}

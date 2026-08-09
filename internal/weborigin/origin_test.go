package weborigin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromRequestUsesForwardedPublicOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/health", nil)
	r.Host = "127.0.0.1:19641"
	r.Header.Set("X-Forwarded-Host", "uadmin.example.test")
	r.Header.Set("X-Forwarded-Proto", "https")

	if got := FromRequest(r); got != "https://uadmin.example.test" {
		t.Fatalf("origin = %q, want https://uadmin.example.test", got)
	}
}

func TestResolveFallsBackToRelativePath(t *testing.T) {
	if got := Resolve(context.Background(), "legal/terms"); got != "/legal/terms" {
		t.Fatalf("resolved path = %q, want /legal/terms", got)
	}
}

func TestResolveUsesRequestOrigin(t *testing.T) {
	ctx := WithOrigin(context.Background(), "https://uadmin.example.test/")
	if got := Resolve(ctx, "/v1/files/content/token"); got != "https://uadmin.example.test/v1/files/content/token" {
		t.Fatalf("resolved URL = %q", got)
	}
}

package weborigin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolverUsesConfiguredPublicOriginOverRequestHeaders(t *testing.T) {
	resolver, err := NewResolver("https://portal.example.test/", []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/health", nil)
	r.RemoteAddr = "127.0.0.1:19641"
	r.Host = "127.0.0.1:19641"
	r.Header.Set("X-Forwarded-Host", "attacker.example.test")
	r.Header.Set("X-Forwarded-Proto", "http")

	if got := resolver.FromRequest(r); got != "https://portal.example.test" {
		t.Fatalf("origin = %q, want https://portal.example.test", got)
	}
}

func TestResolverHonorsForwardedHeadersOnlyFromTrustedPeer(t *testing.T) {
	resolver, err := NewResolver("", []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	trusted := httptest.NewRequest(http.MethodGet, "http://10.0.0.8/health", nil)
	trusted.RemoteAddr = "10.0.0.8:19641"
	trusted.Host = "127.0.0.1:19641"
	trusted.Header.Set("X-Forwarded-Host", "portal.example.test")
	trusted.Header.Set("X-Forwarded-Proto", "https")
	if got := resolver.FromRequest(trusted); got != "https://portal.example.test" {
		t.Fatalf("trusted origin = %q, want https://portal.example.test", got)
	}

	untrusted := httptest.NewRequest(http.MethodGet, "http://203.0.113.9/health", nil)
	untrusted.RemoteAddr = "203.0.113.9:19641"
	untrusted.Host = "safe.example.test"
	untrusted.Header.Set("X-Forwarded-Host", "attacker.example.test")
	untrusted.Header.Set("X-Forwarded-Proto", "https")
	if got := resolver.FromRequest(untrusted); got != "" {
		t.Fatalf("untrusted origin = %q, want empty origin", got)
	}
}

func TestResolverResolvesMultiHopClientIPFromTrustedChain(t *testing.T) {
	resolver, err := NewResolver("https://portal.example.test", []string{"10.0.0.0/8", "192.0.2.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "https://portal.example.test/health", nil)
	r.RemoteAddr = "10.0.0.8:19641"
	r.Header.Set("X-Forwarded-For", "198.51.100.44, 192.0.2.8, 10.0.0.7")
	if got := resolver.ClientIP(r); got != "198.51.100.44" {
		t.Fatalf("client IP = %q, want 198.51.100.44", got)
	}

	r.RemoteAddr = "203.0.113.8:19641"
	r.Header.Set("X-Forwarded-For", "198.51.100.44")
	if got := resolver.ClientIP(r); got != "203.0.113.8" {
		t.Fatalf("untrusted client IP = %q, want 203.0.113.8", got)
	}
}

func TestResolverParsesRFCForwardedHeaders(t *testing.T) {
	resolver, err := NewResolver("", []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/health", nil)
	r.RemoteAddr = "127.0.0.1:19641"
	r.Host = "127.0.0.1:19641"
	r.Header.Set("Forwarded", "for=198.51.100.22;proto=https;host=portal.example.test")
	if got := resolver.FromRequest(r); got != "https://portal.example.test" {
		t.Fatalf("origin = %q, want https://portal.example.test", got)
	}
	if got := resolver.ClientIP(r); got != "198.51.100.22" {
		t.Fatalf("client IP = %q, want 198.51.100.22", got)
	}
}

func TestResolverWalksMultiHopRFCForwardedClientIP(t *testing.T) {
	resolver, err := NewResolver("", []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/health", nil)
	r.RemoteAddr = "10.0.0.8:19641"
	r.Header.Set("Forwarded", "for=198.51.100.22;proto=https;host=portal.example.test, for=10.0.0.7")
	if got := resolver.ClientIP(r); got != "198.51.100.22" {
		t.Fatalf("client IP = %q, want 198.51.100.22", got)
	}
}

func TestNormalizePublicOriginRejectsNonOriginValues(t *testing.T) {
	for _, raw := range []string{
		"portal.example.test",
		"https://portal.example.test/path",
		"https://user:pass@portal.example.test",
		"https://portal.example.test/?next=evil",
		"javascript://portal.example.test",
	} {
		if _, err := NormalizePublicOrigin(raw); err == nil {
			t.Fatalf("NormalizePublicOrigin(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestResolveFallsBackToRelativePath(t *testing.T) {
	if got := Resolve(context.Background(), "legal/terms"); got != "/legal/terms" {
		t.Fatalf("resolved path = %q, want /legal/terms", got)
	}
}

func TestResolveUsesRequestOrigin(t *testing.T) {
	ctx := WithOrigin(context.Background(), "https://portal.example.test/")
	if got := Resolve(ctx, "/v1/files/content/token"); got != "https://portal.example.test/v1/files/content/token" {
		t.Fatalf("resolved URL = %q", got)
	}
}

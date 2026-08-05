package serviceidentity

import (
	"net/http/httptest"
	"testing"
)

func TestSourceResolverRejectsSpoofedForwardedFor(t *testing.T) {
	r, err := NewSourceResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.8:1234"
	req.Header.Set("X-Forwarded-For", "10.0.0.2")
	got, err := r.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "203.0.113.8" {
		t.Fatalf("got %s", got)
	}
}

func TestSourceResolverTrustedProxyChain(t *testing.T) {
	r, err := NewSourceResolver([]string{"10.0.0.0/8", "2001:db8:1::/48"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.9:1234"
	req.Header.Set("X-Forwarded-For", "2001:db8::7, 10.0.0.8")
	got, err := r.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "2001:db8::7" {
		t.Fatalf("got %s", got)
	}
}

func TestParseCIDRsIPv4AndIPv6(t *testing.T) {
	prefixes, err := ParseCIDRs([]string{"192.0.2.1", "2001:db8::/32"})
	if err != nil {
		t.Fatal(err)
	}
	if got := prefixes[0].String(); got != "192.0.2.1/32" {
		t.Fatalf("got %s", got)
	}
	if got := prefixes[1].String(); got != "2001:db8::/32" {
		t.Fatalf("got %s", got)
	}
}

func TestSourceResolverRejectsMalformedTrustedProxyHeader(t *testing.T) {
	r, err := NewSourceResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.9:1234"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	if _, err := r.Resolve(req); err == nil {
		t.Fatal("expected malformed header rejection")
	}
}

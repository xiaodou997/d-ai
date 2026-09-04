package promptaudit

import (
	"net/netip"
	"testing"
)

func TestNormalizeBaseURLRequiresSafeHTTPSShape(t *testing.T) {
	if got, err := NormalizeBaseURL("https://guard.example/v1"); err != nil || got != "https://guard.example" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	for _, raw := range []string{"http://guard.example", "https://user:pass@guard.example", "https://guard.example?q=1", "https://guard.example/#x"} {
		if _, err := NormalizeBaseURL(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestPublicAddressPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "192.0.2.1", "198.51.100.1", "203.0.113.1", "::1", "fc00::1", "2001:db8::1"} {
		if isPublicAddr(netip.MustParseAddr(raw)) {
			t.Fatalf("accepted %s", raw)
		}
	}
	if !isPublicAddr(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public address rejected")
	}
}

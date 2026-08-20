package transport

import (
	"net/http"
	"testing"
)

func TestRefreshCookieSecurityAttributes(t *testing.T) {
	cookie := refreshCookie("dai_rt_test", 3600, true)
	if cookie.Name != refreshCookieName || cookie.Value != "dai_rt_test" {
		t.Fatalf("cookie identity = %s=%s", cookie.Name, cookie.Value)
	}
	if cookie.Path != "/api/auth" || cookie.Domain != "" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie attributes = %+v", cookie)
	}
	if cookie.MaxAge != 3600 || cookie.Expires.IsZero() {
		t.Fatalf("cookie expiry = max-age:%d expires:%v", cookie.MaxAge, cookie.Expires)
	}

	cleared := clearRefreshCookie(true)
	if cleared.Name != refreshCookieName || cleared.Path != "/api/auth" || cleared.Domain != "" || cleared.MaxAge != -1 || !cleared.HttpOnly || !cleared.Secure {
		t.Fatalf("clear cookie = %+v", cleared)
	}
}

func TestSameOriginMatches(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		referer string
		host    string
		tls     bool
		want    bool
	}{
		{name: "same origin", origin: "https://dai.example.test", host: "dai.example.test", tls: true, want: true},
		{name: "same origin referer", referer: "https://dai.example.test/app", host: "dai.example.test", tls: true, want: true},
		{name: "cross origin", origin: "https://evil.example.test", host: "dai.example.test", tls: true, want: false},
		{name: "userinfo origin", origin: "https://user:password@dai.example.test", host: "dai.example.test", tls: true, want: false},
		{name: "downgrade", origin: "http://dai.example.test", host: "dai.example.test", tls: true, want: false},
		{name: "native client", host: "dai.example.test", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameOriginMatches(tt.origin, tt.referer, tt.host, tt.tls); got != tt.want {
				t.Fatalf("sameOriginMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSameOriginMatchesConfiguredPublicOrigin(t *testing.T) {
	if !sameOriginMatchesOrigin("https://portal.example.test", "", "https://portal.example.test") {
		t.Fatal("same configured origin was rejected")
	}
	if sameOriginMatchesOrigin("https://attacker.example.test", "", "https://portal.example.test") {
		t.Fatal("cross-origin request was accepted")
	}
	if sameOriginMatchesOrigin("http://portal.example.test", "", "https://portal.example.test") {
		t.Fatal("scheme downgrade was accepted")
	}
}

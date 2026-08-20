package tokenrefresh

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTokenRefreshOnlyTreatsExplicitInvalidGrantAsPermanent(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		permanent bool
	}{
		{name: "invalid grant", status: http.StatusBadRequest, body: `{"error":"invalid_grant"}`, permanent: true},
		{name: "invalid token", status: http.StatusUnauthorized, body: `{"error":"invalid_token"}`, permanent: true},
		{name: "provider unavailable", status: http.StatusServiceUnavailable, body: `{"error":"temporarily_unavailable"}`, permanent: false},
		{name: "client configuration", status: http.StatusUnauthorized, body: `{"error":"invalid_client"}`, permanent: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			refresher := &Refresher{client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.status,
					Body:       io.NopCloser(strings.NewReader(test.body)),
					Header:     http.Header{},
				}, nil
			})}}
			_, err := refresher.callTokenEndpoint(context.Background(), providerConfig{TokenURL: "https://oauth.example/token"}, "refresh-token")
			if err == nil {
				t.Fatal("expected token endpoint error")
			}
			if got := isPermanentTokenError(err); got != test.permanent {
				t.Fatalf("permanent = %v, want %v; err=%v", got, test.permanent, err)
			}
		})
	}
}

func TestTokenRefreshNetworkFailureIsRetryable(t *testing.T) {
	want := errors.New("connection reset")
	refresher := &Refresher{client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, want
	})}}
	_, err := refresher.callTokenEndpoint(context.Background(), providerConfig{TokenURL: "https://oauth.example/token"}, "refresh-token")
	if err == nil || isPermanentTokenError(err) {
		t.Fatalf("network error must stay retryable: %v", err)
	}
}

func TestTokenRefreshRejectsMissingExplicitClientSecret(t *testing.T) {
	refresher := &Refresher{client: &http.Client{}}
	_, err := refresher.callTokenEndpoint(context.Background(), providerConfig{
		TokenURL:      "https://oauth.example/token",
		ClientID:      "client-id",
		RequireSecret: true,
	}, "refresh-token")
	if err == nil || !strings.Contains(err.Error(), "client secret is not configured") {
		t.Fatalf("missing client secret error = %v", err)
	}
}

func TestDefaultProviderConfigsMatchCurrentClientRefreshContracts(t *testing.T) {
	t.Setenv("GEMINI_OAUTH_CLIENT_SECRET", "")
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET", "")
	configs := defaultProviderConfigs()

	tests := []struct {
		provider domain.FixedProviderType
		tokenURL string
		scope    string
		useJSON  bool
		secret   string
	}{
		{
			provider: domain.FixedProviderCodex,
			tokenURL: "https://auth.openai.com/oauth/token",
			scope:    codexRefreshScope,
		},
		{
			provider: domain.FixedProviderClaudeOAuth,
			tokenURL: "https://platform.claude.com/v1/oauth/token",
			useJSON:  true,
		},
		{
			provider: domain.FixedProviderGeminiCLI,
			tokenURL: "https://oauth2.googleapis.com/token",
			scope:    geminiCLIScope,
		},
		{
			provider: domain.FixedProviderAntigravity,
			tokenURL: "https://oauth2.googleapis.com/token",
			scope:    antigravityScope,
		},
	}

	for _, test := range tests {
		cfg := configs[test.provider]
		if cfg.TokenURL != test.tokenURL || cfg.Scope != test.scope ||
			cfg.UseJSON != test.useJSON || cfg.ClientSecret != test.secret {
			t.Fatalf("%s config drifted: %+v", test.provider, cfg)
		}
	}
}

func TestTokenRefreshEncodesVersionedProviderParameters(t *testing.T) {
	tests := []struct {
		name    string
		useJSON bool
	}{
		{name: "form"},
		{name: "json", useJSON: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got map[string]string
			refresher := &Refresher{client: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				defer req.Body.Close()
				if test.useJSON {
					if req.Header.Get("Content-Type") != "application/json" {
						t.Errorf("content type = %q", req.Header.Get("Content-Type"))
					}
					if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
						t.Errorf("decode JSON body: %v", err)
					}
				} else {
					if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
						t.Errorf("content type = %q", req.Header.Get("Content-Type"))
					}
					body, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("read form body: %v", err)
					}
					values, err := url.ParseQuery(string(body))
					if err != nil {
						t.Errorf("parse form body: %v", err)
					}
					got = make(map[string]string, len(values))
					for key := range values {
						got[key] = values.Get(key)
					}
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"access_token":"access","expires_in":3600}`)),
				}, nil
			})}}

			cfg := providerConfig{
				TokenURL:     "https://oauth.example/token",
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				Scope:        "scope-a scope-b",
				UseJSON:      test.useJSON,
			}
			if _, err := refresher.callTokenEndpoint(context.Background(), cfg, "refresh-token"); err != nil {
				t.Fatalf("callTokenEndpoint: %v", err)
			}

			want := map[string]string{
				"grant_type":    "refresh_token",
				"refresh_token": "refresh-token",
				"client_id":     "client-id",
				"client_secret": "client-secret",
				"scope":         "scope-a scope-b",
			}
			if len(got) != len(want) {
				t.Fatalf("parameters = %#v, want %#v", got, want)
			}
			for key, value := range want {
				if got[key] != value {
					t.Fatalf("%s = %q, want %q; all=%#v", key, got[key], value, got)
				}
			}
		})
	}
}

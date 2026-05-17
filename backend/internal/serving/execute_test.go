package serving

import (
	"net/http"
	"testing"

	"uni-ai-api/backend/internal/domain"
)

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name     string
		cand     *domain.RouteCandidate
		stream   bool
		want     string
	}{
		{
			name: "openai chat default path",
			cand: &domain.RouteCandidate{
				BaseURL:  "https://api.openai.com",
				Protocol: domain.ProtocolOpenAIChat,
			},
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "trailing slash trimmed and custom path",
			cand: &domain.RouteCandidate{
				BaseURL:     "https://example.com/api/",
				RequestPath: "/foo/bar",
				Protocol:    domain.ProtocolOpenAIChat,
			},
			want: "https://example.com/api/foo/bar",
		},
		{
			name: "path without leading slash gets one",
			cand: &domain.RouteCandidate{
				BaseURL:     "https://example.com",
				RequestPath: "rel/path",
				Protocol:    domain.ProtocolOpenAIChat,
			},
			want: "https://example.com/rel/path",
		},
		{
			name: "gemini model placeholder substitution",
			cand: &domain.RouteCandidate{
				BaseURL:       "https://generativelanguage.googleapis.com",
				UpstreamModel: "gemini-2.0-flash",
				Protocol:      domain.ProtocolGeminiGenerate,
			},
			want: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent",
		},
		{
			name: "gemini_cli uses CodeAssist URL regardless of RequestPath",
			cand: &domain.RouteCandidate{
				BaseURL:           "https://ignored.example",
				RequestPath:       "/should/be/ignored",
				FixedProviderType: domain.FixedProviderGeminiCLI,
				Protocol:          domain.ProtocolGeminiGenerate,
			},
			stream: false,
			// We can't hardcode the exact URL without importing the gemini pkg;
			// just sanity-check it does NOT fall back to the RequestPath.
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildURL(tc.cand, tc.stream)
			if tc.want != "" && got != tc.want {
				t.Fatalf("buildURL = %q, want %q", got, tc.want)
			}
			if tc.cand.FixedProviderType == domain.FixedProviderGeminiCLI {
				if got == "" || got == tc.cand.BaseURL+tc.cand.RequestPath {
					t.Fatalf("gemini_cli URL should bypass RequestPath, got %q", got)
				}
			}
		})
	}
}

func TestBuildHeadersAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		cand     *domain.RouteCandidate
		req      *Request
		wantAuth string
		wantKey  string // header name expected to carry the key
	}{
		{
			name: "openai uses Bearer",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolOpenAIChat,
				APIKeyCiphertext: "sk-abc",
			},
			req:      &Request{},
			wantAuth: "Bearer sk-abc",
			wantKey:  "Authorization",
		},
		{
			name: "anthropic uses x-api-key + version",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolAnthropicMessages,
				APIKeyCiphertext: "sk-ant",
			},
			req:     &Request{},
			wantKey: "x-api-key",
		},
		{
			name: "gemini exposes x-gemini-api-key for transport to consume",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolGeminiGenerate,
				APIKeyCiphertext: "AIza-test",
			},
			req:     &Request{},
			wantKey: "x-gemini-api-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := buildHeaders(tc.cand, tc.req)
			if h["Content-Type"] != "application/json" {
				t.Fatalf("missing Content-Type")
			}
			switch tc.wantKey {
			case "Authorization":
				if h["Authorization"] != tc.wantAuth {
					t.Fatalf("Authorization = %q, want %q", h["Authorization"], tc.wantAuth)
				}
			case "x-api-key":
				if h["x-api-key"] != tc.cand.APIKeyCiphertext {
					t.Fatalf("x-api-key not set to API key")
				}
				if h["anthropic-version"] == "" {
					t.Fatalf("anthropic-version missing")
				}
			case "x-gemini-api-key":
				if h["x-gemini-api-key"] != tc.cand.APIKeyCiphertext {
					t.Fatalf("x-gemini-api-key = %q, want %q", h["x-gemini-api-key"], tc.cand.APIKeyCiphertext)
				}
				if _, ok := h["Authorization"]; ok {
					t.Fatalf("Gemini API-key path must not set Authorization header")
				}
			}
		})
	}
}

func TestBuildHeadersStreamAccept(t *testing.T) {
	cand := &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIChat, APIKeyCiphertext: "k"}
	h := buildHeaders(cand, &Request{IsStream: true})
	if h["Accept"] != "text/event-stream" {
		t.Fatalf("stream request must set Accept text/event-stream, got %q", h["Accept"])
	}
}

func TestUpstreamStatusToGateway(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{429, http.StatusTooManyRequests},
		{500, http.StatusBadGateway},
		{502, http.StatusBadGateway},
		{503, http.StatusBadGateway},
		{401, http.StatusBadGateway},
		{403, http.StatusBadGateway},
		{400, 400},
		{404, 404},
		{409, 409},
	}
	for _, tc := range tests {
		if got := upstreamStatusToGateway(tc.in); got != tc.want {
			t.Errorf("upstreamStatusToGateway(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

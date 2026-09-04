package upstreamcompat

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestBuildHeadersForwardsOutboundUserAgent(t *testing.T) {
	cand := &domain.RouteCandidate{
		Protocol:         domain.ProtocolOpenAIChat,
		APIKeyCiphertext: "sk-abc",
	}

	headers := BuildHeaders(cand, RequestMeta{OutboundUserAgent: "curl/8.7.1"})
	if got := headers["user-agent"]; got != "curl/8.7.1" {
		t.Fatalf("user-agent = %q, want curl/8.7.1", got)
	}
}

func TestBuildURLRejectsFixedProviderWithoutClientRuntime(t *testing.T) {
	cand := &domain.RouteCandidate{
		Protocol:          domain.ProtocolAnthropicMessages,
		FixedProviderType: domain.FixedProviderClaudeOAuth,
	}
	if _, err := BuildURL(cand, RequestMeta{}); err == nil {
		t.Fatal("BuildURL() error = nil")
	}
}

func TestBuildURLUsesEndpointPathOverrideAndModelTemplate(t *testing.T) {
	cand := &domain.RouteCandidate{
		Protocol: domain.ProtocolGeminiGenerate, BaseURL: "https://gateway.example/google",
		RequestPath: "/google/v2/models/{model}:generate", UpstreamModel: "gemini-test",
	}
	got, err := BuildURL(cand, RequestMeta{})
	if err != nil {
		t.Fatalf("BuildURL() error = %v", err)
	}
	if got != "https://gateway.example/google/v2/models/gemini-test:generate" {
		t.Fatalf("BuildURL() = %q", got)
	}
}

func TestBuildHeadersUsesEndpointAuthScheme(t *testing.T) {
	cand := &domain.RouteCandidate{
		Protocol: domain.ProtocolOpenAIResponses, APIKeyCiphertext: "secret",
		EndpointAuthScheme: domain.EndpointAuthCustomHeader, EndpointAuthHeader: "X-Provider-Key",
	}
	headers := BuildHeaders(cand, RequestMeta{})
	if headers["X-Provider-Key"] != "secret" || headers["Authorization"] != "" {
		t.Fatalf("headers = %#v", headers)
	}
}

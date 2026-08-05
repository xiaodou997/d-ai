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

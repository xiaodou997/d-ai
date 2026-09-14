package serving

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestMediaFillEstimatedUsageDoesNotInventCompletionTokens(t *testing.T) {
	req := &Request{CapabilityType: domain.CapabilityImage, TokenUsage: domain.TokenUsage{PromptTokens: 0}, UpstreamBodySize: 0}
	fillEstimatedUsage(req, 0)
	if req.TokenCountSource != domain.TokenUsageSourceMissing {
		t.Fatalf("token source = %q, want missing", req.TokenCountSource)
	}
	if req.TokenUsage.PromptTokens != 0 || req.TokenUsage.CompletionTokens != 0 {
		t.Fatalf("estimated usage = %+v, want zero observed usage", req.TokenUsage)
	}
}

func TestMediaFillEstimatedUsageDoesNotConvertBytesToTokens(t *testing.T) {
	req := &Request{CapabilityType: domain.CapabilityImage, TokenUsage: domain.TokenUsage{PromptTokens: 0}, UpstreamBodySize: 4}
	fillEstimatedUsage(req, 7)
	if req.TokenUsage.PromptTokens != 0 || req.TokenUsage.CompletionTokens != 0 {
		t.Fatalf("estimated usage = %+v, want no invented tokens", req.TokenUsage)
	}
}

func TestMediaFillEstimatedUsagePreservesReportedOutput(t *testing.T) {
	req := &Request{CapabilityType: domain.CapabilityImage, TokenUsage: domain.TokenUsage{CompletionTokens: 55}, UpstreamBodySize: 162}
	fillEstimatedUsage(req, 24)
	if req.TokenUsage.PromptTokens != 0 || req.TokenUsage.CompletionTokens != 55 {
		t.Fatalf("usage = %+v, want missing prompt=54 and upstream completion=55", req.TokenUsage)
	}
	if req.TokenCountSource != domain.TokenUsageSourceUpstream {
		t.Fatalf("token source = %q, want upstream", req.TokenCountSource)
	}
}

func TestMediaFillEstimatedUsagePreservesReportedInput(t *testing.T) {
	req := &Request{CapabilityType: domain.CapabilityImage, TokenUsage: domain.TokenUsage{PromptTokens: 544}}
	fillEstimatedUsage(req, 24)
	if req.TokenUsage.PromptTokens != 544 || req.TokenUsage.CompletionTokens != 0 {
		t.Fatalf("usage = %+v, want upstream prompt=544 and unreported completion=0", req.TokenUsage)
	}
	if req.TokenCountSource != domain.TokenUsageSourceUpstream {
		t.Fatalf("token source = %q, want upstream", req.TokenCountSource)
	}
}

package serving

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestFillEstimatedUsageDoesNotInventCompletionTokens(t *testing.T) {
	req := &Request{TokenUsage: domain.TokenUsage{PromptTokens: 0}, UpstreamBodySize: 0}
	fillEstimatedUsage(req, 0)
	if req.TokenCountSource != domain.TokenUsageSourceEstimated {
		t.Fatalf("token source = %q, want estimated", req.TokenCountSource)
	}
	if req.TokenUsage.PromptTokens != 0 || req.TokenUsage.CompletionTokens != 0 {
		t.Fatalf("estimated usage = %+v, want zero observed usage", req.TokenUsage)
	}
}

func TestFillEstimatedUsageUsesObservedBytes(t *testing.T) {
	req := &Request{TokenUsage: domain.TokenUsage{PromptTokens: 0}, UpstreamBodySize: 4}
	fillEstimatedUsage(req, 7)
	if req.TokenUsage.PromptTokens != 2 || req.TokenUsage.CompletionTokens != 3 {
		t.Fatalf("estimated usage = %+v, want prompt=2 completion=3", req.TokenUsage)
	}
}

func TestFillEstimatedUsagePreservesReportedOutput(t *testing.T) {
	req := &Request{TokenUsage: domain.TokenUsage{CompletionTokens: 55}, UpstreamBodySize: 162}
	fillEstimatedUsage(req, 24)
	if req.TokenUsage.PromptTokens != 54 || req.TokenUsage.CompletionTokens != 55 {
		t.Fatalf("usage = %+v, want estimated prompt=54 and upstream completion=55", req.TokenUsage)
	}
	if req.TokenCountSource != domain.TokenUsageSourceMixed {
		t.Fatalf("token source = %q, want mixed", req.TokenCountSource)
	}
}

func TestFillEstimatedUsagePreservesReportedInput(t *testing.T) {
	req := &Request{TokenUsage: domain.TokenUsage{PromptTokens: 544}}
	fillEstimatedUsage(req, 24)
	if req.TokenUsage.PromptTokens != 544 || req.TokenUsage.CompletionTokens != 8 {
		t.Fatalf("usage = %+v, want upstream prompt=544 and estimated completion=8", req.TokenUsage)
	}
	if req.TokenCountSource != domain.TokenUsageSourceMixed {
		t.Fatalf("token source = %q, want mixed", req.TokenCountSource)
	}
}

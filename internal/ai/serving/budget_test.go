package serving

import (
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

func TestRetryBudgetNormalizeDefaults(t *testing.T) {
	got := (RetryBudget{}).Normalize(&Request{CapabilityType: domain.CapabilityChat})

	if got.MaxAttempts != 3 {
		t.Fatalf("MaxAttempts = %d, want 3", got.MaxAttempts)
	}
	if got.MaxElapsed != 15*time.Minute {
		t.Fatalf("MaxElapsed = %s, want 15m", got.MaxElapsed)
	}
}

func TestRetryBudgetHonorsExplicitImageLimit(t *testing.T) {
	got := RetryBudget{MaxAttempts: 2}.Normalize(&Request{
		CapabilityType: domain.CapabilityImage,
	})

	if got.MaxAttempts != 2 {
		t.Fatalf("MaxAttempts = %d, want 2", got.MaxAttempts)
	}
	if got.MaxElapsed != 15*time.Minute {
		t.Fatalf("MaxElapsed = %s, want 15m", got.MaxElapsed)
	}
}

func TestRetryBudgetDoesNotExpandForCandidatesOrCredentials(t *testing.T) {
	got := RetryBudget{MaxAttempts: 1}.Normalize(&Request{
		Candidates: []*domain.RouteCandidate{
			{RouteID: "direct-1"},
			{RouteID: "pool-1", PoolID: "pool-1"},
			{RouteID: "direct-2"},
		},
	})
	if got.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts = %d, want 1", got.MaxAttempts)
	}
}

func TestRetryBudgetCapsConfiguredRequestAmplification(t *testing.T) {
	got := RetryBudget{MaxAttempts: 99}.Normalize(&Request{
		Candidates: []*domain.RouteCandidate{{RouteID: "direct-1"}},
	})
	if got.MaxAttempts != maxUpstreamAttempts {
		t.Fatalf("MaxAttempts = %d, want %d", got.MaxAttempts, maxUpstreamAttempts)
	}
}

func TestRequestLeaseTTLUsesRequestDeadlineInsteadOfSummingAttempts(t *testing.T) {
	got := RequestLeaseTTL(&Request{CapabilityType: domain.CapabilityChat, Candidates: []*domain.RouteCandidate{
		{RouteID: "fast", Timeouts: domain.RouteTimeouts{MaxDuration: 2 * time.Minute}},
		{RouteID: "slow", Timeouts: domain.RouteTimeouts{MaxDuration: 25 * time.Minute}},
	}})
	if got != 17*time.Minute {
		t.Fatalf("lease TTL = %s, want 17m", got)
	}
}

func TestRequestLeaseTTLUsesSystemDeadline(t *testing.T) {
	got := RequestLeaseTTL(&Request{CapabilityType: domain.CapabilityImage, Candidates: []*domain.RouteCandidate{
		{RouteID: "pool", PoolID: "pool-1", Timeouts: domain.RouteTimeouts{MaxDuration: 20 * time.Minute}},
		{RouteID: "direct", Timeouts: domain.RouteTimeouts{MaxDuration: 3 * time.Minute}},
	}})
	if got != 17*time.Minute {
		t.Fatalf("lease TTL = %s, want 17m", got)
	}
}

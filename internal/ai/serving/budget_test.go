package serving

import (
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

func TestRetryBudgetApplyRequestFloorDefaults(t *testing.T) {
	got := (RetryBudget{}).ApplyRequestFloor(&Request{CapabilityType: domain.CapabilityChat})

	if got.MaxAttempts != 3 {
		t.Fatalf("MaxAttempts = %d, want 3", got.MaxAttempts)
	}
	if got.MaxElapsed != 15*time.Minute {
		t.Fatalf("MaxElapsed = %s, want 15m", got.MaxElapsed)
	}
}

func TestRetryBudgetApplyRequestFloorKeepsImageAttemptFloor(t *testing.T) {
	got := RetryBudget{MaxAttempts: 2}.ApplyRequestFloor(&Request{
		CapabilityType: domain.CapabilityImage,
	})

	if got.MaxAttempts != 3 {
		t.Fatalf("MaxAttempts = %d, want 3", got.MaxAttempts)
	}
	if got.MaxElapsed != 15*time.Minute {
		t.Fatalf("MaxElapsed = %s, want 15m", got.MaxElapsed)
	}
}

func TestRetryBudgetCoversRoutePlanAndOnePoolCredentialSwap(t *testing.T) {
	got := RetryBudget{MaxAttempts: 1}.ApplyRequestFloor(&Request{
		Candidates: []*domain.RouteCandidate{
			{RouteID: "direct-1"},
			{RouteID: "pool-1", PoolID: "pool-1"},
			{RouteID: "direct-2"},
		},
	})
	if got.MaxAttempts != 4 {
		t.Fatalf("MaxAttempts = %d, want 4", got.MaxAttempts)
	}
}

func TestRetryBudgetCapsConfiguredRequestAmplification(t *testing.T) {
	got := RetryBudget{MaxAttempts: 99}.ApplyRequestFloor(&Request{
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

package commercial

import (
	"testing"

	"xiaodou/dai/internal/ai/core/routing"
)

func TestNormalizeGroupWriteDefaults(t *testing.T) {
	t.Parallel()

	got, err := normalizeGroupWrite(GroupWrite{
		Code:              "chat-basic",
		RetailPriceBookID: "pb_1",
	})
	if err != nil {
		t.Fatalf("normalizeGroupWrite: %v", err)
	}
	if got.Name != "chat-basic" {
		t.Fatalf("name = %q", got.Name)
	}
	if got.Code != "chat-basic" {
		t.Fatalf("code = %q", got.Code)
	}
	if got.Status != StatusActive {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestNormalizeDispatchRuleRejectsUnknownSurface(t *testing.T) {
	t.Parallel()

	_, err := normalizeDispatchRuleWrite(DispatchRuleWrite{
		ClientSurface: "unknown_surface",
		MatchType:     DispatchMatchExact,
		MatchValue:    "opus-4.7",
		TargetModelID: "gpt-5.4",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNormalizeLimitPolicyRejectsNegativeConcurrency(t *testing.T) {
	t.Parallel()

	concurrency := -1
	_, err := normalizeLimitPolicyWrite(LimitPolicyWrite{
		ScopeType:        LimitScopeTenant,
		ScopeID:          "tenant_1",
		ConcurrencyLimit: &concurrency,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNormalizeRoutingPolicyWriteDefaultsGlobalScope(t *testing.T) {
	t.Parallel()

	got, err := normalizeRoutingPolicyWrite(RoutingPolicyWrite{
		ScopeType: routing.ScopeGlobal,
		Weights: routing.WeightSet{
			Cost:    0.4,
			Latency: 0.3,
			Load:    0.2,
			Health:  0.1,
		},
	})
	if err != nil {
		t.Fatalf("normalizeRoutingPolicyWrite: %v", err)
	}
	if got.ScopeID != "global" {
		t.Fatalf("scope_id = %q", got.ScopeID)
	}
}

func TestNormalizeRoutingPolicyWriteRejectsZeroWeights(t *testing.T) {
	t.Parallel()

	_, err := normalizeRoutingPolicyWrite(RoutingPolicyWrite{
		ScopeType: routing.ScopeGlobal,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

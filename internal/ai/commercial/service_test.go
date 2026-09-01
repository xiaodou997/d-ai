package commercial

import (
	"math"
	"testing"
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

func TestNormalizeGroupWriteDefaultsRoutePolicy(t *testing.T) {
	t.Parallel()

	got, err := normalizeGroupWrite(GroupWrite{
		Name:              "primary",
		RetailPriceBookID: "pb_1",
		RoutePolicy:       RoutePolicyCost,
	})
	if err != nil {
		t.Fatalf("normalizeGroupWrite: %v", err)
	}
	if got.RoutePolicy != RoutePolicyCost {
		t.Fatalf("route policy = %q, want %q", got.RoutePolicy, RoutePolicyCost)
	}
}

func TestNormalizeGroupWriteRejectsNonFinitePolicyNumbers(t *testing.T) {
	t.Parallel()

	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), maxGroupPolicyNumber + 1} {
		if _, err := normalizeGroupWrite(GroupWrite{
			Name:                  "invalid",
			RetailPriceBookID:     "pb_1",
			DefaultUserMultiplier: value,
		}); err == nil {
			t.Fatalf("normalizeGroupWrite accepted default multiplier %v", value)
		}
	}
}

func TestNormalizeGroupWriteRejectsNegativeRoutePolicyVersion(t *testing.T) {
	t.Parallel()

	if _, err := normalizeGroupWrite(GroupWrite{
		Name: "stale", RetailPriceBookID: "pb_1", ExpectedRoutePolicyVersion: -1,
	}); err == nil {
		t.Fatal("normalizeGroupWrite accepted a negative route policy version")
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

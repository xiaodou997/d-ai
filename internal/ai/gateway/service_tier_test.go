package gateway

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestParseServiceTier_DefaultsToStandard(t *testing.T) {
	tier, err := parseServiceTier([]byte(`{"model":"gpt-5.5"}`), "application/json")
	if err != nil {
		t.Fatalf("parse service tier: %v", err)
	}
	if tier != domain.ServiceTierStandard {
		t.Fatalf("tier = %s, want standard", tier)
	}
}

func TestParseServiceTier_PriorityIsFast(t *testing.T) {
	tier, err := parseServiceTier([]byte(`{"model":"gpt-5.5","service_tier":"priority"}`), "application/json")
	if err != nil {
		t.Fatalf("parse service tier: %v", err)
	}
	if tier != domain.ServiceTierFast {
		t.Fatalf("tier = %s, want fast", tier)
	}
}

func TestParseServiceTier_Invalid(t *testing.T) {
	if _, err := parseServiceTier([]byte(`{"model":"gpt-5.5","service_tier":"premium"}`), "application/json"); err == nil {
		t.Fatal("expected invalid service tier error")
	}
}

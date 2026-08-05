package domain

import (
	"testing"
	"time"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
)

func TestCapabilityToCore(t *testing.T) {
	if got := CapabilityImage.ToCore(); got != catalog.CapabilityImageGeneration {
		t.Fatalf("CapabilityImage.ToCore() = %s", got)
	}
	if got := CapabilityChat.ToCore(); got != catalog.CapabilityChat {
		t.Fatalf("CapabilityChat.ToCore() = %s", got)
	}
}

func TestAPIKeyToCore(t *testing.T) {
	now := time.Now()
	limit := int64(100)
	in := APIKey{
		ID:              "k1",
		OwnerType:       OwnerTenant,
		TenantID:        "t1",
		LastFour:        "1234",
		Name:            "main",
		QuotaLimitMicro: &limit,
		Status:          APIKeyStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	out := in.ToCore()
	if out.OwnerScope != identity.ScopeTenant {
		t.Fatalf("OwnerScope = %s", out.OwnerScope)
	}
	if out.QuotaLimitMicro == nil || *out.QuotaLimitMicro != 100 {
		t.Fatalf("QuotaLimitMicro = %#v", out.QuotaLimitMicro)
	}
}

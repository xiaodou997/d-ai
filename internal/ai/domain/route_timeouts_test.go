package domain

import (
	"testing"
	"time"
)

func TestDefaultRouteTimeoutsUsesSystemPolicyForEveryCapability(t *testing.T) {
	t.Parallel()

	for _, capability := range []CapabilityType{CapabilityChat, CapabilityImage, CapabilityVideo} {
		got := DefaultRouteTimeouts(capability)
		if got.ResponseHeader != 5*time.Minute {
			t.Fatalf("%s response-header timeout = %s, want 5m", capability, got.ResponseHeader)
		}
		if got.FirstByte != 5*time.Minute {
			t.Fatalf("%s first-byte timeout = %s, want 5m", capability, got.FirstByte)
		}
		if got.Idle != 3*time.Minute {
			t.Fatalf("%s idle timeout = %s, want 3m", capability, got.Idle)
		}
		if got.MaxDuration != 15*time.Minute {
			t.Fatalf("%s max-duration timeout = %s, want 15m", capability, got.MaxDuration)
		}
	}
}

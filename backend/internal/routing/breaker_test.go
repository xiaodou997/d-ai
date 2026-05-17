package routing

import (
	"testing"
	"time"
)

func TestCircuitBreakerTripsAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(BreakerConfig{FailureThreshold: 3, Cooldown: time.Second})
	const id = "dep-1"

	for i := 1; i <= 2; i++ {
		if tripped := cb.RecordFailure(id); tripped {
			t.Fatalf("tripped too early on failure %d", i)
		}
		if cb.IsOpen(id) {
			t.Fatalf("circuit open before threshold (failure %d)", i)
		}
	}
	if !cb.RecordFailure(id) {
		t.Fatal("third failure should trip the breaker")
	}
	if !cb.IsOpen(id) {
		t.Fatal("circuit should be open after tripping")
	}
}

func TestCircuitBreakerRecordSuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(BreakerConfig{FailureThreshold: 2, Cooldown: time.Second})
	cb.RecordFailure("d")
	cb.RecordSuccess("d")
	// Next failure should not trip immediately (counter reset to 0).
	if tripped := cb.RecordFailure("d"); tripped {
		t.Fatal("counter should have reset after RecordSuccess")
	}
}

func TestCircuitBreakerCooldownResetsCounter(t *testing.T) {
	cb := NewCircuitBreaker(BreakerConfig{FailureThreshold: 2, Cooldown: 10 * time.Millisecond})
	cb.RecordFailure("d")
	cb.RecordFailure("d") // trips
	if !cb.IsOpen("d") {
		t.Fatal("expected open")
	}
	time.Sleep(20 * time.Millisecond)
	if cb.IsOpen("d") {
		t.Fatal("cooldown expired, IsOpen should be false")
	}
	// IsOpen reset cleared the counter — a single new failure must not re-trip.
	if tripped := cb.RecordFailure("d"); tripped {
		t.Fatal("counter should have been reset by cooldown expiry")
	}
}

func TestCircuitBreakerRecordFailureAfterExpiryResetsCounter(t *testing.T) {
	// Same as above but without calling IsOpen first — RecordFailure itself
	// must notice the expired cooldown.
	cb := NewCircuitBreaker(BreakerConfig{FailureThreshold: 2, Cooldown: 10 * time.Millisecond})
	cb.RecordFailure("d")
	cb.RecordFailure("d") // trips
	time.Sleep(20 * time.Millisecond)
	if tripped := cb.RecordFailure("d"); tripped {
		t.Fatal("first failure after cooldown should not immediately re-trip")
	}
}

func TestCircuitBreakerDefaults(t *testing.T) {
	cb := NewCircuitBreaker(BreakerConfig{})
	if cb.cfg.FailureThreshold != defaultFailureThreshold {
		t.Fatalf("default threshold = %d, want %d", cb.cfg.FailureThreshold, defaultFailureThreshold)
	}
	if cb.cfg.Cooldown != defaultCooldownSeconds*time.Second {
		t.Fatalf("default cooldown wrong: %v", cb.cfg.Cooldown)
	}
}

package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/ai/serving"
)

func newTestLimiter(t *testing.T) (*UpstreamConcurrencyLimiter, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewUpstreamConcurrencyLimiter(client), server
}

func TestUpstreamConcurrencyLimiterCapsInFlightPerAccount(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	ctx := context.Background()
	ttl := time.Minute

	if _, err := limiter.Acquire(ctx, "account-a", "req-1", 2, ttl); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := limiter.Acquire(ctx, "account-a", "req-2", 2, ttl); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	// A different account has its own budget.
	if _, err := limiter.Acquire(ctx, "account-b", "req-3", 2, ttl); err != nil {
		t.Fatalf("other account acquire: %v", err)
	}
	if _, err := limiter.Acquire(ctx, "account-a", "req-4", 2, ttl); !errors.Is(err, serving.ErrUpstreamConcurrencyExceeded) {
		t.Fatalf("third acquire error = %v, want concurrency exceeded", err)
	}
}

func TestUpstreamConcurrencyLimiterReleaseFreesSlot(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	ctx := context.Background()
	ttl := time.Minute

	slot, err := limiter.Acquire(ctx, "account-a", "req-1", 1, ttl)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := limiter.Acquire(ctx, "account-a", "req-2", 1, ttl); !errors.Is(err, serving.ErrUpstreamConcurrencyExceeded) {
		t.Fatalf("acquire while full = %v, want concurrency exceeded", err)
	}

	slot.Release(ctx)

	if _, err := limiter.Acquire(ctx, "account-a", "req-2", 1, ttl); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}

// A retry against the same account under the same request must reuse its slot
// rather than consume a second one, or a single request could exhaust the
// account by itself.
func TestUpstreamConcurrencyLimiterIsIdempotentPerRequest(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	ctx := context.Background()
	ttl := time.Minute

	if _, err := limiter.Acquire(ctx, "account-a", "req-1", 1, ttl); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := limiter.Acquire(ctx, "account-a", "req-1", 1, ttl); err != nil {
		t.Fatalf("re-acquire under same request id: %v", err)
	}
	if _, err := limiter.Acquire(ctx, "account-a", "req-2", 1, ttl); !errors.Is(err, serving.ErrUpstreamConcurrencyExceeded) {
		t.Fatalf("other request acquire = %v, want concurrency exceeded", err)
	}
}

// A process that dies without releasing must not strand the slot forever.
func TestUpstreamConcurrencyLimiterEvictsExpiredLeases(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	if _, err := limiter.Acquire(ctx, "account-a", "req-1", 1, time.Minute); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := limiter.Acquire(ctx, "account-a", "req-2", 1, time.Minute); !errors.Is(err, serving.ErrUpstreamConcurrencyExceeded) {
		t.Fatalf("acquire while full = %v, want concurrency exceeded", err)
	}

	// Past the lease expiry the stranded slot is reclaimed.
	now = now.Add(time.Minute + time.Millisecond)
	if _, err := limiter.Acquire(ctx, "account-a", "req-2", 1, time.Minute); err != nil {
		t.Fatalf("acquire after lease expiry: %v", err)
	}
}

func TestUpstreamConcurrencyLimiterUnlimitedAccountNeedsNoSlot(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	slot, err := limiter.Acquire(context.Background(), "account-a", "req-1", 0, time.Minute)
	if err != nil {
		t.Fatalf("acquire with no limit: %v", err)
	}
	if slot != nil {
		t.Fatalf("slot = %#v, want nil for an unlimited account", slot)
	}
}

func TestUpstreamConcurrencyLimiterFailsClosedWithoutRedis(t *testing.T) {
	limiter := NewUpstreamConcurrencyLimiter(nil)
	_, err := limiter.Acquire(context.Background(), "account-a", "req-1", 10, time.Minute)
	if !errors.Is(err, serving.ErrUpstreamConcurrencyLimiterUnavailable) {
		t.Fatalf("error = %v, want limiter unavailable", err)
	}
}

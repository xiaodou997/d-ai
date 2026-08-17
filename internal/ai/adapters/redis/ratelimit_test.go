package redis

import (
	"context"
	"errors"
	"fmt"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	goredis "github.com/redis/go-redis/v9"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/dbtest"
)

func newRateLimiter(t *testing.T) (*RateLimiter, context.Context) {
	t.Helper()
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("rate limit test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewRateLimiter(client, dbgen.New(pool)), ctx
}

func limitRequest(requestID, tenantID, userID string) *serving.Request {
	subject := &coreidentity.Subject{
		AuthMethod: coreidentity.AuthMethodJWT,
		Scope:      coreidentity.ScopeUser,
		TenantID:   tenantID,
		UserID:     userID,
	}
	if userID == "" {
		subject.Scope = coreidentity.ScopeTenant
	}
	return &serving.Request{Subject: subject, RequestID: requestID}
}

// Settlement is post-paid, so admission cannot stop a request that is already
// running — it can only stop the next one. The in-flight cap is therefore the
// only thing that turns "how far can an account overdraw" into a finite number.
func TestDefaultInFlightCapBoundsConcurrentBilledRequests(t *testing.T) {
	limiter, ctx := newRateLimiter(t)
	limiter.WithDefaultInFlight(2)

	var leases []serving.RateLimitLease
	for i := range 2 {
		lease, err := limiter.Acquire(ctx, limitRequest(fmt.Sprintf("req-%d", i), "t1", "u1"))
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		leases = append(leases, lease)
	}

	_, err := limiter.Acquire(ctx, limitRequest("req-over", "t1", "u1"))
	if !errors.Is(err, serving.ErrRateLimitExceeded) {
		t.Fatalf("error = %v, want ErrRateLimitExceeded once the cap is reached", err)
	}

	// Releasing one frees the slot, so the cap throttles rather than locks out.
	leases[0].Release(ctx)
	if _, err := limiter.Acquire(ctx, limitRequest("req-after-release", "t1", "u1")); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}

func TestDefaultInFlightCapIsPerAccount(t *testing.T) {
	limiter, ctx := newRateLimiter(t)
	limiter.WithDefaultInFlight(1)

	if _, err := limiter.Acquire(ctx, limitRequest("a1", "t1", "u1")); err != nil {
		t.Fatalf("first user acquire: %v", err)
	}
	// A different end user under the same tenant has its own user budget, but
	// shares the tenant budget — which is 1 here, so this is refused by the
	// tenant scope.
	if _, err := limiter.Acquire(ctx, limitRequest("b1", "t1", "u2")); !errors.Is(err, serving.ErrRateLimitExceeded) {
		t.Fatalf("second user under a saturated tenant: err = %v, want refused", err)
	}
	// A different tenant is unaffected.
	if _, err := limiter.Acquire(ctx, limitRequest("c1", "t2", "u3")); err != nil {
		t.Fatalf("other tenant acquire: %v", err)
	}
}

// An operator who has deliberately configured a limit must get exactly that
// limit, not that limit plus an implicit one.
func TestExplicitPolicyReplacesTheDefault(t *testing.T) {
	limiter, ctx := newRateLimiter(t)
	limiter.WithDefaultInFlight(1)

	if _, err := limiter.q.CreateLimitPolicy(ctx, dbgen.CreateLimitPolicyParams{
		ScopeType:        "tenant",
		ScopeID:          "t1",
		ConcurrencyLimit: pgtype.Int4{Int32: 3, Valid: true},
		Status:           "active",
	}); err != nil {
		t.Fatalf("seed runtime limit policy: %v", err)
	}

	for i := range 3 {
		if _, err := limiter.Acquire(ctx, limitRequest(fmt.Sprintf("p-%d", i), "t1", "")); err != nil {
			t.Fatalf("acquire %d under explicit limit 3: %v", i, err)
		}
	}
	if _, err := limiter.Acquire(ctx, limitRequest("p-over", "t1", "")); !errors.Is(err, serving.ErrRateLimitExceeded) {
		t.Fatalf("error = %v, want refusal at the configured limit", err)
	}
}

// Zero restores the previous unbounded behaviour, which is also unbounded
// overdraft — kept configurable, but never the default.
func TestZeroDefaultDisablesTheCap(t *testing.T) {
	limiter, ctx := newRateLimiter(t)
	limiter.WithDefaultInFlight(0)

	for i := range 50 {
		if _, err := limiter.Acquire(ctx, limitRequest(fmt.Sprintf("u-%d", i), "t1", "u1")); err != nil {
			t.Fatalf("acquire %d with the cap disabled: %v", i, err)
		}
	}
}

// Untenanted internal traffic is not billed and must not be throttled by a cap
// whose only purpose is bounding a balance.
func TestUntenantedTrafficIsNotCapped(t *testing.T) {
	limiter, ctx := newRateLimiter(t)
	limiter.WithDefaultInFlight(1)

	for i := range 5 {
		lease, err := limiter.Acquire(ctx, limitRequest(fmt.Sprintf("i-%d", i), "", ""))
		if err != nil {
			t.Fatalf("internal acquire %d: %v", i, err)
		}
		if lease != nil {
			t.Fatal("internal traffic should hold no lease")
		}
	}
}

package routing

import (
	"context"
	"errors"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"sync"
	"testing"
	"time"
)

func availabilityFixture(t *testing.T) (*RedisAvailability, *miniredis.Miniredis, FaultScope, time.Time) {
	t.Helper()
	server := miniredis.RunT(t)
	now := time.Now().UTC().Truncate(time.Second)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisAvailability(client), server, NewFaultScope("model", "direct_upstream", "account", "endpoint", "", "model", "responses"), now
}
func failure(t *testing.T, s *RedisAvailability, scope FaultScope) {
	t.Helper()
	p, e := s.Acquire(context.Background(), []FaultScope{scope}, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Complete(context.Background(), p, AvailabilityOutcome{FailureScope: scope.Key, HealthFailure: true, Reason: "server_error"})
	if e != nil {
		t.Fatal(e)
	}
}
func TestAvailabilityAutomaticRecoveryAndTwoSuccesses(t *testing.T) {
	s, server, scope, now := availabilityFixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		failure(t, s, scope)
	}
	states, e := s.Read(ctx, []FaultScope{scope})
	if e != nil || states[scope.Key].Phase != Cooling {
		t.Fatalf("states=%+v err=%v", states, e)
	}
	server.SetTime(now.Add(31 * time.Second))
	states, e = s.Read(ctx, []FaultScope{scope})
	if e != nil || states[scope.Key].Phase != Recovering {
		t.Fatalf("expiry must be visible without admission: %+v %v", states, e)
	}
	for i := 0; i < 2; i++ {
		p, e := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Complete(ctx, p, AvailabilityOutcome{Success: true}); e != nil {
			t.Fatal(e)
		}
	}
	states, _ = s.Read(ctx, []FaultScope{scope})
	if states[scope.Key].Phase != Available || states[scope.Key].Trips != 1 {
		t.Fatalf("premature backoff reset: %+v", states)
	}
}
func TestAvailabilityDistributedLeaseAndNeutralRelease(t *testing.T) {
	s, server, scope, now := availabilityFixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		failure(t, s, scope)
	}
	server.SetTime(now.Add(time.Minute))
	var mu sync.Mutex
	var winner *AdmissionPermit
	count := 0
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
			if e == nil {
				mu.Lock()
				count++
				winner = p
				mu.Unlock()
			} else {
				var denied *AdmissionDenied
				if !errors.As(e, &denied) {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
	if count != 1 {
		t.Fatalf("admitted %d", count)
	}
	_, e := s.Complete(ctx, winner, AvailabilityOutcome{})
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	s.Complete(ctx, winner, AvailabilityOutcome{Success: true})
	if _, e = s.Acquire(ctx, []FaultScope{scope}, time.Minute); e == nil {
		t.Fatal("old completion released new owner")
	}
	s.Complete(ctx, next, AvailabilityOutcome{})
}
func TestAvailabilityFencesLateSuccessAndExpiredOwner(t *testing.T) {
	s, server, scope, now := availabilityFixture(t)
	ctx := context.Background()
	old, e := s.Acquire(ctx, []FaultScope{scope}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 5; i++ {
		failure(t, s, scope)
	}
	s.Complete(ctx, old, AvailabilityOutcome{Success: true})
	states, _ := s.Read(ctx, []FaultScope{scope})
	if states[scope.Key].Phase != Cooling {
		t.Fatal("late success erased cooling")
	}
	server.SetTime(now.Add(time.Minute))
	old, _ = s.Acquire(ctx, []FaultScope{scope}, time.Second)
	server.SetTime(now.Add(time.Minute + 2*time.Second))
	fresh, e := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	s.Complete(ctx, old, AvailabilityOutcome{Success: true})
	if _, e = s.Acquire(ctx, []FaultScope{scope}, time.Minute); e == nil {
		t.Fatal("expired owner erased current lease")
	}
	s.Complete(ctx, fresh, AvailabilityOutcome{})
}
func TestAvailabilityScopeIsolationAndAuthenticationCooldown(t *testing.T) {
	s, server, a, now := availabilityFixture(t)
	ctx := context.Background()
	b := NewFaultScope("model", "direct_upstream", "account", "endpoint", "", "other", "responses")
	for i := 0; i < 5; i++ {
		failure(t, s, a)
	}
	p, e := s.Acquire(ctx, []FaultScope{b}, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	s.Complete(ctx, p, AvailabilityOutcome{Success: true})
	states, _ := s.Read(ctx, []FaultScope{a, b})
	if states[a.Key].Phase != Cooling || states[b.Key].Phase != Available {
		t.Fatal(states)
	}
	auth := NewFaultScope("authentication", "direct_upstream", "account", "", "", "", "")
	p, _ = s.Acquire(ctx, []FaultScope{auth}, time.Minute)
	s.Complete(ctx, p, AvailabilityOutcome{Authentication: true, FailureScope: auth.Key, Reason: "unauthorized"})
	server.SetTime(now.Add(29 * time.Minute))
	if _, e = s.Acquire(ctx, []FaultScope{auth}, time.Minute); e == nil {
		t.Fatal("auth resumed early")
	}
	server.SetTime(now.Add(31 * time.Minute))
	if _, e = s.Acquire(ctx, []FaultScope{auth}, time.Minute); e != nil {
		t.Fatal(e)
	}
}
func TestAvailabilityFailureRateAndRetryAfter(t *testing.T) {
	s, server, scope, now := availabilityFixture(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		p, e := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		s.Complete(ctx, p, AvailabilityOutcome{Success: i%2 == 0, FailureScope: scope.Key, HealthFailure: true})
	}
	states, _ := s.Read(ctx, []FaultScope{scope})
	if states[scope.Key].Phase != Cooling {
		t.Fatal("50 percent failure window did not cool")
	}
	server.SetTime(now.Add(time.Minute))
	p, _ := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
	s.Complete(ctx, p, AvailabilityOutcome{FailureScope: scope.Key, Reason: "rate_limited", CooldownUntil: now.Add(time.Hour).UnixMilli()})
	states, _ = s.Read(ctx, []FaultScope{scope})
	if states[scope.Key].RetryAt != now.Add(time.Hour).UnixMilli() {
		t.Fatal(states)
	}
}
func TestAvailabilityUnavailableIsNotCooling(t *testing.T) {
	s := NewRedisAvailability(nil)
	_, e := s.Acquire(context.Background(), nil, time.Minute)
	if !errors.Is(e, ErrAvailabilityUnavailable) {
		t.Fatal(e)
	}
}

func TestAvailabilityBackoffCapStableResetAndRecoveryRotation(t *testing.T) {
	s, server, scope, now := availabilityFixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		failure(t, s, scope)
	}
	for _, delay := range []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute} {
		states, err := s.Read(ctx, []FaultScope{scope})
		if err != nil {
			t.Fatal(err)
		}
		state := states[scope.Key]
		if state.RetryAt != now.Add(delay).UnixMilli() {
			t.Fatalf("delay=%d want=%v", state.RetryAt-now.UnixMilli(), delay)
		}
		now = now.Add(delay)
		server.SetTime(now)
		failure(t, s, scope)
	}
	states, _ := s.Read(ctx, []FaultScope{scope})
	now = time.UnixMilli(states[scope.Key].RetryAt)
	server.SetTime(now)
	for i := 0; i < 2; i++ {
		p, e := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Complete(ctx, p, AvailabilityOutcome{Success: true}); e != nil {
			t.Fatal(e)
		}
	}
	server.SetTime(now.Add(11 * time.Minute))
	for i := 0; i < 10; i++ {
		p, e := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		s.Complete(ctx, p, AvailabilityOutcome{Success: true})
	}
	states, _ = s.Read(ctx, []FaultScope{scope})
	if states[scope.Key].Trips != 0 {
		t.Fatal("stable window did not reset backoff", states)
	}
	counts := map[string]int{}
	for i := 0; i < 30; i++ {
		id, e := s.RecoveryTurn(ctx, "tier", []string{"a", "b", "c"}, true)
		if e != nil {
			t.Fatal(e)
		}
		counts[id]++
	}
	if counts[""] != 27 || counts["a"] != 1 || counts["b"] != 1 || counts["c"] != 1 {
		t.Fatal(counts)
	}
}

func TestAvailabilityConcurrentFaultScopesUseLatestEligibility(t *testing.T) {
	s, _, model, now := availabilityFixture(t)
	ctx := context.Background()
	auth := NewFaultScope("authentication", "direct_upstream", "account", "", "", "", "")
	if err := s.Suspend(ctx, model, now.Add(time.Minute), "model_error"); err != nil {
		t.Fatal(err)
	}
	if err := s.Suspend(ctx, auth, now.Add(30*time.Minute), "unauthorized"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Acquire(ctx, []FaultScope{model, auth}, time.Minute)
	var denied *AdmissionDenied
	if !errors.As(err, &denied) || denied.RetryAt != now.Add(30*time.Minute).UnixMilli() {
		t.Fatal(err)
	}
}

func TestCanceledAcquireCannotLeaveOrRecreateARecoveryLease(t *testing.T) {
	s, _, scope, _ := availabilityFixture(t)
	ctx := context.Background()
	if err := s.Reset(ctx, []FaultScope{scope}); err != nil {
		t.Fatal(err)
	}
	token := "lost-ack"
	if _, err := s.run(ctx, "acquire", []FaultScope{scope}, token, time.Minute, AvailabilityOutcome{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.run(ctx, "cancel", nil, token, time.Minute, AvailabilityOutcome{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.run(ctx, "acquire", []FaultScope{scope}, token, time.Minute, AvailabilityOutcome{}); err != nil {
		t.Fatal(err)
	}
	p, err := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
	if err != nil {
		t.Fatal("canceled acquire left a lease", err)
	}
	s.Complete(ctx, p, AvailabilityOutcome{})
}
func TestAdminResumeOnlyLiftsCooldownWithoutClearingHealthyOrBusyState(t *testing.T) {
	s, _, scope, _ := availabilityFixture(t)
	ctx := context.Background()
	p, err := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	s.Complete(ctx, p, AvailabilityOutcome{Success: true})
	if err = s.Resume(ctx, scope.ResourceKind, scope.ResourceID); err != nil {
		t.Fatal(err)
	}
	states, _ := s.Read(ctx, []FaultScope{scope})
	if states[scope.Key].Phase != Available {
		t.Fatal("resume changed a healthy scope")
	}
	if err = s.Reset(ctx, []FaultScope{scope}); err != nil {
		t.Fatal(err)
	}
	p, err = s.Acquire(ctx, []FaultScope{scope}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Resume(ctx, scope.ResourceKind, scope.ResourceID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Acquire(ctx, []FaultScope{scope}, time.Minute); err == nil {
		t.Fatal("resume stole a live recovery lease")
	}
	s.Complete(ctx, p, AvailabilityOutcome{})
}

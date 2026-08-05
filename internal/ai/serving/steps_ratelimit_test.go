package serving

import (
	"context"
	"errors"
	"testing"
)

type fakeRateLimitLease struct {
	releases int
}

func (l *fakeRateLimitLease) Release(context.Context) { l.releases++ }

type fakeRateLimiter struct {
	lease RateLimitLease
	err   error
}

func (f fakeRateLimiter) Acquire(context.Context, *Request) (RateLimitLease, error) {
	return f.lease, f.err
}

func TestRateLimitLeaseReleasedExactlyOnce(t *testing.T) {
	lease := &fakeRateLimitLease{}
	step := &RateLimitStep{Limiter: fakeRateLimiter{lease: lease}}
	req := &Request{}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	step.Rollback(context.Background(), req)
	(RateLimitFinalizer{}).Finalize(context.Background(), req)
	if lease.releases != 1 {
		t.Fatalf("releases = %d, want 1", lease.releases)
	}
}

func TestRateLimiterUnavailableReturns503(t *testing.T) {
	step := &RateLimitStep{Limiter: fakeRateLimiter{err: ErrRateLimiterUnavailable}}
	err := step.Execute(context.Background(), &Request{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 503 || apiErr.Code != "rate_limiter_unavailable" {
		t.Fatalf("error = %#v, want rate_limiter_unavailable 503", err)
	}
}

func TestRateLimitExceededReturns429(t *testing.T) {
	step := &RateLimitStep{Limiter: fakeRateLimiter{err: errors.New("redis key tenant:secret rate limit exceeded")}}
	err := step.Execute(context.Background(), &Request{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 || apiErr.Code != "rate_limit_exceeded" {
		t.Fatalf("error = %#v, want rate_limit_exceeded 429", err)
	}
	if apiErr.Message != "request rate limit exceeded" {
		t.Fatalf("public message = %q, want sanitized rate-limit message", apiErr.Message)
	}
}

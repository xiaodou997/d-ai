package serving

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

type billingSnapshotResolverFunc func(context.Context, *Request, *domain.RouteCandidate) (domain.BillingSnapshot, error)

func (f billingSnapshotResolverFunc) PrepareBilling(ctx context.Context, req *Request, candidate *domain.RouteCandidate) (domain.BillingSnapshot, error) {
	return f(ctx, req, candidate)
}

func billedGuardRequest(candidates ...*domain.RouteCandidate) *Request {
	req := &Request{
		Subject:        &coreidentity.Subject{TenantID: "tenant-1"},
		Candidates:     candidates,
		UsedCandidates: map[string]bool{},
	}
	if len(candidates) > 0 {
		req.SetCandidate(candidates[0])
	}
	return req
}

func TestBillingGuardFiltersUnpricedFallbacks(t *testing.T) {
	unpriced := &domain.RouteCandidate{RouteID: "route-unpriced", ModelCode: "model-1"}
	priced := &domain.RouteCandidate{RouteID: "route-priced", ModelCode: "model-1"}
	req := billedGuardRequest(unpriced, priced)
	req.StickyHit = true
	step := &BillingGuardStep{Resolver: billingSnapshotResolverFunc(func(_ context.Context, _ *Request, candidate *domain.RouteCandidate) (domain.BillingSnapshot, error) {
		if candidate.RouteID == unpriced.RouteID {
			return domain.BillingSnapshot{}, domain.ErrNotFound
		}
		return domain.BillingSnapshot{}, nil
	})}

	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("guard execute: %v", err)
	}
	if len(req.Candidates) != 1 || req.Candidates[0].RouteID != priced.RouteID {
		t.Fatalf("billable candidates = %#v", req.Candidates)
	}
	if req.Candidate == nil || req.Candidate.RouteID != priced.RouteID {
		t.Fatalf("selected candidate = %#v", req.Candidate)
	}
	if _, ok := req.BillingSnapshots[priced.RouteID]; !ok {
		t.Fatalf("priced route snapshot missing: %#v", req.BillingSnapshots)
	}
	if req.StickyHit {
		t.Fatal("sticky hit must be cleared when the sticky route is filtered")
	}
}

func TestBillingGuardRejectsWhenEveryRouteIsUnpriced(t *testing.T) {
	req := billedGuardRequest(&domain.RouteCandidate{RouteID: "route-1", ModelCode: "model-1"})
	step := &BillingGuardStep{Resolver: billingSnapshotResolverFunc(func(context.Context, *Request, *domain.RouteCandidate) (domain.BillingSnapshot, error) {
		return domain.BillingSnapshot{}, domain.ErrNotFound
	})}

	err := step.Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusPaymentRequired || apiErr.Code != "no_price_configured" {
		t.Fatalf("error = %#v, want no_price_configured", err)
	}
}

func TestBillingGuardFailsClosedOnResolverError(t *testing.T) {
	req := billedGuardRequest(&domain.RouteCandidate{RouteID: "route-1", ModelCode: "model-1"})
	step := &BillingGuardStep{Resolver: billingSnapshotResolverFunc(func(context.Context, *Request, *domain.RouteCandidate) (domain.BillingSnapshot, error) {
		return domain.BillingSnapshot{}, errors.New("database unavailable")
	})}

	err := step.Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable || apiErr.Code != "billing_snapshot_failed" {
		t.Fatalf("error = %#v, want billing_snapshot_failed", err)
	}
}

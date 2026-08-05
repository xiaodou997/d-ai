package serving

import (
	"context"
	"errors"
	"net/http"

	"xiaodou/dai/internal/ai/domain"
)

// BillingSnapshotResolver resolves every mutable pricing input before upstream.
type BillingSnapshotResolver interface {
	PrepareBilling(ctx context.Context, req *Request, candidate *domain.RouteCandidate) (domain.BillingSnapshot, error)
}

// BillingGuardStep rejects a request before any upstream call when either the
// group's retail price or the candidate account settlement price is missing. Internal/untenanted calls
// (no TenantID) are not billed and pass through untouched.
type BillingGuardStep struct {
	Resolver BillingSnapshotResolver
}

func (s *BillingGuardStep) Name() string { return "billing_guard" }

func (s *BillingGuardStep) Execute(ctx context.Context, req *Request) error {
	if s.Resolver == nil {
		return nil
	}
	subject := req.RuntimeSubject()
	if subject == nil || subject.TenantID == "" {
		return nil // not a billed request
	}
	if len(req.Candidates) == 0 {
		return apiError(http.StatusServiceUnavailable, "billing_snapshot_failed", "no route candidates available for pricing")
	}
	req.BillingSnapshots = make(map[string]domain.BillingSnapshot, len(req.Candidates))
	billableCandidates := make([]*domain.RouteCandidate, 0, len(req.Candidates))
	sawCandidate := false
	for _, candidate := range req.Candidates {
		if candidate == nil {
			continue
		}
		sawCandidate = true
		snapshot, err := s.Resolver.PrepareBilling(ctx, req, candidate)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return apiError(http.StatusServiceUnavailable, "billing_snapshot_failed", "unable to resolve billing snapshot")
		}
		req.BillingSnapshots[candidate.RouteID] = snapshot
		billableCandidates = append(billableCandidates, candidate)
	}
	if len(billableCandidates) == 0 && sawCandidate {
		return apiError(http.StatusPaymentRequired, "no_price_configured",
			"complete retail and upstream account pricing is required for an available route")
	}
	if len(billableCandidates) == 0 {
		return apiError(http.StatusServiceUnavailable, "billing_snapshot_failed", "no billable route candidates")
	}
	previousCandidate := req.Candidate
	req.Candidates = billableCandidates
	req.UsedCandidates = make(map[string]bool, len(billableCandidates))
	req.SetCandidate(billableCandidates[0])
	if previousCandidate != nil && previousCandidate.RouteID != billableCandidates[0].RouteID {
		req.StickyHit = false
	}
	return nil
}

func (s *BillingGuardStep) Rollback(_ context.Context, _ *Request) {}

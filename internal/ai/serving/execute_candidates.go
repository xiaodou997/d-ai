package serving

import (
	"context"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
)

// pickCandidate returns the next candidate using the group's built-in route
// policy. Health admission happens after local request preparation and
// immediately before the transport call.
func (s *ExecuteStep) pickCandidate(ctx context.Context, req *Request) (*domain.RouteCandidate, float64) {
	req.SelectionReason = ""
	// Sticky is an explicit caller affinity decision. RouteCandidatesStep has
	// already validated the binding and BillingGuard has kept Candidate aligned
	// with the filtered list, so honor it for the first attempt before applying
	// structural tiers and dynamic scoring.
	if req.StickyHit && len(req.Attempts) == 0 && req.Candidate != nil && !req.UsedCandidates[req.Candidate.RouteID] {
		candidate := req.Candidate
		req.ModelCode = candidate.ModelCode
		req.SelectionReason = "sticky"
		return candidate, 0
	}
	// A lower-ranked group is a failover boundary. Within the active group,
	// protocol conversion preference is the only hard compatibility boundary;
	// the group route policy chooses among the remaining targets.
	tier := activeBucketTier(
		activeGroupTier(req.Candidates, req.UsedCandidates),
		req.UsedCandidates,
	)
	if len(tier) == 0 {
		return nil, 0
	}
	var cand *domain.RouteCandidate
	var score float64
	scoring := RouteScoringContext{}
	if subject := req.RuntimeSubject(); subject != nil {
		scoring.TenantID = subject.TenantID
	}
	if sp, ok := s.Scorer.(ScoringPicker); ok {
		cand, score = sp.PickWithScore(ctx, scoring, tier, req.UsedCandidates)
	} else if s.Scorer != nil {
		cand = s.Scorer.Pick(ctx, scoring, tier, req.UsedCandidates)
	} else {
		cand = tier[0]
	}
	if cand == nil {
		return nil, 0
	}
	req.ModelCode = cand.ModelCode
	req.SelectionReason = routeSelectionReason(cand, score, s.Scorer != nil)
	if len(tier) == 1 && s.Scorer != nil {
		req.SelectionReason = "single_candidate"
	}
	return cand, score
}

func routeSelectionReason(candidate *domain.RouteCandidate, score float64, scorerConfigured bool) string {
	if candidate == nil {
		return "unknown"
	}
	if !scorerConfigured {
		return "automatic_fallback"
	}
	switch candidate.RoutePolicy {
	case "balanced", "cost", "latency", "stability":
		return candidate.RoutePolicy
	default:
		if score > 0 {
			return "balanced"
		}
		return "automatic_fallback"
	}
}

// selectPoolCredential resolves the OAuth credential for a pool route. On the
// first attempt of a sticky-bound conversation it reuses the pinned credential
// so the upstream keeps seeing one continuous session; every other case (retry,
// pin no longer usable, no binding) falls back to the pool strategy.
func (s *ExecuteStep) selectPoolCredential(ctx context.Context, req *Request, cand *domain.RouteCandidate) (*domain.OAuthCredential, error) {
	if credID := stickyCredentialID(req, cand); credID != "" {
		if pinner, ok := s.OAuthPool.(PinnedCredentialSelector); ok {
			cred, err := pinner.SelectPinnedCredential(ctx, cand.PoolID, credID)
			if err == nil {
				return cred, nil
			}
			zap.L().Info("sticky credential unusable, falling back to pool selection",
				requestLogFields(req,
					zap.String("pool_id", cand.PoolID),
					zap.String("credential_id", credID),
					zap.Error(err),
				)...,
			)
		}
	}
	return s.OAuthPool.SelectCredentialFromPool(ctx, cand.PoolID, cand.OAuthStrategy)
}

// stickyCredentialID returns the credential this conversation is pinned to for
// the given candidate, or "" when the request is not sticky-bound to it.
func stickyCredentialID(req *Request, cand *domain.RouteCandidate) string {
	if req == nil || cand == nil || len(req.Attempts) > 0 {
		return ""
	}
	b := req.StickyBinding
	if !req.StickyHit || b == nil || b.TargetKind != "credential" {
		return ""
	}
	if b.RouteID != cand.RouteID {
		return ""
	}
	return b.CredentialID
}

func (s *ExecuteStep) candidateBlocked(candidate *domain.RouteCandidate) bool {
	if s.Health == nil || candidate == nil {
		return false
	}
	targetID := candidate.EndpointID
	if candidate.IsPoolRoute() {
		targetID = candidate.PoolID
	}
	return targetID != "" && s.Health.IsBlocked(targetID, candidateProbeLease(candidate))
}

func candidateProbeLease(candidate *domain.RouteCandidate) time.Duration {
	if candidate == nil || candidate.Timeouts.MaxDuration <= 0 {
		return 30 * time.Minute
	}
	return candidate.Timeouts.MaxDuration + 2*time.Minute
}

func (s *ExecuteStep) releaseHealthProbe(candidate *domain.RouteCandidate) {
	if s.Health == nil || candidate == nil {
		return
	}
	targetID, _ := healthTarget(candidate)
	if targetID != "" {
		s.Health.ReleaseProbe(targetID)
	}
}

func exhaustPhysicalTarget(req *Request, failed *domain.RouteCandidate) {
	if req == nil || failed == nil {
		return
	}
	key := physicalTargetKey(failed)
	for _, candidate := range req.Candidates {
		if candidate == nil {
			continue
		}
		if physicalTargetKey(candidate) == key {
			req.UsedCandidates[candidate.RouteID] = true
		}
	}
}

func physicalTargetKey(candidate *domain.RouteCandidate) string {
	if candidate == nil {
		return ""
	}
	if candidate.IsPoolRoute() {
		return "pool:" + candidate.PoolID
	}
	if candidate.EndpointID != "" {
		return "account:" + candidate.EndpointID
	}
	return "route:" + candidate.RouteID
}

// activeGroupTier exposes only the highest-ranked group that still has an
// unused route. A lower-priority group is failover, never a peer in scoring.
func activeGroupTier(candidates []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	minRank := int(^uint(0) >> 1)
	for _, candidate := range candidates {
		if candidate == nil || used[candidate.RouteID] {
			continue
		}
		if candidate.GroupRank < minRank {
			minRank = candidate.GroupRank
		}
	}
	if minRank == int(^uint(0)>>1) {
		return nil
	}
	tier := make([]*domain.RouteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && !used[candidate.RouteID] && candidate.GroupRank == minRank {
			tier = append(tier, candidate)
		}
	}
	return tier
}

// activeBucketTier returns the not-yet-used candidates sharing the lowest
// ConversionBucket. Restricting the scorer to this tier makes zero-conversion
// routes strictly preferred: a higher (more lossy) bucket is only reached once
// every lower-bucket route has been exhausted by failover (marked used).
func activeBucketTier(candidates []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	minBucket := int(^uint(0) >> 1) // max int
	for _, c := range candidates {
		if used[c.RouteID] {
			continue
		}
		if c.ConversionBucket < minBucket {
			minBucket = c.ConversionBucket
		}
	}
	var tier []*domain.RouteCandidate
	for _, c := range candidates {
		if !used[c.RouteID] && c.ConversionBucket == minBucket {
			tier = append(tier, c)
		}
	}
	return tier
}

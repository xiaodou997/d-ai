package serving

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

// pickCandidate returns the next candidate using the group's built-in route
// policy. Health admission happens after local request preparation and
// immediately before the transport call.
func (s *ExecuteStep) pickCandidate(ctx context.Context, req *Request) (*domain.RouteCandidate, float64) {
	req.SelectionReason = ""
	req.SelectionError = nil
	candidates := req.Candidates
	recovery := map[string]bool{}
	if s.Availability != nil {
		var err error
		candidates, recovery, err = s.prepareEligible(ctx, req)
		if err != nil {
			req.SelectionError = err
			return nil, 0
		}
	}
	groupTier := activeGroupTier(candidates, req.UsedCandidates)
	priorityTier, manualPriority := activePriorityTier(groupTier, req.UsedCandidates)
	tier := activeBucketTier(priorityTier, req.UsedCandidates)
	if len(tier) == 0 {
		return nil, 0
	}
	if s.Availability != nil {
		trials := []string{}
		normal := []*domain.RouteCandidate{}
		for _, c := range tier {
			if recovery[c.Key()] {
				trials = append(trials, c.Key())
			} else {
				normal = append(normal, c)
			}
		}
		key := fmt.Sprintf("%s:%d:%d:%s:%s", tier[0].GroupID, tier[0].TargetPriority, tier[0].ConversionBucket, req.StickyModelKey(), req.ClientProtocol)
		trial, err := s.Availability.RecoveryTurn(ctx, key, trials, len(normal) > 0)
		if err != nil {
			req.SelectionError = err
			return nil, 0
		}
		if trial != "" {
			for _, c := range tier {
				if c.Key() == trial {
					req.SelectionReason = "recovery"
					return c, 0
				}
			}
		}
		if len(normal) > 0 {
			tier = normal
		}
	}
	if req.StickyHit && len(req.Attempts) == 0 && req.Candidate != nil {
		for _, c := range tier {
			if c.Key() == req.Candidate.Key() || c.CredentialID != "" && c.CredentialID == stickyCredentialID(req, c) {
				req.SelectionReason = "sticky"
				return c, 0
			}
		}
	}
	var cand *domain.RouteCandidate
	var score float64
	scoring := RouteScoringContext{Stream: req.IsStream}
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
	// The active group still carries unused targets at distinct manual
	// priorities: the tenant's hand-set order decided this attempt, not the
	// policy scorer (the scorer only breaks ties inside one priority level).
	if manualPriority {
		req.SelectionReason = "manual_priority"
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

func candidateProbeLease(candidate *domain.RouteCandidate) time.Duration {
	if candidate == nil || candidate.Timeouts.MaxDuration <= 0 {
		return 30 * time.Minute
	}
	return candidate.Timeouts.MaxDuration + 2*time.Minute
}

func (s *ExecuteStep) releaseAvailability(req *Request) {
	req.AvailabilityResult = routing.AvailabilityOutcome{}
	s.finishAvailability(req)
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
			req.UsedCandidates[candidate.Key()] = true
		}
	}
}

func physicalTargetKey(candidate *domain.RouteCandidate) string {
	if candidate == nil {
		return ""
	}
	if candidate.IsPoolRoute() {
		if candidate.CredentialID != "" {
			return "credential:" + candidate.CredentialID + ":" + candidate.EffectiveUpstreamModel() + ":" + candidate.OperationKey()
		}
		return "pool:" + candidate.PoolID
	}
	if candidate.EndpointID != "" {
		return "endpoint:" + candidate.EndpointID + ":" + candidate.EffectiveUpstreamModel() + ":" + candidate.OperationKey()
	}
	return "route:" + candidate.RouteID
}

// activeGroupTier exposes only the highest-ranked group that still has an
// unused route. A lower-priority group is failover, never a peer in scoring.
func activeGroupTier(candidates []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	minRank := int(^uint(0) >> 1)
	for _, candidate := range candidates {
		if candidate == nil || used[candidate.Key()] {
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
		if candidate != nil && !used[candidate.Key()] && candidate.GroupRank == minRank {
			tier = append(tier, candidate)
		}
	}
	return tier
}

// activePriorityTier returns the not-yet-used candidates sharing the lowest
// manual TargetPriority inside the active group tier. The second return value
// reports whether the group still holds unused targets at more than one
// priority level: when true, the tenant's hand-set order decided this attempt
// (the scorer only breaks ties inside one level) and a worse level is reached
// only after the whole preferred level is exhausted by failover.
func activePriorityTier(candidates []*domain.RouteCandidate, used map[string]bool) ([]*domain.RouteCandidate, bool) {
	minPriority := int(^uint(0) >> 1) // max int
	levels := 0
	seen := make(map[int]bool, len(candidates))
	for _, c := range candidates {
		if c == nil || used[c.Key()] {
			continue
		}
		if !seen[c.TargetPriority] {
			seen[c.TargetPriority] = true
			levels++
		}
		if c.TargetPriority < minPriority {
			minPriority = c.TargetPriority
		}
	}
	if levels == 0 {
		return nil, false
	}
	tier := make([]*domain.RouteCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c != nil && !used[c.Key()] && c.TargetPriority == minPriority {
			tier = append(tier, c)
		}
	}
	return tier, levels > 1
}

// activeBucketTier returns the not-yet-used candidates sharing the lowest
// ConversionBucket. Restricting the scorer to this tier makes zero-conversion
// routes strictly preferred: a higher (more lossy) bucket is only reached once
// every lower-bucket route has been exhausted by failover (marked used).
func activeBucketTier(candidates []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	minBucket := int(^uint(0) >> 1) // max int
	for _, c := range candidates {
		if used[c.Key()] {
			continue
		}
		if c.ConversionBucket < minBucket {
			minBucket = c.ConversionBucket
		}
	}
	var tier []*domain.RouteCandidate
	for _, c := range candidates {
		if !used[c.Key()] && c.ConversionBucket == minBucket {
			tier = append(tier, c)
		}
	}
	return tier
}

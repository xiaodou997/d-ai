package serving

import (
	"context"
	"math"
	"math/rand"
	"sort"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

// ScoreWeights holds the four dimension weights for multi-dim route scoring.
// All weights should sum to 1.0; the scorer normalises if they don't.
type ScoreWeights struct {
	Cost    float64 `json:"cost"`
	Latency float64 `json:"latency"`
	Load    float64 `json:"load"`
	Health  float64 `json:"health"`
}

var DefaultScoreWeights = ScoreWeights{Cost: 0.4, Latency: 0.3, Load: 0.2, Health: 0.1}

// RouteScoringContext carries request-scoped policy dimensions without
// exposing the mutable serving Request to scorer implementations.
type RouteScoringContext struct {
	TenantID string
}

// RouteScorer picks one candidate from the eligible (non-exhausted) pool.
type RouteScorer interface {
	// Pick returns the next candidate to attempt. Returns nil when every
	// candidate in the used map has been exhausted.
	Pick(ctx context.Context, scoring RouteScoringContext, candidates []*domain.RouteCandidate, used map[string]bool) *domain.RouteCandidate
}

// ScoringPicker is an optional extension of RouteScorer implemented by scorers
// that can report the normalised softmax probability of the chosen candidate.
// ExecuteStep uses this to populate AttemptRecord.Score for X-Route-Trace.
type ScoringPicker interface {
	RouteScorer
	PickWithScore(ctx context.Context, scoring RouteScoringContext, candidates []*domain.RouteCandidate, used map[string]bool) (*domain.RouteCandidate, float64)
}

// ScoreWeightsSource fetches effective weights for a request.
type ScoreWeightsSource interface {
	EffectiveWeightsFor(ctx context.Context, scopes []ScoreWeightScope) []ScoreWeights
}

type ScoreWeightScope struct {
	TenantID   string
	GroupID    string
	UpstreamID string
}

// ============================================================================
// MultiDimScorer
// ============================================================================

// MultiDimScorer implements RouteScorer using cost/latency/load/health scoring.
// When Redis stats are unavailable (all zero), it falls back to the simpler
// priority-tier random algorithm.
type MultiDimScorer struct {
	Health  routing.HealthTracker   // optional; nil = no health signals
	Stats   routing.RouteStatsStore // optional; nil = priority-tier fallback
	Weights ScoreWeightsSource      // optional; nil = DefaultScoreWeights
}

// Pick returns the best candidate using multi-dim scoring with softmax sampling,
// or priority-tier random when no route stats are available.
func (s *MultiDimScorer) Pick(ctx context.Context, scoring RouteScoringContext, candidates []*domain.RouteCandidate, used map[string]bool) *domain.RouteCandidate {
	c, _ := s.PickWithScore(ctx, scoring, candidates, used)
	return c
}

// PickWithScore is like Pick but also returns the normalised softmax probability
// (0.0 when stats-based scoring is not active).
func (s *MultiDimScorer) PickWithScore(ctx context.Context, scoring RouteScoringContext, candidates []*domain.RouteCandidate, used map[string]bool) (*domain.RouteCandidate, float64) {
	eligible := filterEligible(candidates, used)
	if len(eligible) == 0 {
		return nil, 0
	}
	if len(eligible) == 1 {
		return eligible[0], 0
	}

	if s.Stats != nil {
		stats := s.Stats.Snapshot(ctx, candidateRouteIDs(eligible))
		if hasRoutingStats(stats) {
			return s.pickMultiDimWithScore(ctx, scoring, eligible, stats)
		}
	}
	return pickPriorityTier(eligible), 0
}

func hasRoutingStats(stats map[string]routing.RouteStats) bool {
	for _, st := range stats {
		if st.EWMALatencyMs > 0 || st.InflightCount > 0 {
			return true
		}
	}
	return false
}

func (s *MultiDimScorer) pickMultiDimWithScore(ctx context.Context, scoring RouteScoringContext, eligible []*domain.RouteCandidate, stats map[string]routing.RouteStats) (*domain.RouteCandidate, float64) {
	scores := s.normalizedCandidateScores(ctx, scoring, eligible, stats)
	return softmaxSampleWithScore(eligible, scores)
}

const softmaxTempDefault = 1.0

func (s *MultiDimScorer) normalizedCandidateScores(ctx context.Context, scoring RouteScoringContext, eligible []*domain.RouteCandidate, stats map[string]routing.RouteStats) []float64 {
	costs := make([]float64, len(eligible))
	latencies := make([]float64, len(eligible))
	loads := make([]float64, len(eligible))
	knownLatencies := make([]float64, 0, len(eligible))
	for i, candidate := range eligible {
		costs[i] = math.Max(0, candidate.CostPer1kTokens)
		stat := stats[candidate.RouteID]
		latencies[i] = stat.EWMALatencyMs
		loads[i] = math.Max(0, float64(stat.InflightCount))
		if stat.EWMALatencyMs > 0 {
			knownLatencies = append(knownLatencies, stat.EWMALatencyMs)
		}
	}
	unknownLatency := median(knownLatencies)
	for i := range latencies {
		if latencies[i] <= 0 {
			latencies[i] = unknownLatency
		}
	}

	costScores := normalizeLowerIsBetter(costs)
	latencyScores := normalizeLowerIsBetter(latencies)
	loadScores := normalizeLowerIsBetter(loads)
	healthScores := s.healthScores(eligible)
	resolvedWeights := s.resolveWeightsFor(ctx, scoring, eligible)
	scores := make([]float64, len(eligible))
	for i := range eligible {
		weights := resolvedWeights[i]
		weightSum := weights.Cost + weights.Latency + weights.Load + weights.Health
		if weightSum <= 0 {
			weights = DefaultScoreWeights
			weightSum = 1
		}
		scores[i] = (weights.Cost*costScores[i] +
			weights.Latency*latencyScores[i] +
			weights.Load*loadScores[i] +
			weights.Health*healthScores[i]) / weightSum
	}
	return scores
}

func (s *MultiDimScorer) healthScores(candidates []*domain.RouteCandidate) []float64 {
	scores := make([]float64, len(candidates))
	if s.Health == nil {
		for i := range scores {
			scores[i] = 1
		}
		return scores
	}
	targetIDs := make([]string, len(candidates))
	for i, candidate := range candidates {
		targetIDs[i], _ = healthTarget(candidate)
	}
	states := s.Health.StatesOf(targetIDs)
	for i, targetID := range targetIDs {
		switch states[targetID] {
		case routing.StateHalfOpen:
			scores[i] = 0.3
		case routing.StateOpen:
			scores[i] = 0
		default:
			scores[i] = 1
		}
	}
	return scores
}

func normalizeLowerIsBetter(values []float64) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 {
		return out
	}
	minValue, maxValue := values[0], values[0]
	for _, value := range values[1:] {
		minValue = math.Min(minValue, value)
		maxValue = math.Max(maxValue, value)
	}
	if maxValue == minValue {
		for i := range out {
			out[i] = 1
		}
		return out
	}
	for i, value := range values {
		out[i] = (maxValue - value) / (maxValue - minValue)
	}
	return out
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 1
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	middle := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[middle]
	}
	return (copyValues[middle-1] + copyValues[middle]) / 2
}

func candidateRouteIDs(candidates []*domain.RouteCandidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil {
			ids = append(ids, candidate.RouteID)
		}
	}
	return ids
}

func (s *MultiDimScorer) resolveWeights(ctx context.Context, scoring RouteScoringContext, candidate *domain.RouteCandidate) ScoreWeights {
	return s.resolveWeightsFor(ctx, scoring, []*domain.RouteCandidate{candidate})[0]
}

func (s *MultiDimScorer) resolveWeightsFor(ctx context.Context, scoring RouteScoringContext, candidates []*domain.RouteCandidate) []ScoreWeights {
	out := make([]ScoreWeights, len(candidates))
	if s.Weights == nil {
		for i := range out {
			out[i] = DefaultScoreWeights
		}
		return out
	}
	scopes := make([]ScoreWeightScope, len(candidates))
	for i, candidate := range candidates {
		upstreamID := candidate.EndpointID
		if candidate.IsPoolRoute() {
			upstreamID = candidate.PoolID
		}
		scopes[i] = ScoreWeightScope{TenantID: scoring.TenantID, GroupID: candidate.GroupID, UpstreamID: upstreamID}
	}
	resolved := s.Weights.EffectiveWeightsFor(ctx, scopes)
	for i := range out {
		if i < len(resolved) {
			out[i] = resolved[i]
		} else {
			out[i] = DefaultScoreWeights
		}
	}
	return out
}

// ============================================================================
// Helpers
// ============================================================================

func filterEligible(candidates []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	out := make([]*domain.RouteCandidate, 0, len(candidates))
	for _, c := range candidates {
		if !used[c.RouteID] {
			out = append(out, c)
		}
	}
	return out
}

// pickPriorityTier takes the lowest-priority target tier, then uniformly picks
// one candidate within that tier. Fine-grained dynamic preference is handled by
// the multi-dimensional scorer when runtime statistics are available.
func pickPriorityTier(eligible []*domain.RouteCandidate) *domain.RouteCandidate {
	if len(eligible) == 0 {
		return nil
	}
	minP := eligible[0].Priority
	for _, c := range eligible[1:] {
		if c.Priority < minP {
			minP = c.Priority
		}
	}
	var group []*domain.RouteCandidate
	for _, c := range eligible {
		if c.Priority == minP {
			group = append(group, c)
		}
	}
	if len(group) == 1 {
		return group[0]
	}
	return group[rand.Intn(len(group))]
}

// softmaxSampleWithScore draws one candidate and returns its normalised
// softmax probability (0.0 on empty input or zero-sum).
func softmaxSampleWithScore(candidates []*domain.RouteCandidate, scores []float64) (*domain.RouteCandidate, float64) {
	if len(candidates) == 0 {
		return nil, 0
	}
	maxScore := scores[0]
	for _, s := range scores[1:] {
		if s > maxScore {
			maxScore = s
		}
	}
	probs := make([]float64, len(scores))
	sum := 0.0
	for i, s := range scores {
		probs[i] = math.Exp((s - maxScore) / softmaxTempDefault)
		sum += probs[i]
	}
	if sum <= 0 {
		return candidates[rand.Intn(len(candidates))], 0
	}
	r := rand.Float64() * sum
	cum := 0.0
	for i, p := range probs {
		cum += p
		if r < cum {
			return candidates[i], p / sum
		}
	}
	last := len(candidates) - 1
	return candidates[last], probs[last] / sum
}

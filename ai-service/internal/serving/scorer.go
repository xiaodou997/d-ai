package serving

import (
	"context"
	"math"
	"math/rand"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/routing"
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

// RouteScorer picks one candidate from the eligible (non-exhausted) pool.
type RouteScorer interface {
	// Pick returns the next candidate to attempt. Returns nil when every
	// candidate in the used map has been exhausted.
	Pick(ctx context.Context, candidates []*domain.RouteCandidate, used map[string]bool) *domain.RouteCandidate
}

// ScoringPicker is an optional extension of RouteScorer implemented by scorers
// that can report the normalised softmax probability of the chosen candidate.
// ExecuteStep uses this to populate AttemptRecord.Score for X-Route-Trace.
type ScoringPicker interface {
	RouteScorer
	PickWithScore(ctx context.Context, candidates []*domain.RouteCandidate, used map[string]bool) (*domain.RouteCandidate, float64)
}

// ScoreWeightsSource fetches effective weights for a request.
type ScoreWeightsSource interface {
	GlobalWeights(ctx context.Context) ScoreWeights
}

// ============================================================================
// MultiDimScorer
// ============================================================================

// MultiDimScorer implements RouteScorer using cost/latency/load/health scoring.
// When Redis stats are unavailable (all zero), it falls back to the simpler
// priority+weighted random algorithm.
type MultiDimScorer struct {
	Health  routing.HealthTracker      // optional; nil = no health signals
	Stats   routing.RouteStatsStore    // optional; nil = priority+weighted fallback
	Weights ScoreWeightsSource         // optional; nil = DefaultScoreWeights
}

// Pick returns the best candidate using multi-dim scoring with softmax sampling,
// or priority+weighted random when no route stats are available.
func (s *MultiDimScorer) Pick(ctx context.Context, candidates []*domain.RouteCandidate, used map[string]bool) *domain.RouteCandidate {
	c, _ := s.PickWithScore(ctx, candidates, used)
	return c
}

// PickWithScore is like Pick but also returns the normalised softmax probability
// (0.0 when stats-based scoring is not active).
func (s *MultiDimScorer) PickWithScore(ctx context.Context, candidates []*domain.RouteCandidate, used map[string]bool) (*domain.RouteCandidate, float64) {
	eligible := filterEligible(candidates, used)
	if len(eligible) == 0 {
		return nil, 0
	}
	if len(eligible) == 1 {
		return eligible[0], 0
	}

	if s.Stats != nil && s.hasStats(ctx, eligible) {
		return s.pickMultiDimWithScore(ctx, eligible)
	}
	return pickPriorityWeighted(eligible), 0
}

// hasStats returns true if at least one candidate has non-zero Redis stats,
// indicating that real routing signals are available.
func (s *MultiDimScorer) hasStats(ctx context.Context, eligible []*domain.RouteCandidate) bool {
	for _, c := range eligible {
		st := s.Stats.Stats(ctx, c.RouteID)
		if st.EWMALatencyMs > 0 || st.InflightCount > 0 {
			return true
		}
	}
	return false
}

func (s *MultiDimScorer) pickMultiDim(ctx context.Context, eligible []*domain.RouteCandidate) *domain.RouteCandidate {
	c, _ := s.pickMultiDimWithScore(ctx, eligible)
	return c
}

func (s *MultiDimScorer) pickMultiDimWithScore(ctx context.Context, eligible []*domain.RouteCandidate) (*domain.RouteCandidate, float64) {
	weights := s.resolveWeights(ctx)
	wSum := weights.Cost + weights.Latency + weights.Load + weights.Health
	if wSum <= 0 {
		weights = DefaultScoreWeights
		wSum = 1.0
	}

	scores := make([]float64, len(eligible))
	for i, c := range eligible {
		st := s.Stats.Stats(ctx, c.RouteID)
		scores[i] = s.scoreCandidate(c, st, weights, wSum)
	}

	return softmaxSampleWithScore(eligible, scores)
}

const (
	costCapFree        = 1000.0 // effective inverse cost when CostPer1kTokens == 0 (free/pool route)
	minEWMAms          = 1.0    // floor for EWMA to avoid division by zero
	minInflight        = 1.0    // treat 0 inflight as 1 for score purposes
	softmaxTempDefault = 1.0
)

func (s *MultiDimScorer) scoreCandidate(c *domain.RouteCandidate, st routing.RouteStats, w ScoreWeights, wSum float64) float64 {
	// Cost term: 1/cost; free routes get a high cap.
	var costScore float64
	if c.CostPer1kTokens <= 0 {
		costScore = costCapFree
	} else {
		costScore = 1.0 / c.CostPer1kTokens
	}

	// Latency term: 1/ewma (lower latency = higher score).
	ewma := st.EWMALatencyMs
	if ewma < minEWMAms {
		ewma = minEWMAms
	}
	latencyScore := 1.0 / ewma

	// Load term: 1/inflight.
	inflight := float64(st.InflightCount)
	if inflight < minInflight {
		inflight = minInflight
	}
	loadScore := 1.0 / inflight

	// Health term.
	healthFactor := s.healthFactor(c)

	return (w.Cost*costScore + w.Latency*latencyScore + w.Load*loadScore + w.Health*healthFactor) / wSum
}

func (s *MultiDimScorer) healthFactor(c *domain.RouteCandidate) float64 {
	if s.Health == nil {
		return 1.0
	}
	targetID := c.DeploymentID
	if c.IsPoolRoute() {
		targetID = c.PoolID
	}
	switch s.Health.StateOf(targetID) {
	case routing.StateClosed:
		return 1.0
	case routing.StateHalfOpen:
		return 0.3
	default: // StateOpen — should have been filtered already, but guard anyway
		return 0.0
	}
}

func (s *MultiDimScorer) resolveWeights(ctx context.Context) ScoreWeights {
	if s.Weights != nil {
		return s.Weights.GlobalWeights(ctx)
	}
	return DefaultScoreWeights
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

// pickPriorityWeighted mirrors the legacy behaviour: take the lowest-priority
// number group, then pick proportionally by weight.
func pickPriorityWeighted(eligible []*domain.RouteCandidate) *domain.RouteCandidate {
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
	total := 0
	for _, c := range group {
		total += c.Weight
	}
	if total <= 0 {
		return group[rand.Intn(len(group))]
	}
	pick := rand.Intn(total)
	cum := 0
	for _, c := range group {
		cum += c.Weight
		if pick < cum {
			return c
		}
	}
	return group[len(group)-1]
}

// softmaxSample draws one candidate using softmax-weighted probability.
func softmaxSample(candidates []*domain.RouteCandidate, scores []float64) *domain.RouteCandidate {
	c, _ := softmaxSampleWithScore(candidates, scores)
	return c
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

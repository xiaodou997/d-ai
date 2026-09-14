package serving

import (
	"context"
	"math"
	"math/rand"
	"sort"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

type RouteScoringContext struct {
	TenantID string
	Stream   bool
}
type RouteScorer interface {
	Pick(context.Context, RouteScoringContext, []*domain.RouteCandidate, map[string]bool) *domain.RouteCandidate
}
type ScoringPicker interface {
	RouteScorer
	PickWithScore(context.Context, RouteScoringContext, []*domain.RouteCandidate, map[string]bool) (*domain.RouteCandidate, float64)
}

// MultiDimScorer ranks only candidates admitted by availability and structural
// tiers. Scores are policy utilities, not fictitious softmax probabilities.
type MultiDimScorer struct{ Stats routing.RouteStatsStore }

func (s *MultiDimScorer) Pick(ctx context.Context, scoring RouteScoringContext, cs []*domain.RouteCandidate, used map[string]bool) *domain.RouteCandidate {
	c, _ := s.PickWithScore(ctx, scoring, cs, used)
	return c
}
func candidateStatsKey(c *domain.RouteCandidate, stream bool) string {
	if c.CandidateID == "" {
		return c.RouteID
	}
	return c.StatisticsKey(stream)
}
func (s *MultiDimScorer) PickWithScore(ctx context.Context, scoring RouteScoringContext, cs []*domain.RouteCandidate, used map[string]bool) (*domain.RouteCandidate, float64) {
	eligible := filterEligible(cs, used)
	if len(eligible) == 0 {
		return nil, 0
	}
	if len(eligible) == 1 {
		return eligible[0], 0
	}
	stats := map[string]routing.RouteStats{}
	if s.Stats != nil {
		keys := []string{}
		for _, c := range eligible {
			keys = append(keys, candidateStatsKey(c, scoring.Stream), c.CapacityKey())
		}
		stats = s.Stats.Snapshot(ctx, keys)
		for _, c := range eligible {
			key := candidateStatsKey(c, scoring.Stream)
			v := stats[key]
			v.InflightCount = stats[c.CapacityKey()].InflightCount
			stats[key] = v
		}
	}
	scores := s.normalizedCandidateScores(ctx, scoring, eligible, stats)
	if eligible[0].RoutePolicy == "balanced" || eligible[0].RoutePolicy == "" {
		a := rand.Intn(len(eligible))
		b := rand.Intn(len(eligible) - 1)
		if b >= a {
			b++
		}
		if scores[b] > scores[a] {
			a = b
		}
		return eligible[a], scores[a]
	}
	best := 0
	ties := 1
	for i := 1; i < len(scores); i++ {
		if scores[i] > scores[best] {
			best = i
			ties = 1
		} else if scores[i] == scores[best] {
			ties++
			if rand.Intn(ties) == 0 {
				best = i
			}
		}
	}
	return eligible[best], scores[best]
}
func (s *MultiDimScorer) normalizedCandidateScores(_ context.Context, scoring RouteScoringContext, cs []*domain.RouteCandidate, stats map[string]routing.RouteStats) []float64 {
	latencies := make([]float64, len(cs))
	known := []float64{}
	for i, c := range cs {
		st := stats[candidateStatsKey(c, scoring.Stream)]
		latencies[i] = st.EWMATotalMs
		if scoring.Stream {
			latencies[i] = st.EWMAFirstByteMs
		}
		if latencies[i] <= 0 && c.CandidateID == "" {
			latencies[i] = st.EWMALatencyMs
		}
		if latencies[i] > 0 {
			known = append(known, latencies[i])
		}
	}
	neutral := median(known)
	out := make([]float64, len(cs))
	for i, c := range cs {
		if latencies[i] <= 0 {
			latencies[i] = neutral
		}
		st := stats[candidateStatsKey(c, scoring.Stream)]
		switch c.RoutePolicy {
		case "cost":
			cost := float64(c.EstimatedCostMicro)
			if c.CandidateID == "" {
				cost = c.CostPer1kTokens
			}
			out[i] = -cost
		case "latency":
			out[i] = -latencies[i]
		case "stability":
			out[i] = wilsonLowerBound(st.Successes, st.Failures)
		default:
			out[i] = -latencies[i] * (float64(st.InflightCount) + 1)
		}
	}
	return out
}
func wilsonLowerBound(success, failed int64) float64 {
	n := float64(success + failed)
	if n == 0 {
		return 0
	}
	p := float64(success) / n
	z := 1.96
	return (p + z*z/(2*n) - z*math.Sqrt((p*(1-p)+z*z/(4*n))/n)) / (1 + z*z/n)
}
func filterEligible(cs []*domain.RouteCandidate, used map[string]bool) []*domain.RouteCandidate {
	out := []*domain.RouteCandidate{}
	for _, c := range cs {
		if c != nil && !used[c.Key()] {
			out = append(out, c)
		}
	}
	return out
}
func median(values []float64) float64 {
	if len(values) == 0 {
		return 1000
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	m := len(v) / 2
	if len(v)%2 == 1 {
		return v[m]
	}
	return (v[m-1] + v[m]) / 2
}

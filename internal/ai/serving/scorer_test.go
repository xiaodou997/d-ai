package serving

import (
	"context"
	"math"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

// ─── pickPriorityTier ────────────────────────────────────────────────────────

func TestPickPriorityTier_SingleCandidate(t *testing.T) {
	c := &domain.RouteCandidate{RouteID: "r1", Priority: 100}
	got := pickPriorityTier([]*domain.RouteCandidate{c})
	if got != c {
		t.Fatal("single candidate must be returned directly")
	}
}

func TestPickPriorityTier_LowestPriorityGroupWins(t *testing.T) {
	low := &domain.RouteCandidate{RouteID: "low", Priority: 1}
	high := &domain.RouteCandidate{RouteID: "high", Priority: 10}
	for range 100 {
		got := pickPriorityTier([]*domain.RouteCandidate{high, low})
		if got.RouteID != "low" {
			t.Fatal("lower-priority-number candidate should always win")
		}
	}
}

func TestPickPriorityTier_SamePriorityStillReturnsCandidate(t *testing.T) {
	a := &domain.RouteCandidate{RouteID: "a", Priority: 1}
	b := &domain.RouteCandidate{RouteID: "b", Priority: 1}
	got := pickPriorityTier([]*domain.RouteCandidate{a, b})
	if got == nil {
		t.Fatal("should still return a candidate when same-priority tier has multiple candidates")
	}
}

func TestPickPriorityTier_SamePriorityUsesAllCandidates(t *testing.T) {
	a := &domain.RouteCandidate{RouteID: "a", Priority: 1}
	b := &domain.RouteCandidate{RouteID: "b", Priority: 1}
	seen := map[string]bool{}
	for range 128 {
		seen[pickPriorityTier([]*domain.RouteCandidate{a, b}).RouteID] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("same-priority candidates must use uniform selection, seen=%v", seen)
	}
}

// ─── MultiDimScorer fallback branch ─────────────────────────────────────────

type stubStats struct {
	latency      float64
	snapshotCall int
}

func (s *stubStats) Snapshot(_ context.Context, routeIDs []string) map[string]routing.RouteStats {
	s.snapshotCall++
	out := make(map[string]routing.RouteStats, len(routeIDs))
	for _, routeID := range routeIDs {
		out[routeID] = routing.RouteStats{EWMALatencyMs: s.latency}
	}
	return out
}
func (s *stubStats) IncrInflight(_ context.Context, _ string)         {}
func (s *stubStats) DecrInflight(_ context.Context, _ string)         {}
func (s *stubStats) RecordLatency(_ context.Context, _ string, _ int) {}

func TestMultiDimScorer_NoStatsFallsThroughToPriorityTier(t *testing.T) {
	scorer := &MultiDimScorer{Stats: &stubStats{latency: 0}} // zero latency → no stats
	candidates := []*domain.RouteCandidate{
		{RouteID: "a", Priority: 1},
		{RouteID: "b", Priority: 5},
	}
	used := map[string]bool{}
	got := scorer.Pick(context.Background(), RouteScoringContext{}, candidates, used)
	if got == nil {
		t.Fatal("expected a candidate")
	}
	if got.RouteID != "a" {
		t.Errorf("priority fallback should pick RouteID=a (priority 1), got %q", got.RouteID)
	}
}

func TestMultiDimScorer_MultiDimPathActivatedByNonZeroStats(t *testing.T) {
	stats := &stubStats{latency: 100}
	scorer := &MultiDimScorer{Stats: stats} // non-zero → multi-dim active
	candidates := []*domain.RouteCandidate{
		{RouteID: "a", Priority: 1, CostPer1kTokens: 1.0},
		{RouteID: "b", Priority: 1, CostPer1kTokens: 0.1},
	}
	used := map[string]bool{}
	// just ensure it picks without panic
	got := scorer.Pick(context.Background(), RouteScoringContext{}, candidates, used)
	if got == nil {
		t.Fatal("expected a candidate from multi-dim scorer")
	}
	if stats.snapshotCall != 1 {
		t.Fatalf("stats snapshot calls = %d, want one batch read", stats.snapshotCall)
	}
}

func TestMultiDimScorerAdaptiveUsesObjective(t *testing.T) {
	scorer := &MultiDimScorer{}
	candidates := []*domain.RouteCandidate{
		{RouteID: "cheap", CostPer1kTokens: 0.01, RouteStrategy: "adaptive", RouteObjective: "cost"},
		{RouteID: "fast", CostPer1kTokens: 1, RouteStrategy: "adaptive", RouteObjective: "cost"},
	}
	stats := map[string]routing.RouteStats{
		"cheap": {EWMALatencyMs: 200},
		"fast":  {EWMALatencyMs: 20},
	}
	scores := scorer.normalizedCandidateScores(context.Background(), RouteScoringContext{}, candidates, stats)
	if len(scores) != 2 || scores[0] <= scores[1] {
		t.Fatalf("normalized scores = %v, cost objective should prefer the cheap route", scores)
	}
}

func TestMultiDimScorerUsesNeutralLatencyForColdRoute(t *testing.T) {
	scorer := &MultiDimScorer{}
	candidates := []*domain.RouteCandidate{{RouteID: "known", RouteStrategy: "adaptive"}, {RouteID: "cold", RouteStrategy: "adaptive"}}
	stats := map[string]routing.RouteStats{"known": {EWMALatencyMs: 100}}
	scores := scorer.normalizedCandidateScores(context.Background(), RouteScoringContext{}, candidates, stats)
	if scores[0] != scores[1] {
		t.Fatalf("scores = %v, a cold route must not be treated as a 1ms route", scores)
	}
}

func TestMultiDimScorerWeightedStrategyUsesTargetWeights(t *testing.T) {
	scorer := &MultiDimScorer{}
	candidates := []*domain.RouteCandidate{{RouteID: "zero", RouteStrategy: "weighted", RoutingWeight: 0}, {RouteID: "one", RouteStrategy: "weighted", RoutingWeight: 1}}
	for range 20 {
		if got, _ := scorer.PickWithScore(context.Background(), RouteScoringContext{}, candidates, map[string]bool{}); got.RouteID != "one" {
			t.Fatalf("weighted strategy selected %q", got.RouteID)
		}
	}
}

func TestMultiDimScorerWeightedStrategyTreatsNonFiniteWeightsAsZero(t *testing.T) {
	scorer := &MultiDimScorer{}
	candidates := []*domain.RouteCandidate{
		{RouteID: "bad", RouteStrategy: "weighted", RoutingWeight: math.NaN()},
		{RouteID: "good", RouteStrategy: "weighted", RoutingWeight: 1},
	}
	for range 20 {
		got, _ := scorer.PickWithScore(context.Background(), RouteScoringContext{}, candidates, map[string]bool{})
		if got == nil || got.RouteID != "good" {
			t.Fatalf("weighted strategy selected %#v", got)
		}
	}
}

func TestMultiDimScorerKeepsPriorityAsFailoverBoundary(t *testing.T) {
	scorer := &MultiDimScorer{}
	candidates := []*domain.RouteCandidate{
		{RouteID: "primary", Priority: 10, RouteStrategy: "weighted", RoutingWeight: 0},
		{RouteID: "backup", Priority: 20, RouteStrategy: "weighted", RoutingWeight: 100},
	}
	for range 20 {
		got, _ := scorer.PickWithScore(context.Background(), RouteScoringContext{}, candidates, map[string]bool{})
		if got == nil || got.RouteID != "primary" {
			t.Fatalf("weighted strategy crossed priority boundary and selected %#v", got)
		}
	}
}

// ─── findStickyCandidate (sticky hit / miss) ─────────────────────────────────

func TestFindStickyCandidate_DeploymentHit(t *testing.T) {
	candidates := []*domain.RouteCandidate{
		{RouteID: "r1", EndpointID: "d1"},
		{RouteID: "r2", EndpointID: "d2"},
	}
	b := &routing.StickyBinding{TargetKind: "account", RouteID: "r2", EndpointID: "d2"}
	idx := findStickyCandidate(candidates, b)
	if idx != 1 {
		t.Errorf("expected index 1, got %d", idx)
	}
}

func TestFindStickyCandidate_Miss(t *testing.T) {
	candidates := []*domain.RouteCandidate{
		{RouteID: "r1", EndpointID: "d1"},
	}
	b := &routing.StickyBinding{TargetKind: "account", RouteID: "r99", EndpointID: "d99"}
	idx := findStickyCandidate(candidates, b)
	if idx != -1 {
		t.Errorf("expected -1 for miss, got %d", idx)
	}
}

func TestFindStickyCandidate_PoolHit(t *testing.T) {
	candidates := []*domain.RouteCandidate{
		{RouteID: "rp", PoolID: "pool1"},
	}
	b := &routing.StickyBinding{TargetKind: "credential", RouteID: "rp", CredentialID: "cred1"}
	idx := findStickyCandidate(candidates, b)
	if idx != 0 {
		t.Errorf("expected index 0, got %d", idx)
	}
}

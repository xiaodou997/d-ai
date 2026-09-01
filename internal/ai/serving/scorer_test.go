package serving

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

// ─── MultiDimScorer automatic policy ─────────────────────────────────────────

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

func TestMultiDimScorer_NoStatsStillSelectsAnAutomaticCandidate(t *testing.T) {
	scorer := &MultiDimScorer{Stats: &stubStats{latency: 0}}
	candidates := []*domain.RouteCandidate{
		{RouteID: "a", RoutePolicy: "balanced"},
		{RouteID: "b", RoutePolicy: "balanced"},
	}
	used := map[string]bool{}
	got := scorer.Pick(context.Background(), RouteScoringContext{}, candidates, used)
	if got == nil {
		t.Fatal("expected a candidate")
	}
	if got.RouteID != "a" && got.RouteID != "b" {
		t.Errorf("automatic policy picked unexpected route %q", got.RouteID)
	}
}

func TestMultiDimScorer_MultiDimPathActivatedByNonZeroStats(t *testing.T) {
	stats := &stubStats{latency: 100}
	scorer := &MultiDimScorer{Stats: stats} // non-zero → multi-dim active
	candidates := []*domain.RouteCandidate{
		{RouteID: "a", RoutePolicy: "cost", CostPer1kTokens: 1.0},
		{RouteID: "b", RoutePolicy: "cost", CostPer1kTokens: 0.1},
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

func TestMultiDimScorerUsesRoutePolicy(t *testing.T) {
	scorer := &MultiDimScorer{}
	candidates := []*domain.RouteCandidate{
		{RouteID: "cheap", CostPer1kTokens: 0.01, RoutePolicy: "cost"},
		{RouteID: "fast", CostPer1kTokens: 1, RoutePolicy: "cost"},
	}
	stats := map[string]routing.RouteStats{
		"cheap": {EWMALatencyMs: 200},
		"fast":  {EWMALatencyMs: 20},
	}
	scores := scorer.normalizedCandidateScores(context.Background(), RouteScoringContext{}, candidates, stats)
	if len(scores) != 2 || scores[0] <= scores[1] {
		t.Fatalf("normalized scores = %v, cost policy should prefer the cheap route", scores)
	}
}

func TestMultiDimScorerUsesNeutralLatencyForColdRoute(t *testing.T) {
	scorer := &MultiDimScorer{}
	candidates := []*domain.RouteCandidate{{RouteID: "known", RoutePolicy: "balanced"}, {RouteID: "cold", RoutePolicy: "balanced"}}
	stats := map[string]routing.RouteStats{"known": {EWMALatencyMs: 100}}
	scores := scorer.normalizedCandidateScores(context.Background(), RouteScoringContext{}, candidates, stats)
	if scores[0] != scores[1] {
		t.Fatalf("scores = %v, a cold route must not be treated as a 1ms route", scores)
	}
}

func TestMultiDimScorerDoesNotRequireTargetWeights(t *testing.T) {
	scorer := &MultiDimScorer{}
	candidates := []*domain.RouteCandidate{{RouteID: "a", RoutePolicy: "balanced"}, {RouteID: "b", RoutePolicy: "balanced"}}
	seen := map[string]bool{}
	for range 128 {
		got, _ := scorer.PickWithScore(context.Background(), RouteScoringContext{}, candidates, map[string]bool{})
		seen[got.RouteID] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("automatic policy should consider all active targets, seen=%v", seen)
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

package serving

import (
	"context"
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

func TestMultiDimScorerNormalizesDimensionsBeforeApplyingWeights(t *testing.T) {
	scorer := &MultiDimScorer{Weights: fixedWeightsSource{weights: ScoreWeights{Cost: 0.1, Latency: 0.9}}}
	candidates := []*domain.RouteCandidate{
		{RouteID: "cheap", CostPer1kTokens: 0.01},
		{RouteID: "fast", CostPer1kTokens: 1},
	}
	stats := map[string]routing.RouteStats{
		"cheap": {EWMALatencyMs: 200},
		"fast":  {EWMALatencyMs: 20},
	}
	scores := scorer.normalizedCandidateScores(context.Background(), RouteScoringContext{}, candidates, stats)
	if len(scores) != 2 || scores[1] <= scores[0] {
		t.Fatalf("normalized scores = %v, latency weight should make the fast route win", scores)
	}
}

func TestMultiDimScorerUsesNeutralLatencyForColdRoute(t *testing.T) {
	scorer := &MultiDimScorer{Weights: fixedWeightsSource{weights: ScoreWeights{Latency: 1}}}
	candidates := []*domain.RouteCandidate{{RouteID: "known"}, {RouteID: "cold"}}
	stats := map[string]routing.RouteStats{"known": {EWMALatencyMs: 100}}
	scores := scorer.normalizedCandidateScores(context.Background(), RouteScoringContext{}, candidates, stats)
	if scores[0] != scores[1] {
		t.Fatalf("scores = %v, a cold route must not be treated as a 1ms route", scores)
	}
}

type fixedWeightsSource struct{ weights ScoreWeights }

func (s fixedWeightsSource) EffectiveWeightsFor(_ context.Context, scopes []ScoreWeightScope) []ScoreWeights {
	out := make([]ScoreWeights, len(scopes))
	for i := range out {
		out[i] = s.weights
	}
	return out
}

type recordingWeightsSource struct {
	tenantID   string
	groupID    string
	upstreamID string
	calls      int
}

func (s *recordingWeightsSource) EffectiveWeightsFor(_ context.Context, scopes []ScoreWeightScope) []ScoreWeights {
	s.calls++
	out := make([]ScoreWeights, len(scopes))
	for index, scope := range scopes {
		s.tenantID, s.groupID, s.upstreamID = scope.TenantID, scope.GroupID, scope.UpstreamID
		out[index] = ScoreWeights{Health: 1}
	}
	return out
}

func TestMultiDimScorerLoadsCandidateWeightsInOneBatch(t *testing.T) {
	weights := &recordingWeightsSource{}
	scorer := &MultiDimScorer{Weights: weights}
	candidates := []*domain.RouteCandidate{
		{GroupID: "group-1", EndpointID: "account-1"},
		{GroupID: "group-2", PoolID: "pool-2"},
	}
	resolved := scorer.resolveWeightsFor(context.Background(), RouteScoringContext{TenantID: "tenant-1"}, candidates)
	if len(resolved) != 2 || weights.calls != 1 {
		t.Fatalf("resolved weights = %d, batch calls = %d; want 2 results from one call", len(resolved), weights.calls)
	}
}

func TestMultiDimScorerResolvesCandidateScopedWeights(t *testing.T) {
	weights := &recordingWeightsSource{}
	scorer := &MultiDimScorer{Weights: weights}
	candidate := &domain.RouteCandidate{GroupID: "group-1", PoolID: "pool-1"}
	got := scorer.resolveWeights(context.Background(), RouteScoringContext{TenantID: "tenant-1"}, candidate)
	if got.Health != 1 {
		t.Fatalf("weights = %+v", got)
	}
	if weights.tenantID != "tenant-1" || weights.groupID != "group-1" || weights.upstreamID != "pool-1" {
		t.Fatalf("weight scope = tenant:%q group:%q upstream:%q", weights.tenantID, weights.groupID, weights.upstreamID)
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

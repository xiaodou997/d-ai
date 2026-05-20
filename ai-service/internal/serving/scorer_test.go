package serving

import (
	"context"
	"testing"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/routing"
)

// ─── pickPriorityWeighted ────────────────────────────────────────────────────

func TestPickPriorityWeighted_SingleCandidate(t *testing.T) {
	c := &domain.RouteCandidate{RouteID: "r1", Priority: 100, Weight: 100}
	got := pickPriorityWeighted([]*domain.RouteCandidate{c})
	if got != c {
		t.Fatal("single candidate must be returned directly")
	}
}

func TestPickPriorityWeighted_LowestPriorityGroupWins(t *testing.T) {
	low := &domain.RouteCandidate{RouteID: "low", Priority: 1, Weight: 100}
	high := &domain.RouteCandidate{RouteID: "high", Priority: 10, Weight: 100}
	for range 100 {
		got := pickPriorityWeighted([]*domain.RouteCandidate{high, low})
		if got.RouteID != "low" {
			t.Fatal("lower-priority-number candidate should always win")
		}
	}
}

func TestPickPriorityWeighted_ZeroWeightFallback(t *testing.T) {
	a := &domain.RouteCandidate{RouteID: "a", Priority: 1, Weight: 0}
	b := &domain.RouteCandidate{RouteID: "b", Priority: 1, Weight: 0}
	got := pickPriorityWeighted([]*domain.RouteCandidate{a, b})
	if got == nil {
		t.Fatal("should still return a candidate when all weights are 0")
	}
}

// ─── MultiDimScorer fallback branch ─────────────────────────────────────────

type stubStats struct{ latency float64 }

func (s *stubStats) Stats(_ context.Context, _ string) routing.RouteStats {
	return routing.RouteStats{EWMALatencyMs: s.latency}
}
func (s *stubStats) IncrInflight(_ context.Context, _ string)               {}
func (s *stubStats) DecrInflight(_ context.Context, _ string)               {}
func (s *stubStats) RecordLatency(_ context.Context, _ string, _ int)       {}

func TestMultiDimScorer_NoStatsFallsThroughToPriorityWeighted(t *testing.T) {
	scorer := &MultiDimScorer{Stats: &stubStats{latency: 0}} // zero latency → no stats
	candidates := []*domain.RouteCandidate{
		{RouteID: "a", Priority: 1, Weight: 100},
		{RouteID: "b", Priority: 5, Weight: 100},
	}
	used := map[string]bool{}
	got := scorer.Pick(context.Background(), candidates, used)
	if got == nil {
		t.Fatal("expected a candidate")
	}
	if got.RouteID != "a" {
		t.Errorf("priority fallback should pick RouteID=a (priority 1), got %q", got.RouteID)
	}
}

func TestMultiDimScorer_MultiDimPathActivatedByNonZeroStats(t *testing.T) {
	scorer := &MultiDimScorer{Stats: &stubStats{latency: 100}} // non-zero → multi-dim active
	candidates := []*domain.RouteCandidate{
		{RouteID: "a", Priority: 1, Weight: 100, CostPer1kTokens: 1.0},
		{RouteID: "b", Priority: 1, Weight: 100, CostPer1kTokens: 0.1},
	}
	used := map[string]bool{}
	// just ensure it picks without panic
	got := scorer.Pick(context.Background(), candidates, used)
	if got == nil {
		t.Fatal("expected a candidate from multi-dim scorer")
	}
}

// ─── findStickyCandidate (sticky hit / miss) ─────────────────────────────────

func TestFindStickyCandidate_DeploymentHit(t *testing.T) {
	candidates := []*domain.RouteCandidate{
		{RouteID: "r1", DeploymentID: "d1"},
		{RouteID: "r2", DeploymentID: "d2"},
	}
	b := &routing.StickyBinding{TargetKind: "deployment", RouteID: "r2", DeploymentID: "d2"}
	idx := findStickyCandidate(candidates, b)
	if idx != 1 {
		t.Errorf("expected index 1, got %d", idx)
	}
}

func TestFindStickyCandidate_Miss(t *testing.T) {
	candidates := []*domain.RouteCandidate{
		{RouteID: "r1", DeploymentID: "d1"},
	}
	b := &routing.StickyBinding{TargetKind: "deployment", RouteID: "r99", DeploymentID: "d99"}
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

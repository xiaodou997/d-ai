package postgres

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/serving"
)

func TestRouteWeightsEffectiveScopePrecedence(t *testing.T) {
	now := time.Now()
	store := &RouteWeightsStore{cache: map[string]cachedWeights{
		globalScope:          {weights: serving.ScoreWeights{Cost: 1}, fetchAt: now, found: true},
		"tenant:tenant-1":    {weights: serving.ScoreWeights{Latency: 1}, fetchAt: now, found: true},
		"group:group-1":      {weights: serving.ScoreWeights{Load: 1}, fetchAt: now, found: true},
		"upstream:account-1": {weights: serving.ScoreWeights{Health: 1}, fetchAt: now, found: true},
		"tenant:missing":     {fetchAt: now, found: false},
		"group:missing":      {fetchAt: now, found: false},
		"upstream:missing":   {fetchAt: now, found: false},
	}}

	if got := store.EffectiveWeights(context.Background(), "tenant-1", "group-1", "account-1"); got.Health != 1 {
		t.Fatalf("upstream weights = %+v", got)
	}
	if got := store.EffectiveWeights(context.Background(), "tenant-1", "group-1", "missing"); got.Load != 1 {
		t.Fatalf("group weights = %+v", got)
	}
	if got := store.EffectiveWeights(context.Background(), "tenant-1", "missing", "missing"); got.Latency != 1 {
		t.Fatalf("tenant weights = %+v", got)
	}
	if got := store.EffectiveWeights(context.Background(), "missing", "missing", "missing"); got.Cost != 1 {
		t.Fatalf("global weights = %+v", got)
	}
}

func TestRouteWeightsEffectiveWeightsForLoadsColdScopesInBatch(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_route_score_weights (scope, weights) VALUES
		  ('global', '{"cost":1,"latency":0,"load":0,"health":0}'::jsonb),
		  ('group:group-1', '{"cost":0,"latency":1,"load":0,"health":0}'::jsonb),
		  ('upstream:account-1', '{"cost":0,"latency":0,"load":0,"health":1}'::jsonb)
	`); err != nil {
		t.Fatalf("seed route weights: %v", err)
	}
	store := NewRouteWeightsStore(pool)

	got := store.EffectiveWeightsFor(ctx, []serving.ScoreWeightScope{
		{TenantID: "tenant-1", GroupID: "group-1", UpstreamID: "account-1"},
		{TenantID: "tenant-1", GroupID: "group-1", UpstreamID: "account-2"},
		{TenantID: "tenant-1", GroupID: "group-2", UpstreamID: "account-2"},
	})
	if len(got) != 3 || got[0].Health != 1 || got[1].Latency != 1 || got[2].Cost != 1 {
		t.Fatalf("batch scope precedence = %#v", got)
	}
}

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/unihub/ai-service/internal/serving"
)

const (
	weightsLocalCacheTTL = 5 * time.Minute
	globalScope          = "global"
)

type cachedWeights struct {
	weights serving.ScoreWeights
	fetchAt time.Time
}

// RouteWeightsStore loads and persists ai_route_score_weights rows.
// It satisfies serving.ScoreWeightsSource via GlobalWeights().
type RouteWeightsStore struct {
	pool *pgxpool.Pool

	mu    sync.RWMutex
	cache map[string]cachedWeights
}

// NewRouteWeightsStore creates a store backed by pool.
func NewRouteWeightsStore(pool *pgxpool.Pool) *RouteWeightsStore {
	return &RouteWeightsStore{
		pool:  pool,
		cache: make(map[string]cachedWeights),
	}
}

// GlobalWeights returns the effective scorer weights for scope "global".
// Results are cached for 5 minutes to avoid hot-path DB reads. Falls back to
// serving.DefaultScoreWeights on any error.
func (s *RouteWeightsStore) GlobalWeights(ctx context.Context) serving.ScoreWeights {
	return s.Get(ctx, globalScope)
}

// Get returns the scorer weights for the given scope. Falls back to
// serving.DefaultScoreWeights on cache miss + DB error.
func (s *RouteWeightsStore) Get(ctx context.Context, scope string) serving.ScoreWeights {
	s.mu.RLock()
	if c, ok := s.cache[scope]; ok && time.Since(c.fetchAt) < weightsLocalCacheTTL {
		s.mu.RUnlock()
		return c.weights
	}
	s.mu.RUnlock()

	weights, err := s.loadFromDB(ctx, scope)
	if err != nil {
		return serving.DefaultScoreWeights
	}

	s.mu.Lock()
	s.cache[scope] = cachedWeights{weights: weights, fetchAt: time.Now()}
	s.mu.Unlock()
	return weights
}

// Upsert writes (or updates) the weights for scope and invalidates the cache.
func (s *RouteWeightsStore) Upsert(ctx context.Context, scope string, w serving.ScoreWeights) error {
	raw, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("marshal weights: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO ai_route_score_weights (scope, weights, updated_at)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (scope) DO UPDATE
		  SET weights    = excluded.weights,
		      updated_at = excluded.updated_at
	`, scope, string(raw))
	if err != nil {
		return fmt.Errorf("upsert route weights: %w", err)
	}
	s.mu.Lock()
	delete(s.cache, scope)
	s.mu.Unlock()
	return nil
}

func (s *RouteWeightsStore) loadFromDB(ctx context.Context, scope string) (serving.ScoreWeights, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT weights FROM ai_route_score_weights WHERE scope = $1`, scope)
	var raw string
	if err := row.Scan(&raw); err != nil {
		return serving.DefaultScoreWeights, fmt.Errorf("query weights for scope %q: %w", scope, err)
	}
	var w serving.ScoreWeights
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return serving.DefaultScoreWeights, fmt.Errorf("unmarshal weights: %w", err)
	}
	return w, nil
}

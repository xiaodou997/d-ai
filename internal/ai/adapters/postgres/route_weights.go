package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/serving"
)

const (
	weightsLocalCacheTTL = 5 * time.Minute
	globalScope          = "global"
)

type cachedWeights struct {
	weights serving.ScoreWeights
	fetchAt time.Time
	found   bool
}

// RouteWeightsStore loads and persists ai_route_score_weights rows.
// It satisfies serving.ScoreWeightsSource via EffectiveWeightsFor().
type RouteWeightsStore struct {
	pool *translatingPool

	mu    sync.RWMutex
	cache map[string]cachedWeights
}

// NewRouteWeightsStore creates a store backed by pool.
func NewRouteWeightsStore(pool *pgxpool.Pool) *RouteWeightsStore {
	return &RouteWeightsStore{
		pool:  newTranslatingPool(pool),
		cache: make(map[string]cachedWeights),
	}
}

// GlobalWeights returns the effective scorer weights for scope "global".
// Results are cached for 5 minutes to avoid hot-path DB reads. Falls back to
// serving.DefaultScoreWeights on any error.
func (s *RouteWeightsStore) GlobalWeights(ctx context.Context) serving.ScoreWeights {
	return s.Get(ctx, globalScope)
}

// EffectiveWeights resolves one scope chain. Runtime scoring uses the batch
// variant below so a cold cache never multiplies database round trips.
func (s *RouteWeightsStore) EffectiveWeights(ctx context.Context, tenantID, groupID, upstreamID string) serving.ScoreWeights {
	resolved := s.EffectiveWeightsFor(ctx, []serving.ScoreWeightScope{{
		TenantID: tenantID, GroupID: groupID, UpstreamID: upstreamID,
	}})
	if len(resolved) == 0 {
		return serving.DefaultScoreWeights
	}
	return resolved[0]
}

// EffectiveWeightsFor resolves all candidate scope chains with one query for
// every cache miss in the set. Results preserve input order.
func (s *RouteWeightsStore) EffectiveWeightsFor(ctx context.Context, requests []serving.ScoreWeightScope) []serving.ScoreWeights {
	results := make([]serving.ScoreWeights, len(requests))
	if len(requests) == 0 {
		return results
	}
	chains := make([][]string, len(requests))
	uniqueScopes := make(map[string]struct{}, len(requests)*4)
	for index, request := range requests {
		chains[index] = weightScopeChain(request.TenantID, request.GroupID, request.UpstreamID)
		for _, scope := range chains[index] {
			uniqueScopes[scope] = struct{}{}
		}
	}

	now := time.Now()
	available := make(map[string]cachedWeights, len(uniqueScopes))
	missing := make([]string, 0, len(uniqueScopes))
	s.mu.RLock()
	for scope := range uniqueScopes {
		if cached, ok := s.cache[scope]; ok && now.Sub(cached.fetchAt) < weightsLocalCacheTTL {
			available[scope] = cached
		} else {
			missing = append(missing, scope)
		}
	}
	s.mu.RUnlock()

	if len(missing) > 0 && s.pool != nil {
		loaded, err := s.loadManyFromDB(ctx, missing)
		if err == nil {
			s.mu.Lock()
			for _, scope := range missing {
				weights, found := loaded[scope]
				cached := cachedWeights{weights: weights, fetchAt: time.Now(), found: found}
				s.cache[scope] = cached
				available[scope] = cached
			}
			s.mu.Unlock()
		}
	}

	for index, chain := range chains {
		results[index] = serving.DefaultScoreWeights
		for _, scope := range chain {
			if cached, ok := available[scope]; ok && cached.found {
				results[index] = cached.weights
				break
			}
		}
	}
	return results
}

func weightScopeChain(tenantID, groupID, upstreamID string) []string {
	scopes := make([]string, 0, 4)
	if upstreamID != "" {
		scopes = append(scopes, "upstream:"+upstreamID)
	}
	if groupID != "" {
		scopes = append(scopes, "group:"+groupID)
	}
	if tenantID != "" {
		scopes = append(scopes, "tenant:"+tenantID)
	}
	return append(scopes, globalScope)
}

// Get returns the scorer weights for the given scope. Falls back to
// serving.DefaultScoreWeights on cache miss + DB error.
func (s *RouteWeightsStore) Get(ctx context.Context, scope string) serving.ScoreWeights {
	if weights, found := s.getOptional(ctx, scope); found {
		return weights
	}
	return serving.DefaultScoreWeights
}

func (s *RouteWeightsStore) getOptional(ctx context.Context, scope string) (serving.ScoreWeights, bool) {
	s.mu.RLock()
	if c, ok := s.cache[scope]; ok && time.Since(c.fetchAt) < weightsLocalCacheTTL {
		s.mu.RUnlock()
		return c.weights, c.found
	}
	s.mu.RUnlock()

	weights, err := s.loadFromDB(ctx, scope)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return serving.ScoreWeights{}, false
		}
		s.mu.Lock()
		s.cache[scope] = cachedWeights{fetchAt: time.Now(), found: false}
		s.mu.Unlock()
		return serving.ScoreWeights{}, false
	}

	s.mu.Lock()
	s.cache[scope] = cachedWeights{weights: weights, fetchAt: time.Now(), found: true}
	s.mu.Unlock()
	return weights, true
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

func (s *RouteWeightsStore) loadManyFromDB(ctx context.Context, scopes []string) (map[string]serving.ScoreWeights, error) {
	rows, err := s.pool.Query(ctx, `SELECT scope, weights FROM ai_route_score_weights WHERE scope = ANY($1::text[])`, scopes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]serving.ScoreWeights, len(scopes))
	for rows.Next() {
		var scope, raw string
		if err := rows.Scan(&scope, &raw); err != nil {
			return nil, err
		}
		var weights serving.ScoreWeights
		if err := json.Unmarshal([]byte(raw), &weights); err != nil {
			return nil, fmt.Errorf("unmarshal weights for scope %q: %w", scope, err)
		}
		out[scope] = weights
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

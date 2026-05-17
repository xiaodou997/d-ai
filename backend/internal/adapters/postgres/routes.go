package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/routing"
	"uni-ai-api/backend/internal/secret"
	"uni-ai-api/backend/internal/serving"
)

const (
	conversationBindingTTL = 24 * time.Hour
	conversationKeyPrefix  = "uni_ai_api:conv:"
)

// routeRow holds one resolved route from the dual-source UNION query.
// All pointer fields are NULL for the route type that doesn't apply.
type routeRow struct {
	RouteID        string
	RoutePriority  int32
	RouteWeight    int32
	SupportsStream bool

	// Deployment-based fields (nil for pool routes)
	DeploymentID       *string
	UpstreamModel      *string
	CapabilityType     *string
	UpstreamProtocol   *string
	RequestPath        *string
	UpstreamParameters []byte
	HealthStatus       *string
	EndpointID         *string
	BaseURL            *string
	APIKeyCiphertext   *string
	ExtraHeaders       []byte
	TimeoutMs          int32
	EndpointWeight     int32
	ProviderCode       *string

	// Pool-based fields (nil for deployment routes)
	PoolID            *string
	PoolUpstreamModel *string
	FixedProviderType *string
	OAuthStrategy     *string
}

// listRoutesForModel fetches all active routes for a model, supporting both
// deployment-based (API Key) and pool-based (OAuth Fixed Provider) routes.
func (s *RouteSelector) listRoutesForModel(ctx context.Context, modelID pgtype.UUID) ([]routeRow, error) {
	const q = `
		-- Deployment-based routes (API Key type)
		SELECT
		  r.id::text       AS route_id,
		  r.priority       AS route_priority,
		  r.weight         AS route_weight,
		  r.supports_stream,
		  ud.id::text      AS deployment_id,
		  ud.upstream_model,
		  ud.capability_type,
		  ud.upstream_protocol,
		  ud.request_path,
		  ud.upstream_parameters,
		  ud.health_status,
		  e.id::text       AS endpoint_id,
		  e.base_url,
		  e.api_key_ciphertext,
		  e.extra_headers,
		  e.timeout_ms,
		  e.weight         AS endpoint_weight,
		  p.code           AS provider_code,
		  NULL::text       AS pool_id,
		  NULL::text       AS pool_upstream_model,
		  NULL::text       AS fixed_provider_type,
		  NULL::text       AS oauth_strategy
		FROM ai_model_routes r
		JOIN ai_upstream_deployments ud ON ud.id = r.upstream_deployment_id
		JOIN ai_provider_endpoints   e  ON e.id  = ud.endpoint_id
		JOIN ai_providers            p  ON p.id  = e.provider_id
		WHERE r.model_id               = $1
		  AND r.status                 = 'active'
		  AND r.upstream_deployment_id IS NOT NULL
		  AND ud.status                = 'active'
		  AND ud.health_status         IN ('healthy', 'unknown')
		  AND e.status                 = 'active'
		  AND p.status                 = 'active'

		UNION ALL

		-- Pool-based routes (OAuth Fixed Provider)
		SELECT
		  r.id::text       AS route_id,
		  r.priority       AS route_priority,
		  r.weight         AS route_weight,
		  r.supports_stream,
		  NULL::text       AS deployment_id,
		  r.pool_upstream_model AS upstream_model,
		  NULL::text       AS capability_type,
		  NULL::text       AS upstream_protocol,
		  NULL::text       AS request_path,
		  NULL::jsonb      AS upstream_parameters,
		  NULL::text       AS health_status,
		  NULL::text       AS endpoint_id,
		  NULL::text       AS base_url,
		  NULL::text       AS api_key_ciphertext,
		  NULL::jsonb      AS extra_headers,
		  30000::int       AS timeout_ms,
		  100::int         AS endpoint_weight,
		  cp.fixed_provider_type AS provider_code,
		  cp.id::text      AS pool_id,
		  r.pool_upstream_model,
		  cp.fixed_provider_type,
		  cp.oauth_strategy
		FROM ai_model_routes r
		JOIN ai_credential_pools cp ON cp.id = r.credential_pool_id
		WHERE r.model_id             = $1
		  AND r.status               = 'active'
		  AND r.credential_pool_id   IS NOT NULL
		  AND cp.status              = 'active'

		ORDER BY route_priority ASC, route_weight DESC, endpoint_weight DESC`

	pgRows, err := s.pool.Query(ctx, q, modelID)
	if err != nil {
		return nil, fmt.Errorf("list routes for model: %w", err)
	}
	defer pgRows.Close()

	var out []routeRow
	for pgRows.Next() {
		var r routeRow
		if err := pgRows.Scan(
			&r.RouteID, &r.RoutePriority, &r.RouteWeight, &r.SupportsStream,
			&r.DeploymentID, &r.UpstreamModel, &r.CapabilityType, &r.UpstreamProtocol,
			&r.RequestPath, &r.UpstreamParameters, &r.HealthStatus,
			&r.EndpointID, &r.BaseURL, &r.APIKeyCiphertext, &r.ExtraHeaders,
			&r.TimeoutMs, &r.EndpointWeight, &r.ProviderCode,
			&r.PoolID, &r.PoolUpstreamModel, &r.FixedProviderType, &r.OAuthStrategy,
		); err != nil {
			return nil, fmt.Errorf("scan route row: %w", err)
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}

// conversationBinding stores sticky routing state in Redis.
type conversationBinding struct {
	DeploymentID string `json:"deployment_id"`
	EndpointID   string `json:"endpoint_id"`
}

// RouteSelector loads all healthy routes for the requested model and selects
// one according to priority → weighted random selection.
// It satisfies serving.RouteSelector.
type RouteSelector struct {
	q            *dbgen.Queries
	pool         *pgxpool.Pool
	masterKey    string
	grantChecker *ModelGrantChecker
	breaker      *routing.CircuitBreaker
	redis        *redis.Client
	oauthCreds   *OAuthCredentialStore
}

func NewRouteSelector(q *dbgen.Queries, pool *pgxpool.Pool, masterKey string, gc *ModelGrantChecker) *RouteSelector {
	return &RouteSelector{
		q:            q,
		pool:         pool,
		masterKey:    masterKey,
		grantChecker: gc,
		breaker:      routing.NewCircuitBreaker(routing.DefaultBreakerConfig()),
	}
}

func (s *RouteSelector) WithOAuthCredentialStore(store *OAuthCredentialStore) *RouteSelector {
	s.oauthCreds = store
	return s
}

func (s *RouteSelector) WithRedis(rdb *redis.Client) *RouteSelector {
	s.redis = rdb
	return s
}

// SelectRoute resolves model → routes and picks the best candidate.
func (s *RouteSelector) SelectRoute(ctx context.Context, req *serving.Request) error {
	model, err := s.grantChecker.resolveModelID(ctx,
		req.APIKey.TenantID, req.ModelCode, string(req.CapabilityType))
	if err != nil {
		return fmt.Errorf("resolve model id: %w", err)
	}

	rows, err := s.listRoutesForModel(ctx, mustParseUUID(model.ID))
	if err != nil {
		return fmt.Errorf("list routes: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("no healthy routes for model %q", req.ModelCode)
	}

	// Filter circuit-broken deployment routes (pool routes skip CB).
	rows = s.filterOpenCircuits(rows)
	if len(rows) == 0 {
		return fmt.Errorf("all routes for model %q are circuit-broken", req.ModelCode)
	}

	if req.IsStream {
		rows = filterStreamable(rows)
		if len(rows) == 0 {
			return fmt.Errorf("no streaming-capable routes for model %q", req.ModelCode)
		}
	}

	// Sticky routing only for deployment-based routes.
	var row routeRow
	stickyKey := s.conversationKey(req)
	if stickyKey != "" {
		if pinned, ok := s.getStickyRoute(ctx, rows, stickyKey); ok {
			row = pinned
		} else {
			group := lowestPriorityGroup(rows)
			row = weightedSelectRoute(group)
			if row.DeploymentID != nil {
				s.setStickyRoute(ctx, stickyKey, row)
			}
		}
	} else {
		group := lowestPriorityGroup(rows)
		row = weightedSelectRoute(group)
	}

	req.Candidate = s.buildCandidate(model.ID, row)

	if req.Candidate.IsPoolRoute() && s.oauthCreds != nil {
		cred, err := s.oauthCreds.SelectCredentialFromPool(ctx, row.strVal(row.PoolID), row.strVal(row.OAuthStrategy))
		if err != nil {
			return fmt.Errorf("select oauth credential: %w", err)
		}
		req.SelectedCredential = cred
		return nil
	}

	// Deployment-based: decrypt API key.
	if row.APIKeyCiphertext != nil && *row.APIKeyCiphertext != "" {
		key, err := secret.DecryptProviderKey(s.masterKey, *row.APIKeyCiphertext)
		if err != nil {
			return fmt.Errorf("decrypt api key: %w", err)
		}
		req.Candidate.APIKeyCiphertext = key
	}

	return nil
}

func (s *RouteSelector) buildCandidate(modelID string, row routeRow) *domain.RouteCandidate {
	c := &domain.RouteCandidate{
		RouteID:        row.RouteID,
		Priority:       int(row.RoutePriority),
		Weight:         int(row.RouteWeight),
		SupportsStream: row.SupportsStream,
		ModelID:        modelID,
	}

	if row.PoolID != nil {
		// Pool-based route
		c.PoolID = *row.PoolID
		c.PoolUpstreamModel = row.strVal(row.PoolUpstreamModel)
		c.FixedProviderType = domain.FixedProviderType(row.strVal(row.FixedProviderType))
		c.OAuthStrategy = row.strVal(row.OAuthStrategy)
		c.ProviderCode = row.strVal(row.FixedProviderType)
		c.UpstreamModel = c.PoolUpstreamModel
		c.BaseURL = domain.FixedProviderBaseURL(c.FixedProviderType)
		c.Protocol = domain.FixedProviderProtocol(c.FixedProviderType)
		c.TimeoutMs = 60000
		return c
	}

	// Deployment-based route
	c.DeploymentID = row.strVal(row.DeploymentID)
	c.UpstreamModel = row.strVal(row.UpstreamModel)
	c.CapabilityType = domain.CapabilityType(row.strVal(row.CapabilityType))
	c.Protocol = domain.UpstreamProtocol(row.strVal(row.UpstreamProtocol))
	c.RequestPath = row.strVal(row.RequestPath)
	c.HealthStatus = domain.HealthStatus(row.strVal(row.HealthStatus))
	c.EndpointID = row.strVal(row.EndpointID)
	c.BaseURL = row.strVal(row.BaseURL)
	c.TimeoutMs = int(row.TimeoutMs)
	c.ProviderCode = row.strVal(row.ProviderCode)
	c.ExtraHeaders = unmarshalStringMap(row.ExtraHeaders)
	if len(row.UpstreamParameters) > 0 {
		_ = json.Unmarshal(row.UpstreamParameters, &c.UpstreamParameters)
	}
	return c
}

func (r routeRow) strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *RouteSelector) RecordSuccess(deploymentID string) {
	s.breaker.RecordSuccess(deploymentID)
}

func (s *RouteSelector) RecordFailure(ctx context.Context, deploymentID string, errMsg string) {
	tripped := s.breaker.RecordFailure(deploymentID)
	if tripped {
		go func() {
			_ = s.q.UpdateDeploymentHealth(context.Background(), dbgen.UpdateDeploymentHealthParams{
				ID:              mustParseUUID(deploymentID),
				HealthStatus:    string(domain.HealthUnhealthy),
				LastHealthError: pgtype.Text{String: errMsg, Valid: true},
			})
		}()
	}
}

func (s *RouteSelector) BreakerSnapshot() []routing.BreakerState {
	return s.breaker.ListStates()
}

// filterOpenCircuits removes deployment routes with open circuit breakers.
// Pool routes always pass through (CB not applicable).
func (s *RouteSelector) filterOpenCircuits(rows []routeRow) []routeRow {
	out := rows[:0]
	for _, r := range rows {
		if r.DeploymentID != nil && s.breaker.IsOpen(*r.DeploymentID) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func filterStreamable(rows []routeRow) []routeRow {
	out := rows[:0]
	for _, r := range rows {
		if r.SupportsStream {
			out = append(out, r)
		}
	}
	return out
}

func lowestPriorityGroup(rows []routeRow) []routeRow {
	if len(rows) == 0 {
		return nil
	}
	minPri := rows[0].RoutePriority
	out := []routeRow{}
	for _, r := range rows {
		if r.RoutePriority < minPri {
			minPri = r.RoutePriority
			out = out[:0]
		}
		if r.RoutePriority == minPri {
			out = append(out, r)
		}
	}
	return out
}

func weightedSelectRoute(rows []routeRow) routeRow {
	if len(rows) == 1 {
		return rows[0]
	}
	total := int32(0)
	for _, r := range rows {
		total += r.RouteWeight
	}
	if total <= 0 {
		return rows[rand.Intn(len(rows))]
	}
	pick := rand.Int31n(total)
	cum := int32(0)
	for _, r := range rows {
		cum += r.RouteWeight
		if pick < cum {
			return r
		}
	}
	return rows[len(rows)-1]
}

// ============================================================================
// Sticky routing helpers (deployment routes only)
// ============================================================================

func (s *RouteSelector) conversationKey(req *serving.Request) string {
	if s.redis == nil || req.ConversationID == "" || req.APIKey == nil {
		return ""
	}
	identity := req.APIKey.TenantID
	if req.APIKey.UserID != "" {
		identity = req.APIKey.UserID
	}
	return conversationKeyPrefix + req.APIKey.TenantID + ":" + identity + ":" + req.ModelCode + ":" + req.ConversationID
}

func (s *RouteSelector) getStickyRoute(ctx context.Context, rows []routeRow, key string) (routeRow, bool) {
	if s.redis == nil {
		return routeRow{}, false
	}
	raw, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		return routeRow{}, false
	}
	var binding conversationBinding
	if err := json.Unmarshal([]byte(raw), &binding); err != nil {
		return routeRow{}, false
	}
	for _, r := range rows {
		if r.DeploymentID != nil && *r.DeploymentID == binding.DeploymentID &&
			r.EndpointID != nil && *r.EndpointID == binding.EndpointID {
			return r, true
		}
	}
	return routeRow{}, false
}

func (s *RouteSelector) setStickyRoute(ctx context.Context, key string, row routeRow) {
	if s.redis == nil || row.DeploymentID == nil {
		return
	}
	value, err := json.Marshal(conversationBinding{
		DeploymentID: *row.DeploymentID,
		EndpointID:   row.strVal(row.EndpointID),
	})
	if err != nil {
		return
	}
	_ = s.redis.Set(ctx, key, value, conversationBindingTTL).Err()
}

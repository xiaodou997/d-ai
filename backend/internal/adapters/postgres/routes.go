package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/routing"
	"uni-ai-api/backend/internal/secret"
	"uni-ai-api/backend/internal/serving"
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
	Pricing            []byte
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

	// P3 scoring hints
	CostPer1kTokens      float64
	ScoreWeightsOverride []byte // JSONB or NULL
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
		  ud.pricing,
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
		  NULL::text       AS oauth_strategy,
		  r.cost_per_1k_tokens,
		  r.score_weights_override
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
		  NULL::jsonb      AS pricing,
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
		  cp.oauth_strategy,
		  r.cost_per_1k_tokens,
		  r.score_weights_override
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
			&r.RequestPath, &r.UpstreamParameters, &r.Pricing, &r.HealthStatus,
			&r.EndpointID, &r.BaseURL, &r.APIKeyCiphertext, &r.ExtraHeaders,
			&r.TimeoutMs, &r.EndpointWeight, &r.ProviderCode,
			&r.PoolID, &r.PoolUpstreamModel, &r.FixedProviderType, &r.OAuthStrategy,
			&r.CostPer1kTokens, &r.ScoreWeightsOverride,
		); err != nil {
			return nil, fmt.Errorf("scan route row: %w", err)
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}

// RouteSelector loads all healthy routes for the requested model and selects
// one according to priority → weighted random selection.
// It satisfies serving.RouteSelector.
type RouteSelector struct {
	q            *dbgen.Queries
	pool         *pgxpool.Pool
	masterKey    string
	grantChecker *ModelGrantChecker
	health       routing.HealthTracker
	oauthCreds   *OAuthCredentialStore
}

func NewRouteSelector(q *dbgen.Queries, pool *pgxpool.Pool, masterKey string, gc *ModelGrantChecker) *RouteSelector {
	return &RouteSelector{
		q:            q,
		pool:         pool,
		masterKey:    masterKey,
		grantChecker: gc,
	}
}

// WithHealth injects the shared HealthTracker (must be called before serving).
func (s *RouteSelector) WithHealth(h routing.HealthTracker) *RouteSelector {
	s.health = h
	return s
}

func (s *RouteSelector) WithOAuthCredentialStore(store *OAuthCredentialStore) *RouteSelector {
	s.oauthCreds = store
	return s
}

// SelectCandidates returns the ordered list of routes that may serve this
// request. The caller (ExecuteStep) is responsible for picking one and
// retrying on the rest. API keys for deployment routes are decrypted eagerly
// so retries don't re-do the DB roundtrip.
//
// Sticky routing: if X-Conversation-Id maps to a known prior route via the
// Redis binding, that candidate is moved to the head of the list. If no
// binding exists, the first deployment-eligible candidate is written as the
// sticky target for the next call. Pool credential selection itself is
// deferred to ExecuteStep so that retries can swap credentials cleanly.
func (s *RouteSelector) SelectCandidates(ctx context.Context, req *serving.Request) ([]*domain.RouteCandidate, error) {
	model, err := s.grantChecker.resolveModelID(ctx,
		req.APIKey.TenantID, req.ModelCode, string(req.CapabilityType))
	if err != nil {
		return nil, fmt.Errorf("resolve model id: %w", err)
	}

	rows, err := s.listRoutesForModel(ctx, mustParseUUID(model.ID))
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no routes for model %q", req.ModelCode)
	}

	if req.IsStream {
		rows = filterStreamable(rows)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no healthy routes for model %q", req.ModelCode)
	}

	candidates := make([]*domain.RouteCandidate, 0, len(rows))
	for _, row := range rows {
		c := s.buildCandidate(model.ID, row)
		if !c.IsPoolRoute() && c.APIKeyCiphertext != "" {
			key, derr := secret.DecryptProviderKey(s.masterKey, c.APIKeyCiphertext)
			if derr != nil {
				slog.WarnContext(ctx, "decrypt api key failed, skipping route",
					"route_id", c.RouteID, "error", derr)
				continue
			}
			c.APIKeyCiphertext = key
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no usable candidates for model %q", req.ModelCode)
	}
	return candidates, nil
}

func (s *RouteSelector) buildCandidate(modelID string, row routeRow) *domain.RouteCandidate {
	c := &domain.RouteCandidate{
		RouteID:        row.RouteID,
		Priority:       int(row.RoutePriority),
		Weight:         int(row.RouteWeight),
		SupportsStream: row.SupportsStream,
		ModelID:        modelID,
		// P3 scoring hints
		CostPer1kTokens: row.CostPer1kTokens,
	}
	if len(row.ScoreWeightsOverride) > 0 {
		var m map[string]float64
		if err := json.Unmarshal(row.ScoreWeightsOverride, &m); err == nil {
			c.ScoreWeightsOverride = m
		}
	}

	if row.PoolID != nil {
		// Pool-based route — cost is 0 (free/OAuth), scorer will use costCapFree.
		c.CostPer1kTokens = 0
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
	if len(row.Pricing) > 0 {
		var p domain.Pricing
		if err := json.Unmarshal(row.Pricing, &p); err == nil {
			c.Pricing = &p
		}
	}
	return c
}

func (r routeRow) strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// HealthSnapshot returns a read-only view of all tracked health states for
// the admin /system/status endpoint.
func (s *RouteSelector) HealthSnapshot() []routing.HealthRecord {
	if s.health == nil {
		return nil
	}
	return s.health.Snapshot()
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


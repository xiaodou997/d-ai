package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"math/rand"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/uni-ai-api/internal/db/gen"
	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/routing"
	"xiaodou/uni-ai-api/internal/secret"
	"xiaodou/uni-ai-api/internal/serving"
)

// oauthFixedTypesForProtocol maps a client_protocol to the set of
// ai_credential_pools.fixed_provider_type values that natively serve it.
// Strict 1:1: only protocols handled by OAuth pools have entries here.
// Returns nil for protocols that are not served by any OAuth pool — callers
// should still allow API-key deployments matching the protocol directly.
func oauthFixedTypesForProtocol(p domain.UpstreamProtocol) []string {
	switch p {
	case domain.ProtocolOpenAIResponses:
		return []string{string(domain.FixedProviderCodex)}
	case domain.ProtocolAnthropicMessages:
		return []string{string(domain.FixedProviderClaudeOAuth)}
	case domain.ProtocolGeminiGenerate:
		return []string{string(domain.FixedProviderGeminiCLI), string(domain.FixedProviderAntigravity)}
	default:
		return nil
	}
}

// routeRow holds one resolved route. All endpoint/pool fields may be nil
// depending on whether the deployment is API Key-based or OAuth pool-based.
type routeRow struct {
	RouteID        string
	RoutePriority  int32
	RouteWeight    int32
	SupportsStream bool

	DeploymentID       *string
	UpstreamModel      *string
	CapabilityType     *string
	UpstreamProtocol   *string
	RequestPath        *string
	UpstreamParameters []byte
	Pricing            []byte
	HealthStatus       *string

	// Endpoint fields — nil for pool deployments
	EndpointID       *string
	BaseURL          *string
	APIKeyCiphertext *string
	ExtraHeaders     []byte
	EndpointWeight   int32
	ProviderCode     *string

	// Pool fields — nil for endpoint deployments
	PoolID            *string
	FixedProviderType *string
	OAuthStrategy     *string

	// P3 scoring hints
	CostPer1kTokens      float64
	ScoreWeightsOverride []byte

	// 三段式超时覆盖值（nullable，NULL=继承下一级）。R*=route 级，M*=model 级。
	RConnectMs, RFirstByteMs, RIdleMs, RMaxDurMs *int32
	MConnectMs, MFirstByteMs, MIdleMs, MMaxDurMs *int32
}

// listRoutesForModel fetches all active routes for a model that are compatible
// with the given client_protocol. Strict 1:1 protocol matching:
//   - API-key deployments (endpoint_id IS NOT NULL): ud.upstream_protocol must
//     equal clientProtocol.
//   - OAuth pool deployments (credential_pool_id IS NOT NULL): the pool's
//     fixed_provider_type must be in the set that natively serves clientProtocol
//     (e.g. codex serves openai_responses; claude_oauth serves anthropic_messages).
//
// Cross-protocol routing is intentionally not supported — the caller should
// fail fast with HTTP 400 when this returns an empty result.
func (s *RouteSelector) listRoutesForModel(ctx context.Context, modelID pgtype.UUID, clientProtocol domain.UpstreamProtocol) ([]routeRow, error) {
	const q = `
		SELECT
		  r.id::text                  AS route_id,
		  r.priority                  AS route_priority,
		  r.weight                    AS route_weight,
		  r.supports_stream,
		  ud.id::text                 AS deployment_id,
		  ud.upstream_model,
		  ud.capability_type,
		  ud.upstream_protocol,
		  ud.request_path,
		  ud.upstream_parameters,
		  ud.pricing,
		  ud.health_status,
		  e.id::text                  AS endpoint_id,
		  e.base_url,
		  e.api_key_ciphertext,
		  e.extra_headers,
		  COALESCE(e.weight, 100)       AS endpoint_weight,
		  p.code                       AS provider_code,
		  cp.id::text                  AS pool_id,
		  cp.fixed_provider_type,
		  cp.oauth_strategy,
		  r.cost_per_1k_tokens,
		  r.score_weights_override,
		  r.connect_timeout_ms, r.first_byte_timeout_ms, r.idle_timeout_ms, r.max_duration_ms,
		  m.connect_timeout_ms, m.first_byte_timeout_ms, m.idle_timeout_ms, m.max_duration_ms
		FROM ai_model_routes r
		JOIN ai_models m ON m.id = r.model_id
		JOIN ai_upstream_deployments ud ON ud.id = r.upstream_deployment_id
		LEFT JOIN ai_provider_endpoints e  ON e.id  = ud.endpoint_id
		LEFT JOIN ai_providers          p  ON p.id  = e.provider_id
		LEFT JOIN ai_credential_pools   cp ON cp.id = ud.credential_pool_id
		WHERE r.model_id         = $1
		  AND r.status           = 'active'
		  AND ud.status          = 'active'
		  AND ud.health_status   IN ('healthy', 'unknown')
		  AND (e.status = 'active' OR e.status IS NULL)
		  AND (p.status = 'active' OR p.status IS NULL)
		  AND (
		    (ud.endpoint_id IS NOT NULL AND ud.upstream_protocol = $2)
		    OR
		    (ud.credential_pool_id IS NOT NULL AND cp.fixed_provider_type = ANY($3::text[]))
		  )
		ORDER BY route_priority ASC, route_weight DESC, endpoint_weight DESC`

	oauthTypes := oauthFixedTypesForProtocol(clientProtocol)
	if oauthTypes == nil {
		oauthTypes = []string{} // pgx encodes empty slice as empty array; ANY({}) → false
	}
	pgRows, err := s.pool.Query(ctx, q, modelID, string(clientProtocol), oauthTypes)
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
			&r.EndpointWeight, &r.ProviderCode,
			&r.PoolID, &r.FixedProviderType, &r.OAuthStrategy,
			&r.CostPer1kTokens, &r.ScoreWeightsOverride,
			&r.RConnectMs, &r.RFirstByteMs, &r.RIdleMs, &r.RMaxDurMs,
			&r.MConnectMs, &r.MFirstByteMs, &r.MIdleMs, &r.MMaxDurMs,
		); err != nil {
			return nil, fmt.Errorf("scan route row: %w", err)
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}

// ModelsWithProtocolRoute returns, as a set, the subset of modelIDs that are
// active and have at least one active route reachable over clientProtocol.
// When requireStream is true only routes whose ai_model_routes.supports_stream
// is true count.
//
// It answers "which of these models can a console feature speaking
// clientProtocol actually call" and is used to filter the web console model
// picker. Transient health (health_status) is intentionally NOT considered —
// this reflects configuration, so the picker stays stable while the runtime
// path still fails over or surfaces health errors on its own.
func (s *RouteSelector) ModelsWithProtocolRoute(
	ctx context.Context,
	modelIDs []pgtype.UUID,
	clientProtocol domain.UpstreamProtocol,
	requireStream bool,
) (map[pgtype.UUID]bool, error) {
	out := make(map[pgtype.UUID]bool, len(modelIDs))
	if len(modelIDs) == 0 {
		return out, nil
	}
	const q = `
		SELECT DISTINCT r.model_id
		FROM ai_model_routes r
		JOIN ai_models               m  ON m.id  = r.model_id
		JOIN ai_upstream_deployments ud ON ud.id = r.upstream_deployment_id
		LEFT JOIN ai_provider_endpoints e  ON e.id  = ud.endpoint_id
		LEFT JOIN ai_providers          p  ON p.id  = e.provider_id
		LEFT JOIN ai_credential_pools   cp ON cp.id = ud.credential_pool_id
		WHERE r.model_id = ANY($1)
		  AND m.status  = 'active'
		  AND r.status  = 'active'
		  AND ud.status = 'active'
		  AND (e.status = 'active' OR e.status IS NULL)
		  AND (p.status = 'active' OR p.status IS NULL)
		  AND (NOT $4::boolean OR r.supports_stream)
		  AND (
		    (ud.endpoint_id IS NOT NULL AND ud.upstream_protocol = $2)
		    OR
		    (ud.credential_pool_id IS NOT NULL AND cp.fixed_provider_type = ANY($3::text[]))
		  )`

	oauthTypes := oauthFixedTypesForProtocol(clientProtocol)
	if oauthTypes == nil {
		oauthTypes = []string{}
	}
	rows, err := s.pool.Query(ctx, q, modelIDs, string(clientProtocol), oauthTypes, requireStream)
	if err != nil {
		return nil, fmt.Errorf("models with protocol route: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan model id: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// RouteSelector loads all healthy routes for the requested model and selects
// one according to priority → weighted random selection.
// It satisfies serving.RouteSelector.
type RouteSelector struct {
	q               *dbgen.Queries
	pool            *pgxpool.Pool
	masterKey       string
	grantChecker    *ModelGrantChecker
	health          routing.HealthTracker
	oauthCreds      *OAuthCredentialStore
	defaultTimeouts domain.RouteTimeouts // 全局兜底超时（route/model 未配置时使用）
}

func NewRouteSelector(q *dbgen.Queries, pool *pgxpool.Pool, masterKey string, gc *ModelGrantChecker) *RouteSelector {
	return &RouteSelector{
		q:            q,
		pool:         pool,
		masterKey:    masterKey,
		grantChecker: gc,
		// 均衡档兜底值；正常由 WithDefaultTimeouts 从 config 注入覆盖。
		defaultTimeouts: domain.RouteTimeouts{
			Connect:     10 * time.Second,
			FirstByte:   60 * time.Second,
			Idle:        60 * time.Second,
			MaxDuration: 15 * time.Minute,
		},
	}
}

// WithDefaultTimeouts injects the global default 三段式 timeouts (from config).
// These are the lowest priority in the route > model > global chain.
func (s *RouteSelector) WithDefaultTimeouts(t domain.RouteTimeouts) *RouteSelector {
	s.defaultTimeouts = t
	return s
}

// resolveTimeouts flattens the route > model > global timeout chain for one row.
func (s *RouteSelector) resolveTimeouts(row routeRow) domain.RouteTimeouts {
	pick := func(route, model *int32, global time.Duration) time.Duration {
		if route != nil {
			return time.Duration(*route) * time.Millisecond
		}
		if model != nil {
			return time.Duration(*model) * time.Millisecond
		}
		return global
	}
	return domain.RouteTimeouts{
		Connect:     pick(row.RConnectMs, row.MConnectMs, s.defaultTimeouts.Connect),
		FirstByte:   pick(row.RFirstByteMs, row.MFirstByteMs, s.defaultTimeouts.FirstByte),
		Idle:        pick(row.RIdleMs, row.MIdleMs, s.defaultTimeouts.Idle),
		MaxDuration: pick(row.RMaxDurMs, row.MMaxDurMs, s.defaultTimeouts.MaxDuration),
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
	identity := req.RuntimeIdentity()
	if identity == nil {
		return nil, fmt.Errorf("missing runtime identity")
	}
	model, err := s.grantChecker.resolveModelID(ctx,
		identity.TenantID, req.ModelCode, string(req.CapabilityType))
	if err != nil {
		return nil, fmt.Errorf("resolve model id: %w", err)
	}

	rows, err := s.listRoutesForModel(ctx, mustParseUUID(model.ID), req.ClientProtocol)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	if len(rows) == 0 {
		return nil, &serving.APIError{
			Status: http.StatusBadRequest,
			Code:   "no_matching_deployment",
			Message: fmt.Sprintf("no upstream deployment configured for model %q with client protocol %q; configure a deployment whose upstream_protocol matches, or use a different client endpoint",
				req.ModelCode, string(req.ClientProtocol)),
		}
	}

	if req.IsStream {
		rows = filterStreamable(rows)
	}
	if len(rows) == 0 {
		return nil, &serving.APIError{
			Status:  http.StatusBadRequest,
			Code:    "no_matching_deployment",
			Message: fmt.Sprintf("no streaming-capable deployment for model %q with client protocol %q", req.ModelCode, string(req.ClientProtocol)),
		}
	}

	candidates := make([]*domain.RouteCandidate, 0, len(rows))
	for _, row := range rows {
		c := s.buildCandidate(model.ID, row)
		if !c.IsPoolRoute() && c.APIKeyCiphertext != "" {
			key, derr := secret.DecryptProviderKey(s.masterKey, c.APIKeyCiphertext)
			if derr != nil {
				zap.L().Warn("decrypt api key failed, skipping route",
					zap.String("route_id", c.RouteID), zap.Error(derr))
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
	c.Timeouts = s.resolveTimeouts(row)

	if row.PoolID != nil {
		// Pool-based route — cost is 0 (free/OAuth), scorer will use costCapFree.
		c.CostPer1kTokens = 0
		c.PoolID = *row.PoolID
		c.FixedProviderType = domain.FixedProviderType(row.strVal(row.FixedProviderType))
		c.OAuthStrategy = row.strVal(row.OAuthStrategy)
		c.ProviderCode = row.strVal(row.FixedProviderType)
		c.UpstreamModel = row.strVal(row.UpstreamModel)
		c.PoolUpstreamModel = c.UpstreamModel
		c.BaseURL = domain.FixedProviderBaseURL(c.FixedProviderType)
		c.Protocol = domain.FixedProviderProtocol(c.FixedProviderType)
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
	c.APIKeyCiphertext = row.strVal(row.APIKeyCiphertext)
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

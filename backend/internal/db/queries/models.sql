-- ============================================================================
-- Runtime Model Queries
-- ============================================================================

-- name: ListModelsForTenant :many
SELECT
  m.id,
  m.model_code,
  m.display_name,
  m.capability_type,
  m.context_window,
  m.default_max_output_tokens,
  m.max_output_tokens
FROM ai_models m
JOIN ai_tenant_model_grants tg ON tg.model_id = m.id
WHERE tg.tenant_id = $1
  AND tg.status = 'active'
  AND m.status = 'active'
ORDER BY m.model_code ASC;

-- name: GetTenantModel :one
SELECT
  m.id,
  m.model_code,
  m.display_name,
  m.capability_type,
  m.default_max_output_tokens
FROM ai_models m
JOIN ai_tenant_model_grants tg ON tg.model_id = m.id
WHERE tg.tenant_id = $1
  AND m.model_code = $2
  AND m.capability_type = $3
  AND tg.status = 'active'
  AND m.status = 'active';

-- name: ListRoutesForModel :many
-- Runtime route lookup: model -> routes -> upstream_deployments -> endpoints -> providers
SELECT
  r.id AS route_id,
  r.priority AS route_priority,
  r.weight AS route_weight,
  r.supports_stream,
  ud.id AS upstream_deployment_id,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.request_path,
  ud.upstream_parameters,
  ud.health_status,
  e.id AS endpoint_id,
  e.base_url,
  e.api_key_ciphertext,
  e.extra_headers,
  e.timeout_ms,
  e.weight AS endpoint_weight,
  p.id AS provider_id,
  p.code AS provider_code
FROM ai_model_routes r
JOIN ai_upstream_deployments ud ON ud.id = r.upstream_deployment_id
JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
JOIN ai_providers p ON p.id = e.provider_id
WHERE r.model_id = $1
  AND r.status = 'active'
  AND ud.status = 'active'
  AND ud.health_status IN ('healthy', 'unknown')
  AND e.status = 'active'
  AND p.status = 'active'
ORDER BY r.priority ASC, r.weight DESC, e.weight DESC;
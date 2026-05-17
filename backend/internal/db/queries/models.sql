-- ============================================================================
-- Runtime Model Queries
-- ============================================================================

-- name: ListModelsForTenant :many
-- Lists models the tenant is granted access to.
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
  AND tg.status   = 'active'
  AND m.status    = 'active'
ORDER BY m.model_code ASC;

-- name: GetTenantModel :one
-- Validates a model is accessible to the given tenant.
SELECT
  m.id,
  m.model_code,
  m.display_name,
  m.capability_type,
  m.default_max_output_tokens,
  m.max_output_tokens
FROM ai_models m
JOIN ai_tenant_model_grants tg ON tg.model_id = m.id
WHERE tg.tenant_id      = $1
  AND m.model_code      = $2
  AND m.capability_type = $3
  AND tg.status         = 'active'
  AND m.status          = 'active';

-- name: GetUserModel :one
-- Validates a model is accessible to a specific user (explicit user grant).
-- When user has no explicit grants, caller falls back to tenant-level check.
SELECT
  m.id,
  m.model_code,
  m.display_name,
  m.capability_type,
  m.default_max_output_tokens,
  m.max_output_tokens
FROM ai_models m
JOIN ai_user_model_grants ug ON ug.model_id = m.id
WHERE ug.tenant_id      = $1
  AND ug.user_id        = $2
  AND m.model_code      = $3
  AND m.capability_type = $4
  AND ug.status         = 'active'
  AND m.status          = 'active';

-- name: HasUserModelGrants :one
-- Returns true if the user has ANY explicit model grants.
-- Used to decide whether to apply user-level filtering or fall back to tenant grants.
SELECT EXISTS(
  SELECT 1 FROM ai_user_model_grants
  WHERE tenant_id = $1
    AND user_id   = $2
    AND status    = 'active'
) AS has_grants;

-- name: ListRoutesForModel :many
-- Full route lookup: model → routes → upstream_deployments → endpoints → providers.
-- Only healthy/unknown deployments on active endpoints are returned.
SELECT
  r.id                   AS route_id,
  r.priority             AS route_priority,
  r.weight               AS route_weight,
  r.supports_stream,
  ud.id                  AS upstream_deployment_id,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.request_path,
  ud.upstream_parameters,
  ud.health_status,
  e.id                   AS endpoint_id,
  e.base_url,
  e.api_key_ciphertext,
  e.extra_headers,
  e.timeout_ms,
  e.weight               AS endpoint_weight,
  e.auth_type,
  e.fixed_provider_type,
  e.oauth_strategy,
  p.id                   AS provider_id,
  p.code                 AS provider_code
FROM ai_model_routes r
JOIN ai_upstream_deployments ud ON ud.id = r.upstream_deployment_id
JOIN ai_provider_endpoints   e  ON e.id  = ud.endpoint_id
JOIN ai_providers            p  ON p.id  = e.provider_id
WHERE r.model_id       = $1
  AND r.status         = 'active'
  AND ud.status        = 'active'
  AND ud.health_status IN ('healthy', 'unknown')
  AND e.status         = 'active'
  AND p.status         = 'active'
ORDER BY r.priority ASC, r.weight DESC, e.weight DESC;

-- name: UpdateDeploymentHealth :exec
UPDATE ai_upstream_deployments
SET
  health_status        = $2,
  last_health_check_at = now(),
  last_health_error    = $3,
  updated_at           = now()
WHERE id = $1;


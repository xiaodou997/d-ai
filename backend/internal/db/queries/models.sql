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

-- name: GetUserModel :one
SELECT
  m.id,
  m.model_code,
  m.display_name,
  m.capability_type,
  m.default_max_output_tokens
FROM ai_models m
JOIN ai_tenant_model_grants tg ON tg.model_id = m.id
JOIN ai_user_model_grants ug ON ug.model_id = m.id
WHERE tg.tenant_id = $1
  AND ug.tenant_id = $1
  AND ug.user_id = $2
  AND m.model_code = $3
  AND m.capability_type = $4
  AND tg.status = 'active'
  AND ug.status = 'active'
  AND m.status = 'active';

-- name: ListDeploymentsForModel :many
SELECT
  d.id AS deployment_id,
  d.upstream_model,
  d.capability_type,
  d.upstream_protocol,
  d.upstream_parameters,
  d.priority,
  d.weight AS deployment_weight,
  d.supports_stream,
  e.id AS endpoint_id,
  e.base_url,
  e.protocol_type AS endpoint_protocol_type,
  e.api_key_ciphertext,
  e.custom_path,
  e.extra_headers,
  e.timeout_ms,
  e.weight AS endpoint_weight,
  p.id AS provider_id,
  p.code AS provider_code
FROM ai_model_deployments d
JOIN ai_provider_endpoints e ON e.id = d.endpoint_id
JOIN ai_providers p ON p.id = e.provider_id
WHERE d.model_id = $1
  AND d.capability_type = $2
  AND d.status = 'active'
  AND e.status = 'active'
  AND e.health_status IN ('healthy', 'unknown')
  AND p.status = 'active'
ORDER BY d.priority ASC, d.weight DESC, e.weight DESC;

-- name: ListModelsForUser :many
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
JOIN ai_user_model_grants ug ON ug.model_id = m.id
WHERE tg.tenant_id = $1
  AND ug.tenant_id = $1
  AND ug.user_id = $2
  AND tg.status = 'active'
  AND ug.status = 'active'
  AND m.status = 'active'
ORDER BY m.model_code ASC;

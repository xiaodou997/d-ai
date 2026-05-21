-- ============================================================================
-- Provider CRUD
-- ============================================================================

-- name: CreateProvider :one
INSERT INTO ai_providers (
  code,
  name,
  config,
  status
) VALUES (
  $1, $2, $3, $4
)
RETURNING
  id,
  code,
  name,
  config,
  status,
  created_at,
  updated_at;

-- name: ListProviders :many
SELECT
  id,
  code,
  name,
  config,
  status,
  created_at,
  updated_at
FROM ai_providers
ORDER BY code ASC;

-- name: GetProvider :one
SELECT
  id,
  code,
  name,
  config,
  status,
  created_at,
  updated_at
FROM ai_providers
WHERE id = $1;

-- name: UpdateProviderStatus :one
UPDATE ai_providers
SET status = $2,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  code,
  name,
  config,
  status,
  created_at,
  updated_at;

-- name: UpdateProvider :one
UPDATE ai_providers
SET code = $2,
    name = $3,
    config = $4,
    status = $5,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  code,
  name,
  config,
  status,
  created_at,
  updated_at;

-- ============================================================================
-- Endpoint CRUD
-- ============================================================================

-- name: CreateProviderEndpoint :one
INSERT INTO ai_provider_endpoints (
  provider_id,
  name,
  base_url,
  api_key_ciphertext,
  extra_headers,
  weight,
  timeout_ms,
  default_protocol,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING
  id,
  provider_id,
  name,
  base_url,
  extra_headers,
  weight,
  timeout_ms,
  default_protocol,
  status,
  created_at,
  updated_at;

-- name: ListProviderEndpoints :many
SELECT
  id,
  provider_id,
  name,
  base_url,
  extra_headers,
  weight,
  timeout_ms,
  default_protocol,
  status,
  created_at,
  updated_at
FROM ai_provider_endpoints
WHERE provider_id = $1
ORDER BY name ASC;

-- name: GetProviderEndpoint :one
SELECT
  id,
  provider_id,
  name,
  base_url,
  api_key_ciphertext,
  extra_headers,
  weight,
  timeout_ms,
  default_protocol,
  status,
  created_at,
  updated_at
FROM ai_provider_endpoints
WHERE provider_id = $1
  AND id = $2;

-- name: UpdateProviderEndpointStatus :one
UPDATE ai_provider_endpoints
SET status = $3,
    updated_at = now()
WHERE provider_id = $1
  AND id = $2
RETURNING
  id,
  provider_id,
  name,
  base_url,
  extra_headers,
  weight,
  timeout_ms,
  default_protocol,
  status,
  created_at,
  updated_at;

-- name: UpdateProviderEndpoint :one
UPDATE ai_provider_endpoints
SET name = $3,
    base_url = $4,
    api_key_ciphertext = $5,
    extra_headers = $6,
    weight = $7,
    timeout_ms = $8,
    default_protocol = $9,
    status = $10,
    updated_at = now()
WHERE provider_id = $1
  AND id = $2
RETURNING
  id,
  provider_id,
  name,
  base_url,
  extra_headers,
  weight,
  timeout_ms,
  default_protocol,
  status,
  created_at,
  updated_at;

-- ============================================================================
-- Upstream Deployment CRUD
-- ============================================================================

-- name: CreateUpstreamDeployment :one
INSERT INTO ai_upstream_deployments (
  endpoint_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  pricing,
  health_status,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, 'unknown', $8
)
RETURNING
  id,
  endpoint_id,
  credential_pool_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  pricing,
  health_status,
  last_health_check_at,
  last_health_error,
  status,
  created_at,
  updated_at;

-- name: CreatePoolDeployment :one
INSERT INTO ai_upstream_deployments (
  credential_pool_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  status
) VALUES (
  $1, $2, $3, $4, 'active'
)
RETURNING
  id,
  endpoint_id,
  credential_pool_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  pricing,
  health_status,
  last_health_check_at,
  last_health_error,
  status,
  created_at,
  updated_at;

-- name: ListUpstreamDeployments :many
SELECT
  ud.id,
  ud.endpoint_id,
  ud.credential_pool_id,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.request_path,
  ud.upstream_parameters,
  ud.pricing,
  ud.health_status,
  ud.last_health_check_at,
  ud.last_health_error,
  ud.status,
  ud.created_at,
  ud.updated_at,
  CASE WHEN ud.endpoint_id IS NOT NULL THEN 'endpoint' ELSE 'pool' END AS credential_source,
  COALESCE(e.name, '')          AS endpoint_name,
  COALESCE(e.base_url, '')      AS base_url,
  COALESCE(e.weight, 0)         AS endpoint_weight,
  COALESCE(p.id, '00000000-0000-0000-0000-000000000000'::uuid) AS provider_id,
  COALESCE(p.code, '')          AS provider_code,
  COALESCE(p.name, '')          AS provider_name,
  COALESCE(cp.name, '')         AS pool_name,
  COALESCE(cp.fixed_provider_type, '') AS fixed_provider_type
FROM ai_upstream_deployments ud
LEFT JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
LEFT JOIN ai_providers p ON p.id = e.provider_id
LEFT JOIN ai_credential_pools cp ON cp.id = ud.credential_pool_id
WHERE ud.endpoint_id = $1
ORDER BY ud.upstream_model ASC;

-- name: GetUpstreamDeployment :one
SELECT
  ud.id,
  ud.endpoint_id,
  ud.credential_pool_id,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.request_path,
  ud.upstream_parameters,
  ud.pricing,
  ud.health_status,
  ud.last_health_check_at,
  ud.last_health_error,
  ud.status,
  ud.created_at,
  ud.updated_at,
  CASE WHEN ud.endpoint_id IS NOT NULL THEN 'endpoint' ELSE 'pool' END AS credential_source,
  COALESCE(e.name, '')          AS endpoint_name,
  COALESCE(e.base_url, '')      AS base_url,
  COALESCE(e.api_key_ciphertext, '') AS api_key_ciphertext,
  COALESCE(e.extra_headers, '{}'::jsonb) AS extra_headers,
  COALESCE(e.timeout_ms, 30000) AS timeout_ms,
  COALESCE(p.id, '00000000-0000-0000-0000-000000000000'::uuid) AS provider_id,
  COALESCE(p.code, '')          AS provider_code,
  COALESCE(p.name, '')          AS provider_name,
  COALESCE(cp.name, '')         AS pool_name,
  COALESCE(cp.fixed_provider_type, '') AS fixed_provider_type
FROM ai_upstream_deployments ud
LEFT JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
LEFT JOIN ai_providers p ON p.id = e.provider_id
LEFT JOIN ai_credential_pools cp ON cp.id = ud.credential_pool_id
WHERE ud.id = $1;

-- name: UpdateUpstreamDeploymentStatus :one
UPDATE ai_upstream_deployments
SET status = $2,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  endpoint_id,
  credential_pool_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  pricing,
  health_status,
  last_health_check_at,
  last_health_error,
  status,
  created_at,
  updated_at;

-- name: UpdateUpstreamDeployment :one
UPDATE ai_upstream_deployments
SET upstream_model = $2,
    capability_type = $3,
    upstream_protocol = $4,
    request_path = $5,
    upstream_parameters = $6,
    pricing = $7,
    status = $8,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  endpoint_id,
  credential_pool_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  pricing,
  health_status,
  last_health_check_at,
  last_health_error,
  status,
  created_at,
  updated_at;

-- name: UpdateUpstreamDeploymentHealth :one
UPDATE ai_upstream_deployments
SET health_status = $2,
    last_health_check_at = now(),
    last_health_error = $3,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  endpoint_id,
  credential_pool_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  pricing,
  health_status,
  last_health_check_at,
  last_health_error,
  status,
  created_at,
  updated_at;

-- name: GetUpstreamDeploymentForHealthCheck :one
SELECT
  ud.id,
  ud.endpoint_id,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.request_path,
  ud.upstream_parameters,
  ud.health_status,
  e.name AS endpoint_name,
  e.base_url,
  e.api_key_ciphertext,
  e.extra_headers,
  e.timeout_ms,
  e.default_protocol AS endpoint_default_protocol,
  p.code AS provider_code,
  p.name AS provider_name
FROM ai_upstream_deployments ud
JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
JOIN ai_providers p ON p.id = e.provider_id
WHERE ud.id = $1;

-- ============================================================================
-- Model CRUD
-- ============================================================================

-- name: CreateModel :one
INSERT INTO ai_models (
  model_code,
  capability_type,
  context_window,
  default_max_output_tokens,
  max_output_tokens,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6
)
RETURNING
  id,
  model_code,
  capability_type,
  context_window,
  default_max_output_tokens,
  max_output_tokens,
  status,
  created_at,
  updated_at;

-- name: ListAdminModels :many
SELECT
  id,
  model_code,
  capability_type,
  context_window,
  default_max_output_tokens,
  max_output_tokens,
  status,
  created_at,
  updated_at
FROM ai_models
ORDER BY model_code ASC;

-- name: GetModel :one
SELECT
  id,
  model_code,
  capability_type,
  context_window,
  default_max_output_tokens,
  max_output_tokens,
  status,
  created_at,
  updated_at
FROM ai_models
WHERE id = $1;

-- name: UpdateModelStatus :one
UPDATE ai_models
SET status = $2,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  model_code,
  capability_type,
  context_window,
  default_max_output_tokens,
  max_output_tokens,
  status,
  created_at,
  updated_at;

-- name: UpdateModel :one
UPDATE ai_models
SET model_code = $2,
    capability_type = $3,
    context_window = $4,
    default_max_output_tokens = $5,
    max_output_tokens = $6,
    status = $7,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  model_code,
  capability_type,
  context_window,
  default_max_output_tokens,
  max_output_tokens,
  status,
  created_at,
  updated_at;

-- ============================================================================
-- Model Price CRUD (1:1 with model, upsert pattern)
-- ============================================================================

-- name: GetModelPrice :one
SELECT
  id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_at,
  updated_at
FROM ai_model_prices
WHERE model_id = $1;

-- name: UpsertModelPrice :one
INSERT INTO ai_model_prices (
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (model_id) DO UPDATE SET
  input_price_per_1m           = EXCLUDED.input_price_per_1m,
  output_price_per_1m          = EXCLUDED.output_price_per_1m,
  image_prices            = EXCLUDED.image_prices,
  video_prices       = EXCLUDED.video_prices,
  audio_tts_price_per_1m_chars = EXCLUDED.audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute   = EXCLUDED.audio_stt_price_per_minute,
  updated_at                   = now()
RETURNING
  id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_at,
  updated_at;

-- ============================================================================
-- Model Route CRUD
-- ============================================================================

-- name: CreateModelRoute :one
INSERT INTO ai_model_routes (
  model_id,
  upstream_deployment_id,
  priority,
  weight,
  supports_stream,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: ListModelRoutes :many
SELECT
  r.id,
  r.model_id,
  r.upstream_deployment_id,
  r.priority,
  r.weight,
  r.supports_stream,
  r.status,
  r.created_at,
  r.updated_at,
  ud.upstream_model                                               AS upstream_deployment_name,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.health_status,
  CASE WHEN ud.endpoint_id IS NOT NULL THEN 'endpoint' ELSE 'pool' END AS credential_source,
  COALESCE(e.id, '00000000-0000-0000-0000-000000000000'::uuid)   AS endpoint_id,
  COALESCE(e.name, '')                                           AS endpoint_name,
  COALESCE(e.base_url, '')                                       AS base_url,
  COALESCE(p.id, '00000000-0000-0000-0000-000000000000'::uuid)   AS provider_id,
  COALESCE(p.code, '')                                           AS provider_code,
  COALESCE(p.name, '')                                           AS provider_name,
  COALESCE(cp.id, '00000000-0000-0000-0000-000000000000'::uuid)  AS pool_id,
  COALESCE(cp.name, '')                                          AS pool_name,
  COALESCE(cp.fixed_provider_type, '')                           AS fixed_provider_type
FROM ai_model_routes r
JOIN ai_upstream_deployments ud ON ud.id = r.upstream_deployment_id
LEFT JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
LEFT JOIN ai_providers p ON p.id = e.provider_id
LEFT JOIN ai_credential_pools cp ON cp.id = ud.credential_pool_id
WHERE r.model_id = $1
ORDER BY r.priority ASC, r.weight DESC;

-- name: GetModelRoute :one
SELECT *
FROM ai_model_routes
WHERE id = $1;

-- name: UpdateModelRouteStatus :one
UPDATE ai_model_routes
SET status = $3,
    updated_at = now()
WHERE model_id = $1
  AND id = $2
RETURNING *;

-- name: UpdateModelRoute :one
UPDATE ai_model_routes
SET upstream_deployment_id = $3,
    priority = $4,
    weight = $5,
    supports_stream = $6,
    status = $7,
    updated_at = now()
WHERE model_id = $1
  AND id = $2
RETURNING *;

-- ============================================================================
-- Tenant Model Grants CRUD
-- ============================================================================

-- name: GrantModelToTenant :one
INSERT INTO ai_tenant_model_grants (
  tenant_id,
  model_id,
  status,
  created_by
) VALUES (
  $1, $2, $3, $4
)
ON CONFLICT (tenant_id, model_id) DO UPDATE SET
  status = EXCLUDED.status,
  created_by = EXCLUDED.created_by
RETURNING
  id,
  tenant_id,
  model_id,
  status,
  created_by,
  created_at;

-- name: ListTenantModelGrants :many
SELECT
  tg.id,
  tg.tenant_id,
  tg.model_id,
  tg.status,
  tg.created_by,
  tg.created_at,
  m.model_code,
  m.capability_type
FROM ai_tenant_model_grants tg
JOIN ai_models m ON m.id = tg.model_id
WHERE tg.tenant_id = $1
ORDER BY m.model_code ASC;

-- name: UpdateTenantModelGrantStatus :one
UPDATE ai_tenant_model_grants
SET status = $3
WHERE tenant_id = $1
  AND model_id = $2
RETURNING
  id,
  tenant_id,
  model_id,
  status,
  created_by,
  created_at;

-- ============================================================================
-- Tenant Model Price Overrides CRUD
-- ============================================================================

-- name: GetTenantModelPriceOverride :one
SELECT
  id,
  tenant_id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_by,
  created_at,
  updated_at
FROM ai_tenant_model_price_overrides
WHERE tenant_id = $1
  AND model_id = $2;

-- name: UpsertTenantModelPriceOverride :one
INSERT INTO ai_tenant_model_price_overrides (
  tenant_id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (tenant_id, model_id) DO UPDATE SET
  input_price_per_1m           = EXCLUDED.input_price_per_1m,
  output_price_per_1m          = EXCLUDED.output_price_per_1m,
  image_prices            = EXCLUDED.image_prices,
  video_prices       = EXCLUDED.video_prices,
  audio_tts_price_per_1m_chars = EXCLUDED.audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute   = EXCLUDED.audio_stt_price_per_minute,
  updated_at                   = now()
RETURNING
  id,
  tenant_id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_by,
  created_at,
  updated_at;

-- name: DeleteTenantModelPriceOverride :exec
DELETE FROM ai_tenant_model_price_overrides
WHERE tenant_id = $1
  AND model_id = $2;

-- name: ListTenantModelPriceOverrides :many
SELECT
  o.id,
  o.tenant_id,
  o.model_id,
  o.input_price_per_1m,
  o.output_price_per_1m,
  o.image_prices,
  o.video_prices,
  o.audio_tts_price_per_1m_chars,
  o.audio_stt_price_per_minute,
  o.created_by,
  o.created_at,
  o.updated_at,
  m.model_code,
  m.capability_type
FROM ai_tenant_model_price_overrides o
JOIN ai_models m ON m.id = o.model_id
WHERE o.tenant_id = $1
ORDER BY m.model_code ASC;

-- ============================================================================
-- Tenant User Prices CRUD (租户售价 - 租户对用户的定价)
-- ============================================================================

-- name: GetTenantUserPrice :one
SELECT
  id,
  tenant_id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_by,
  created_at,
  updated_at
FROM ai_tenant_user_prices
WHERE tenant_id = $1
  AND model_id = $2;

-- name: UpsertTenantUserPrice :one
INSERT INTO ai_tenant_user_prices (
  tenant_id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (tenant_id, model_id) DO UPDATE SET
  input_price_per_1m           = EXCLUDED.input_price_per_1m,
  output_price_per_1m          = EXCLUDED.output_price_per_1m,
  image_prices            = EXCLUDED.image_prices,
  video_prices       = EXCLUDED.video_prices,
  audio_tts_price_per_1m_chars = EXCLUDED.audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute   = EXCLUDED.audio_stt_price_per_minute,
  updated_at                   = now()
RETURNING
  id,
  tenant_id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_by,
  created_at,
  updated_at;

-- name: DeleteTenantUserPrice :exec
DELETE FROM ai_tenant_user_prices
WHERE tenant_id = $1
  AND model_id = $2;

-- name: ListTenantUserPrices :many
SELECT
  p.id,
  p.tenant_id,
  p.model_id,
  p.input_price_per_1m,
  p.output_price_per_1m,
  p.image_prices,
  p.video_prices,
  p.audio_tts_price_per_1m_chars,
  p.audio_stt_price_per_minute,
  p.created_by,
  p.created_at,
  p.updated_at,
  m.model_code,
  m.capability_type
FROM ai_tenant_user_prices p
JOIN ai_models m ON m.id = p.model_id
WHERE p.tenant_id = $1
ORDER BY m.model_code ASC;


-- ============================================================================
-- Usage Logs Queries
-- ============================================================================

-- name: ListUsageLogs :many
SELECT
  id,
  request_id,
  trace_id,
  api_key_id,
  key_owner_type,
  tenant_id,
  user_id,
  external_user_id,
  model_id,
  model_code,
  model_route_id,
  upstream_deployment_id,
  endpoint_id,
  provider_code,
  upstream_model,
  conversation_id,
  stream,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  billable_unit_type,
  billable_units,
  provider_cost,
  platform_cost,
  user_cost,
  api_key_quota_cost,
  urm_transaction_id,
  billing_status,
  request_status,
  http_status,
  upstream_status,
  latency_ms,
  first_token_latency_ms,
  error_code,
  error_message,
  usage_estimated,
  usage_source,
  created_at
FROM ai_usage_logs
WHERE tenant_id = $1
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text)
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code')::text)
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status')::text)
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at <= sqlc.narg('date_to')::timestamptz)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountUsageLogs :one
SELECT COUNT(*) AS count
FROM ai_usage_logs
WHERE tenant_id = $1
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text)
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code')::text)
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status')::text)
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at <= sqlc.narg('date_to')::timestamptz);

-- name: ListUsageLogsByAPIKey :many
SELECT
  id,
  request_id,
  trace_id,
  api_key_id,
  key_owner_type,
  tenant_id,
  user_id,
  external_user_id,
  model_id,
  model_code,
  model_route_id,
  upstream_deployment_id,
  endpoint_id,
  provider_code,
  upstream_model,
  conversation_id,
  stream,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  billable_unit_type,
  billable_units,
  provider_cost,
  platform_cost,
  user_cost,
  api_key_quota_cost,
  urm_transaction_id,
  billing_status,
  request_status,
  http_status,
  upstream_status,
  latency_ms,
  first_token_latency_ms,
  error_code,
  error_message,
  usage_estimated,
  usage_source,
  created_at
FROM ai_usage_logs
WHERE api_key_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountUsageLogsByAPIKey :one
SELECT COUNT(*) AS count
FROM ai_usage_logs
WHERE api_key_id = $1;

-- name: ListUsageLogsByUser :many
SELECT
  id,
  request_id,
  trace_id,
  api_key_id,
  key_owner_type,
  tenant_id,
  user_id,
  external_user_id,
  model_id,
  model_code,
  model_route_id,
  upstream_deployment_id,
  endpoint_id,
  provider_code,
  upstream_model,
  conversation_id,
  stream,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  billable_unit_type,
  billable_units,
  provider_cost,
  platform_cost,
  user_cost,
  api_key_quota_cost,
  urm_transaction_id,
  billing_status,
  request_status,
  http_status,
  upstream_status,
  latency_ms,
  first_token_latency_ms,
  error_code,
  error_message,
  usage_estimated,
  usage_source,
  created_at
FROM ai_usage_logs
WHERE tenant_id = $1
  AND user_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountUsageLogsByUser :one
SELECT COUNT(*) AS count
FROM ai_usage_logs
WHERE tenant_id = $1
  AND user_id = $2;

-- ============================================================================
-- Limit Policies CRUD
-- ============================================================================

-- name: CreateLimitPolicy :one
INSERT INTO ai_runtime_limit_policies (
  scope_type,
  scope_id,
  capability_type,
  model_code,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING
  id,
  scope_type,
  scope_id,
  capability_type,
  model_code,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at;

-- name: ListLimitPolicies :many
SELECT
  id,
  scope_type,
  scope_id,
  capability_type,
  model_code,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at
FROM ai_runtime_limit_policies
ORDER BY scope_type ASC, scope_id ASC, capability_type ASC;

-- name: ListActiveRuntimeLimitPolicies :many
-- Get all active limit policies for a given capability_type and optional model_code,
-- filtering by scope_id at various scope levels (tenant, user, api_key, provider, endpoint)
SELECT
  id,
  scope_type,
  scope_id,
  capability_type,
  model_code,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at
FROM ai_runtime_limit_policies
WHERE status = 'active'
  AND capability_type = $1
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (
    (scope_type = 'tenant' AND scope_id = $2)
    OR (scope_type = 'user' AND scope_id = $3)
    OR (scope_type = 'api_key' AND scope_id = $4)
    OR (scope_type = 'provider' AND scope_id = $5)
    OR (scope_type = 'endpoint' AND scope_id = $6)
  )
ORDER BY scope_type ASC;

-- name: UpdateLimitPolicy :one
UPDATE ai_runtime_limit_policies
SET scope_type = $2,
    scope_id = $3,
    capability_type = $4,
    model_code = $5,
    rpm_limit = $6,
    tpm_limit = $7,
    concurrency_limit = $8,
    status = $9,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  scope_type,
  scope_id,
  capability_type,
  model_code,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at;

-- name: UpdateLimitPolicyStatus :one
UPDATE ai_runtime_limit_policies
SET status = $2,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  scope_type,
  scope_id,
  capability_type,
  model_code,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at;

-- ============================================================================
-- Conversation Bindings
-- ============================================================================

-- name: GetConversationBinding :one
SELECT
  conversation_id,
  tenant_id,
  identity_id,
  model_id,
  upstream_deployment_id,
  endpoint_id,
  expires_at,
  created_at,
  updated_at
FROM ai_conversation_bindings
WHERE conversation_id = $1
  AND tenant_id = $2
  AND identity_id = $3
  AND model_id = $4;

-- name: CreateConversationBinding :one
INSERT INTO ai_conversation_bindings (
  conversation_id,
  tenant_id,
  identity_id,
  model_id,
  upstream_deployment_id,
  endpoint_id,
  expires_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING
  conversation_id,
  tenant_id,
  identity_id,
  model_id,
  upstream_deployment_id,
  endpoint_id,
  expires_at,
  created_at,
  updated_at;

-- ============================================================================
-- Audit Logs
-- ============================================================================

-- name: CreateAuditLog :one
INSERT INTO ai_admin_audit_logs (
  actor,
  action,
  object_type,
  object_id,
  request_summary,
  result,
  http_status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING
  id,
  actor,
  action,
  object_type,
  object_id,
  request_summary,
  result,
  http_status,
  created_at;

-- ============================================================================
-- Usage Summary Queries
-- ============================================================================

-- name: ListUsageSummary :many
SELECT
  model_code,
  COALESCE(SUM(request_count), 0)::bigint AS request_count,
  COALESCE(SUM(prompt_tokens), 0)::bigint AS total_prompt_tokens,
  COALESCE(SUM(completion_tokens), 0)::bigint AS total_completion_tokens,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(provider_cost), 0)::bigint AS total_provider_cost,
  COALESCE(SUM(platform_cost), 0)::bigint AS total_platform_cost,
  COALESCE(SUM(user_cost), 0)::bigint AS total_user_cost,
  COALESCE(SUM(api_key_quota_cost), 0)::bigint AS total_quota_cost
FROM ai_usage_rollups_hourly
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = COALESCE(sqlc.narg('user_id')::text, ''))
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status'))
  AND (sqlc.narg('since')::timestamptz IS NULL OR bucket_start >= date_trunc('hour', sqlc.narg('since')::timestamptz))
GROUP BY model_code
ORDER BY request_count DESC;

-- name: ListUsageUnitSummary :many
SELECT
  billable_unit_type,
  COALESCE(SUM(request_count), 0)::bigint AS request_count,
  COALESCE(SUM(billable_units), 0)::bigint AS total_billable_units,
  COALESCE(SUM(provider_cost), 0)::bigint AS total_provider_cost,
  COALESCE(SUM(platform_cost), 0)::bigint AS total_platform_cost,
  COALESCE(SUM(user_cost), 0)::bigint AS total_user_cost
FROM ai_usage_rollups_hourly
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = COALESCE(sqlc.narg('user_id')::text, ''))
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status'))
  AND (sqlc.narg('since')::timestamptz IS NULL OR bucket_start >= date_trunc('hour', sqlc.narg('since')::timestamptz))
GROUP BY billable_unit_type
ORDER BY request_count DESC;

-- ============================================================================
-- Dashboard Queries
-- ============================================================================

-- name: GetDashboardSummary :one
SELECT
  COALESCE(SUM(request_count), 0)::bigint AS total_requests,
  COALESCE(SUM(success_count), 0)::bigint AS successful_requests,
  COALESCE(SUM(failed_count), 0)::bigint AS failed_requests,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(prompt_tokens), 0)::bigint AS total_prompt_tokens,
  COALESCE(SUM(completion_tokens), 0)::bigint AS total_completion_tokens,
  COALESCE(SUM(provider_cost), 0)::bigint AS total_provider_cost,
  COALESCE(SUM(platform_cost), 0)::bigint AS total_platform_cost,
  COALESCE(SUM(user_cost), 0)::bigint AS total_user_cost,
  COALESCE(SUM(latency_success_sum_ms)::double precision / NULLIF(SUM(latency_success_count), 0), 0)::double precision AS avg_latency_ms
FROM ai_usage_rollups_hourly
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = COALESCE(sqlc.narg('user_id')::text, ''))
  AND (sqlc.narg('since')::timestamptz IS NULL OR bucket_start >= date_trunc('hour', sqlc.narg('since')::timestamptz));

-- name: ListDashboardTopModels :many
SELECT
  model_code,
  COALESCE(SUM(request_count), 0)::bigint AS request_count,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(platform_cost), 0)::bigint AS total_cost
FROM ai_usage_rollups_hourly
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = COALESCE(sqlc.narg('user_id')::text, ''))
  AND (sqlc.narg('since')::timestamptz IS NULL OR bucket_start >= date_trunc('hour', sqlc.narg('since')::timestamptz))
GROUP BY model_code
ORDER BY request_count DESC
LIMIT sqlc.arg('limit');

-- name: ListDashboardTopTenants :many
SELECT
  tenant_id,
  COALESCE(SUM(request_count), 0)::bigint AS request_count,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(platform_cost), 0)::bigint AS total_cost
FROM ai_usage_rollups_hourly
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = COALESCE(sqlc.narg('user_id')::text, ''))
  AND (sqlc.narg('since')::timestamptz IS NULL OR bucket_start >= date_trunc('hour', sqlc.narg('since')::timestamptz))
GROUP BY tenant_id
ORDER BY request_count DESC
LIMIT sqlc.arg('limit');

-- name: ListDashboardRecentErrors :many
SELECT
  request_id,
  model_code,
  request_status,
  error_code,
  error_message,
  http_status,
  created_at
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('since')::timestamptz IS NULL OR created_at >= sqlc.narg('since'))
  AND request_status = 'failed'
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

-- ============================================================================
-- List Audit Logs
-- ============================================================================

-- name: ListAuditLogs :many
SELECT
  id,
  actor,
  action,
  object_type,
  object_id,
  request_summary,
  result,
  http_status,
  created_at
FROM ai_admin_audit_logs
ORDER BY created_at DESC
LIMIT $1;

-- ============================================================================
-- API Queries for /api/v1/* routes (role-based filtering)
-- ============================================================================

-- name: ListUserAvailableModels :many
-- 用户可用的模型 = 租户授权的模型 + 三级定价 fallback（用户售价 → 租户折扣价 → 平台公价）
SELECT
  m.id,
  m.model_code,
  m.capability_type,
  m.context_window,
  m.default_max_output_tokens,
  m.max_output_tokens,
  m.status,
  tg.status AS grant_status,
  tg.created_at AS granted_at,
  COALESCE(up.input_price_per_1m, tp.input_price_per_1m, bp.input_price_per_1m, 0) AS input_price_per_1m,
  COALESCE(up.output_price_per_1m, tp.output_price_per_1m, bp.output_price_per_1m, 0) AS output_price_per_1m,
  COALESCE(up.image_prices, tp.image_prices, bp.image_prices) AS image_prices,
  COALESCE(up.video_prices, tp.video_prices, bp.video_prices) AS video_prices,
  COALESCE(up.audio_tts_price_per_1m_chars, tp.audio_tts_price_per_1m_chars, bp.audio_tts_price_per_1m_chars, 0) AS audio_tts_price_per_1m_chars,
  COALESCE(up.audio_stt_price_per_minute, tp.audio_stt_price_per_minute, bp.audio_stt_price_per_minute, 0) AS audio_stt_price_per_minute
FROM ai_models m
JOIN ai_tenant_model_grants tg ON tg.model_id = m.id
LEFT JOIN ai_tenant_user_prices up ON up.tenant_id = tg.tenant_id AND up.model_id = m.id
LEFT JOIN ai_tenant_model_price_overrides tp ON tp.tenant_id = tg.tenant_id AND tp.model_id = m.id
LEFT JOIN ai_model_prices bp ON bp.model_id = m.id
WHERE tg.tenant_id = $1
  AND tg.status = 'active'
  AND m.status = 'active'
ORDER BY m.model_code ASC;

-- name: ListUsageLogsByTenantUser :many
SELECT
  id,
  request_id,
  trace_id,
  tenant_id,
  user_id,
  model_id,
  model_code,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  billable_unit_type,
  billable_units,
  user_cost,
  request_status,
  http_status,
  latency_ms,
  error_code,
  error_message,
  created_at
FROM ai_usage_logs
WHERE tenant_id = $1
  AND user_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: CountUsageLogsByTenantUser :one
SELECT COUNT(*) AS count
FROM ai_usage_logs
WHERE tenant_id = $1
  AND user_id = $2;

-- name: ListUsageSummaryByTenantUser :one
SELECT
  COALESCE(SUM(request_count), 0)::bigint AS request_count,
  COALESCE(SUM(success_count), 0)::bigint AS success_requests,
  COALESCE(SUM(failed_count), 0)::bigint AS failed_requests,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(prompt_tokens), 0)::bigint AS total_prompt_tokens,
  COALESCE(SUM(completion_tokens), 0)::bigint AS total_completion_tokens,
  COALESCE(SUM(user_cost), 0)::bigint AS total_user_cost,
  COALESCE(SUM(latency_success_sum_ms)::double precision / NULLIF(SUM(latency_success_count), 0), 0)::double precision AS avg_latency_ms
FROM ai_usage_rollups_hourly
WHERE tenant_id = $1
  AND user_id = $2;


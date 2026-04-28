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
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING
  id,
  provider_id,
  name,
  base_url,
  extra_headers,
  weight,
  timeout_ms,
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
    status = $9,
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
  status,
  created_at,
  updated_at;

-- ============================================================================
-- Upstream Deployment CRUD
-- ============================================================================

-- name: CreateUpstreamDeployment :one
INSERT INTO ai_upstream_deployments (
  endpoint_id,
  name,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  tags,
  health_status,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, 'unknown', $9
)
RETURNING
  id,
  endpoint_id,
  name,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  tags,
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
  ud.name,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.request_path,
  ud.upstream_parameters,
  ud.tags,
  ud.health_status,
  ud.last_health_check_at,
  ud.last_health_error,
  ud.status,
  ud.created_at,
  ud.updated_at,
  e.name AS endpoint_name,
  e.base_url,
  e.weight AS endpoint_weight,
  p.id AS provider_id,
  p.code AS provider_code,
  p.name AS provider_name
FROM ai_upstream_deployments ud
JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
JOIN ai_providers p ON p.id = e.provider_id
WHERE ud.endpoint_id = $1
ORDER BY ud.name ASC;

-- name: GetUpstreamDeployment :one
SELECT
  ud.id,
  ud.endpoint_id,
  ud.name,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.request_path,
  ud.upstream_parameters,
  ud.tags,
  ud.health_status,
  ud.last_health_check_at,
  ud.last_health_error,
  ud.status,
  ud.created_at,
  ud.updated_at,
  e.name AS endpoint_name,
  e.base_url,
  e.api_key_ciphertext,
  e.extra_headers,
  e.timeout_ms,
  p.id AS provider_id,
  p.code AS provider_code,
  p.name AS provider_name
FROM ai_upstream_deployments ud
JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
JOIN ai_providers p ON p.id = e.provider_id
WHERE ud.id = $1;

-- name: UpdateUpstreamDeploymentStatus :one
UPDATE ai_upstream_deployments
SET status = $2,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  endpoint_id,
  name,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  tags,
  health_status,
  last_health_check_at,
  last_health_error,
  status,
  created_at,
  updated_at;

-- name: UpdateUpstreamDeployment :one
UPDATE ai_upstream_deployments
SET name = $2,
    upstream_model = $3,
    capability_type = $4,
    upstream_protocol = $5,
    request_path = $6,
    upstream_parameters = $7,
    tags = $8,
    status = $9,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  endpoint_id,
  name,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  tags,
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
  name,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  tags,
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
  ud.name,
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
  p.code AS provider_code,
  p.name AS provider_name
FROM ai_upstream_deployments ud
JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
JOIN ai_providers p ON p.id = e.provider_id
WHERE ud.id = $1;

-- ============================================================================
-- Upstream Deployment Cost Price CRUD
-- ============================================================================

-- name: CreateUpstreamDeploymentCostPrice :one
INSERT INTO ai_upstream_deployment_cost_prices (
  upstream_deployment_id,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  image_size_prices,
  video_cost_per_second,
  effective_from,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING
  id,
  upstream_deployment_id,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  image_size_prices,
  video_cost_per_second,
  effective_from,
  status,
  created_at;

-- name: ListUpstreamDeploymentCostPrices :many
SELECT
  id,
  upstream_deployment_id,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  image_size_prices,
  video_cost_per_second,
  effective_from,
  status,
  created_at
FROM ai_upstream_deployment_cost_prices
WHERE upstream_deployment_id = $1
ORDER BY effective_from DESC;

-- name: UpdateUpstreamDeploymentCostPriceStatus :one
UPDATE ai_upstream_deployment_cost_prices
SET status = $3
WHERE upstream_deployment_id = $1
  AND id = $2
RETURNING
  id,
  upstream_deployment_id,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  image_size_prices,
  video_cost_per_second,
  effective_from,
  status,
  created_at;

-- name: UpdateUpstreamDeploymentCostPrice :one
UPDATE ai_upstream_deployment_cost_prices
SET capability_type = $3,
    currency = $4,
    input_cost_per_1m = $5,
    output_cost_per_1m = $6,
    request_cost = $7,
    image_cost = $8,
    image_size_prices = $9,
    video_cost_per_second = $10,
    effective_from = $11,
    status = $12
WHERE upstream_deployment_id = $1
  AND id = $2
RETURNING
  id,
  upstream_deployment_id,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  image_size_prices,
  video_cost_per_second,
  effective_from,
  status,
  created_at;

-- ============================================================================
-- Model CRUD
-- ============================================================================

-- name: CreateModel :one
INSERT INTO ai_models (
  model_code,
  display_name,
  capability_type,
  context_window,
  default_max_output_tokens,
  max_output_tokens,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING
  id,
  model_code,
  display_name,
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
  display_name,
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
  display_name,
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
  display_name,
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
    display_name = $3,
    capability_type = $4,
    context_window = $5,
    default_max_output_tokens = $6,
    max_output_tokens = $7,
    status = $8,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  model_code,
  display_name,
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
  image_size_prices,
  video_price_per_second,
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
  image_size_prices,
  video_price_per_second,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (model_id) DO UPDATE SET
  input_price_per_1m           = EXCLUDED.input_price_per_1m,
  output_price_per_1m          = EXCLUDED.output_price_per_1m,
  image_size_prices            = EXCLUDED.image_size_prices,
  video_price_per_second       = EXCLUDED.video_price_per_second,
  audio_tts_price_per_1m_chars = EXCLUDED.audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute   = EXCLUDED.audio_stt_price_per_minute,
  updated_at                   = now()
RETURNING
  id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_size_prices,
  video_price_per_second,
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
RETURNING
  id,
  model_id,
  upstream_deployment_id,
  priority,
  weight,
  supports_stream,
  status,
  created_at,
  updated_at;

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
  ud.name AS upstream_deployment_name,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.health_status,
  e.id AS endpoint_id,
  e.name AS endpoint_name,
  e.base_url,
  p.id AS provider_id,
  p.code AS provider_code,
  p.name AS provider_name
FROM ai_model_routes r
JOIN ai_upstream_deployments ud ON ud.id = r.upstream_deployment_id
JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
JOIN ai_providers p ON p.id = e.provider_id
WHERE r.model_id = $1
ORDER BY r.priority ASC, r.weight DESC, p.code ASC, e.name ASC;

-- name: GetModelRoute :one
SELECT
  r.id,
  r.model_id,
  r.upstream_deployment_id,
  r.priority,
  r.weight,
  r.supports_stream,
  r.status,
  r.created_at,
  r.updated_at
FROM ai_model_routes r
WHERE r.id = $1;

-- name: UpdateModelRouteStatus :one
UPDATE ai_model_routes
SET status = $3,
    updated_at = now()
WHERE model_id = $1
  AND id = $2
RETURNING
  id,
  model_id,
  upstream_deployment_id,
  priority,
  weight,
  supports_stream,
  status,
  created_at,
  updated_at;

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
RETURNING
  id,
  model_id,
  upstream_deployment_id,
  priority,
  weight,
  supports_stream,
  status,
  created_at,
  updated_at;

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
  m.display_name,
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
  image_size_prices,
  video_price_per_second,
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
  image_size_prices,
  video_price_per_second,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute,
  created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (tenant_id, model_id) DO UPDATE SET
  input_price_per_1m           = EXCLUDED.input_price_per_1m,
  output_price_per_1m          = EXCLUDED.output_price_per_1m,
  image_size_prices            = EXCLUDED.image_size_prices,
  video_price_per_second       = EXCLUDED.video_price_per_second,
  audio_tts_price_per_1m_chars = EXCLUDED.audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute   = EXCLUDED.audio_stt_price_per_minute,
  updated_at                   = now()
RETURNING
  id,
  tenant_id,
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_size_prices,
  video_price_per_second,
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
  o.image_size_prices,
  o.video_price_per_second,
  o.audio_tts_price_per_1m_chars,
  o.audio_stt_price_per_minute,
  o.created_by,
  o.created_at,
  o.updated_at,
  m.model_code,
  m.display_name,
  m.capability_type
FROM ai_tenant_model_price_overrides o
JOIN ai_models m ON m.id = o.model_id
WHERE o.tenant_id = $1
ORDER BY m.model_code ASC;

-- ============================================================================
-- API Keys CRUD (Tenant)
-- ============================================================================

-- name: CreateTenantAPIKey :one
INSERT INTO ai_api_keys (
  owner_type,
  tenant_id,
  user_id,
  key_hash,
  key_prefix,
  name,
  quota_limit,
  allowed_models,
  status,
  expires_at,
  created_by
) VALUES (
  'tenant',
  $1, NULL, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING
  id,
  owner_type,
  tenant_id,
  user_id,
  key_prefix,
  name,
  quota_limit,
  quota_used,
  quota_reserved,
  allowed_models,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at;

-- name: ListTenantAPIKeys :many
SELECT
  id,
  owner_type,
  tenant_id,
  user_id,
  key_prefix,
  name,
  quota_limit,
  quota_used,
  quota_reserved,
  allowed_models,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at
FROM ai_api_keys
WHERE tenant_id = $1
  AND owner_type = 'tenant'
ORDER BY created_at DESC;

-- name: UpdateTenantAPIKeyStatus :one
UPDATE ai_api_keys
SET status = $3,
    updated_at = now()
WHERE tenant_id = $1
  AND id = $2
  AND owner_type = 'tenant'
RETURNING
  id,
  owner_type,
  tenant_id,
  user_id,
  key_prefix,
  name,
  quota_limit,
  quota_used,
  quota_reserved,
  allowed_models,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at;

-- name: UpdateTenantAPIKey :one
UPDATE ai_api_keys
SET name = $3,
    quota_limit = $4,
    allowed_models = $5,
    status = $6,
    expires_at = $7,
    updated_at = now()
WHERE tenant_id = $1
  AND id = $2
  AND owner_type = 'tenant'
RETURNING
  id,
  owner_type,
  tenant_id,
  user_id,
  key_prefix,
  name,
  quota_limit,
  quota_used,
  quota_reserved,
  allowed_models,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at;

-- ============================================================================
-- API Keys CRUD (User)
-- ============================================================================

-- name: CreateUserAPIKey :one
INSERT INTO ai_api_keys (
  owner_type,
  tenant_id,
  user_id,
  key_hash,
  key_prefix,
  name,
  quota_limit,
  allowed_models,
  status,
  expires_at,
  created_by
) VALUES (
  'user',
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING
  id,
  owner_type,
  tenant_id,
  user_id,
  key_prefix,
  name,
  quota_limit,
  quota_used,
  quota_reserved,
  allowed_models,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at;

-- name: ListUserAPIKeys :many
SELECT
  id,
  owner_type,
  tenant_id,
  user_id,
  key_prefix,
  name,
  quota_limit,
  quota_used,
  quota_reserved,
  allowed_models,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at
FROM ai_api_keys
WHERE tenant_id = $1
  AND user_id = $2
  AND owner_type = 'user'
ORDER BY created_at DESC;

-- name: UpdateUserAPIKeyStatus :one
UPDATE ai_api_keys
SET status = $4,
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND id = $3
  AND owner_type = 'user'
RETURNING
  id,
  owner_type,
  tenant_id,
  user_id,
  key_prefix,
  name,
  quota_limit,
  quota_used,
  quota_reserved,
  allowed_models,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at;

-- name: UpdateUserAPIKey :one
UPDATE ai_api_keys
SET name = $4,
    quota_limit = $5,
    allowed_models = $6,
    status = $7,
    expires_at = $8,
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND id = $3
  AND owner_type = 'user'
RETURNING
  id,
  owner_type,
  tenant_id,
  user_id,
  key_prefix,
  name,
  quota_limit,
  quota_used,
  quota_reserved,
  allowed_models,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at;

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
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountUsageLogs :one
SELECT COUNT(*) AS count
FROM ai_usage_logs
WHERE tenant_id = $1;

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
  COUNT(*) AS request_count,
  SUM(prompt_tokens) AS total_prompt_tokens,
  SUM(completion_tokens) AS total_completion_tokens,
  SUM(total_tokens) AS total_tokens,
  SUM(provider_cost) AS total_provider_cost,
  SUM(platform_cost) AS total_platform_cost,
  SUM(user_cost) AS total_user_cost,
  SUM(api_key_quota_cost) AS total_quota_cost
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status'))
GROUP BY model_code
ORDER BY request_count DESC;

-- name: ListUsageUnitSummary :many
SELECT
  billable_unit_type,
  COUNT(*) AS request_count,
  SUM(billable_units) AS total_billable_units,
  SUM(provider_cost) AS total_provider_cost,
  SUM(platform_cost) AS total_platform_cost,
  SUM(user_cost) AS total_user_cost
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status'))
GROUP BY billable_unit_type
ORDER BY request_count DESC;

-- ============================================================================
-- Dashboard Queries
-- ============================================================================

-- name: GetDashboardSummary :one
SELECT
  COUNT(*) AS total_requests,
  COUNT(*) FILTER (WHERE request_status = 'success') AS successful_requests,
  COUNT(*) FILTER (WHERE request_status = 'failed') AS failed_requests,
  SUM(total_tokens) AS total_tokens,
  SUM(prompt_tokens) AS total_prompt_tokens,
  SUM(completion_tokens) AS total_completion_tokens,
  SUM(provider_cost) AS total_provider_cost,
  SUM(platform_cost) AS total_platform_cost,
  SUM(user_cost) AS total_user_cost,
  AVG(latency_ms) FILTER (WHERE request_status = 'success') AS avg_latency_ms
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('since')::timestamptz IS NULL OR created_at >= sqlc.narg('since'));

-- name: ListDashboardTopModels :many
SELECT
  model_code,
  COUNT(*) AS request_count,
  SUM(total_tokens) AS total_tokens,
  SUM(platform_cost) AS total_cost
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('since')::timestamptz IS NULL OR created_at >= sqlc.narg('since'))
GROUP BY model_code
ORDER BY request_count DESC
LIMIT sqlc.arg('limit');

-- name: ListDashboardTopTenants :many
SELECT
  tenant_id,
  COUNT(*) AS request_count,
  SUM(total_tokens) AS total_tokens,
  SUM(platform_cost) AS total_cost
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('since')::timestamptz IS NULL OR created_at >= sqlc.narg('since'))
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
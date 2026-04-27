-- name: CreateProvider :one
INSERT INTO ai_providers (
  code,
  name,
  provider_type,
  protocol_type,
  is_custom,
  config,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING
  id,
  code,
  name,
  provider_type,
  protocol_type,
  is_custom,
  config,
  status,
  created_at,
  updated_at;

-- name: ListProviders :many
SELECT
  id,
  code,
  name,
  provider_type,
  protocol_type,
  is_custom,
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
  provider_type,
  protocol_type,
  is_custom,
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
  provider_type,
  protocol_type,
  is_custom,
  config,
  status,
  created_at,
  updated_at;

-- name: CreateProviderEndpoint :one
INSERT INTO ai_provider_endpoints (
  provider_id,
  name,
  base_url,
  protocol_type,
  api_key_ciphertext,
  extra_headers,
  custom_path,
  protocol_overrides,
  weight,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  timeout_ms,
  status,
  health_status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'unknown'
)
RETURNING
  id,
  provider_id,
  name,
  base_url,
  protocol_type,
  extra_headers,
  custom_path,
  protocol_overrides,
  weight,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  timeout_ms,
  status,
  health_status,
  created_at,
  updated_at;

-- name: ListProviderEndpoints :many
SELECT
  id,
  provider_id,
  name,
  base_url,
  protocol_type,
  extra_headers,
  custom_path,
  protocol_overrides,
  weight,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  timeout_ms,
  status,
  health_status,
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
  protocol_type,
  api_key_ciphertext,
  extra_headers,
  custom_path,
  protocol_overrides,
  weight,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  timeout_ms,
  status,
  health_status,
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
  protocol_type,
  extra_headers,
  custom_path,
  protocol_overrides,
  weight,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  timeout_ms,
  status,
  health_status,
  created_at,
  updated_at;

-- name: UpdateProviderEndpointHealth :one
UPDATE ai_provider_endpoints
SET health_status = $3,
    last_health_check_at = now(),
    updated_at = now()
WHERE provider_id = $1
  AND id = $2
RETURNING
  id,
  provider_id,
  name,
  base_url,
  protocol_type,
  extra_headers,
  custom_path,
  protocol_overrides,
  weight,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  timeout_ms,
  status,
  health_status,
  created_at,
  updated_at;

-- name: GetFirstActiveChatDeploymentForEndpoint :one
SELECT
  d.id,
  d.upstream_model,
  d.upstream_parameters,
  d.upstream_protocol,
  d.capability_type
FROM ai_model_deployments d
WHERE d.endpoint_id = $1
  AND d.capability_type = 'chat'
  AND d.status = 'active'
ORDER BY d.priority ASC, d.weight DESC, d.created_at ASC
LIMIT 1;

-- name: CreateProviderModelPrice :one
INSERT INTO ai_provider_model_prices (
  provider_id,
  endpoint_id,
  upstream_model,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  video_cost_per_second,
  effective_from,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING
  id,
  provider_id,
  endpoint_id,
  upstream_model,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  video_cost_per_second,
  effective_from,
  status,
  created_at;

-- name: ListProviderModelPrices :many
SELECT
  id,
  provider_id,
  endpoint_id,
  upstream_model,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  video_cost_per_second,
  effective_from,
  status,
  created_at
FROM ai_provider_model_prices
WHERE provider_id = $1
ORDER BY upstream_model ASC, capability_type ASC, effective_from DESC;

-- name: UpdateProviderModelPriceStatus :one
UPDATE ai_provider_model_prices
SET status = $3
WHERE provider_id = $1
  AND id = $2
RETURNING
  id,
  provider_id,
  endpoint_id,
  upstream_model,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  video_cost_per_second,
  effective_from,
  status,
  created_at;

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

-- name: CreateModelPrice :one
INSERT INTO ai_model_prices (
  model_id,
  platform_input_price_per_1m,
  platform_output_price_per_1m,
  platform_image_price,
  tenant_input_price_per_1m,
  tenant_output_price_per_1m,
  tenant_image_price,
  effective_from,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING
  id,
  model_id,
  platform_input_price_per_1m,
  platform_output_price_per_1m,
  platform_image_price,
  tenant_input_price_per_1m,
  tenant_output_price_per_1m,
  tenant_image_price,
  effective_from,
  status,
  created_at;

-- name: ListModelPrices :many
SELECT
  id,
  model_id,
  platform_input_price_per_1m,
  platform_output_price_per_1m,
  platform_image_price,
  tenant_input_price_per_1m,
  tenant_output_price_per_1m,
  tenant_image_price,
  effective_from,
  status,
  created_at
FROM ai_model_prices
WHERE model_id = $1
ORDER BY effective_from DESC;

-- name: UpdateModelPriceStatus :one
UPDATE ai_model_prices
SET status = $3
WHERE model_id = $1
  AND id = $2
RETURNING
  id,
  model_id,
  platform_input_price_per_1m,
  platform_output_price_per_1m,
  platform_image_price,
  tenant_input_price_per_1m,
  tenant_output_price_per_1m,
  tenant_image_price,
  effective_from,
  status,
  created_at;

-- name: CreateModelDeployment :one
INSERT INTO ai_model_deployments (
  model_id,
  endpoint_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  upstream_parameters,
  priority,
  weight,
  supports_stream,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING
  id,
  model_id,
  endpoint_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  upstream_parameters,
  priority,
  weight,
  supports_stream,
  status,
  created_at,
  updated_at;

-- name: ListModelDeployments :many
SELECT
  d.id,
  d.model_id,
  d.endpoint_id,
  d.upstream_model,
  d.capability_type,
  d.upstream_protocol,
  d.upstream_parameters,
  d.priority,
  d.weight,
  d.supports_stream,
  d.status,
  d.created_at,
  d.updated_at,
  e.name AS endpoint_name,
  e.base_url,
  p.code AS provider_code,
  p.name AS provider_name
FROM ai_model_deployments d
JOIN ai_provider_endpoints e ON e.id = d.endpoint_id
JOIN ai_providers p ON p.id = e.provider_id
WHERE d.model_id = $1
ORDER BY d.priority ASC, d.weight DESC, p.code ASC, e.name ASC;

-- name: UpdateModelDeploymentStatus :one
UPDATE ai_model_deployments
SET status = $3,
    updated_at = now()
WHERE model_id = $1
  AND id = $2
RETURNING
  id,
  model_id,
  endpoint_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  upstream_parameters,
  priority,
  weight,
  supports_stream,
  status,
  created_at,
  updated_at;

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

-- name: GrantModelToUser :one
INSERT INTO ai_user_model_grants (
  tenant_id,
  user_id,
  model_id,
  status,
  created_by
) VALUES (
  $1, $2, $3, $4, $5
)
ON CONFLICT (user_id, model_id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  status = EXCLUDED.status,
  created_by = EXCLUDED.created_by
RETURNING
  id,
  tenant_id,
  user_id,
  model_id,
  status,
  created_by,
  created_at;

-- name: ListUserModelGrants :many
SELECT
  ug.id,
  ug.tenant_id,
  ug.user_id,
  ug.model_id,
  ug.status,
  ug.created_by,
  ug.created_at,
  m.model_code,
  m.display_name,
  m.capability_type
FROM ai_user_model_grants ug
JOIN ai_models m ON m.id = ug.model_id
WHERE ug.tenant_id = $1
  AND ug.user_id = $2
ORDER BY m.model_code ASC;

-- name: UpdateUserModelGrantStatus :one
UPDATE ai_user_model_grants
SET status = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND model_id = $3
RETURNING
  id,
  tenant_id,
  user_id,
  model_id,
  status,
  created_by,
  created_at;

-- name: CreateTenantAPIKey :one
INSERT INTO ai_api_keys (
  owner_type,
  tenant_id,
  user_id,
  key_hash,
  key_prefix,
  name,
  quota_limit,
  daily_quota,
  monthly_quota,
  allowed_models,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  expires_at,
  created_by
) VALUES (
  'tenant',
  $1, NULL, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
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
  daily_quota,
  daily_used,
  monthly_quota,
  monthly_used,
  allowed_models,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
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
  daily_quota,
  daily_used,
  monthly_quota,
  monthly_used,
  allowed_models,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
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
  daily_quota,
  daily_used,
  monthly_quota,
  monthly_used,
  allowed_models,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at;

-- name: CreateUserAPIKey :one
INSERT INTO ai_api_keys (
  owner_type,
  tenant_id,
  user_id,
  key_hash,
  key_prefix,
  name,
  quota_limit,
  daily_quota,
  monthly_quota,
  allowed_models,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  expires_at,
  created_by
) VALUES (
  'user',
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
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
  daily_quota,
  daily_used,
  monthly_quota,
  monthly_used,
  allowed_models,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
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
  daily_quota,
  daily_used,
  monthly_quota,
  monthly_used,
  allowed_models,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
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
  daily_quota,
  daily_used,
  monthly_quota,
  monthly_used,
  allowed_models,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  expires_at,
  created_by,
  created_at,
  updated_at;

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
  model_code,
  capability_type,
  deployment_id,
  endpoint_id,
  provider_code,
  upstream_model,
  conversation_id,
  stream,
  prompt_tokens,
  completion_tokens,
  total_tokens,
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
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

-- name: ListRuntimeLimitPolicies :many
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
WHERE (sqlc.narg('scope_type')::text IS NULL OR scope_type = sqlc.narg('scope_type'))
  AND (sqlc.narg('scope_id')::text IS NULL OR scope_id = sqlc.narg('scope_id'))
  AND (sqlc.narg('capability_type')::text IS NULL OR capability_type = sqlc.narg('capability_type'))
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

-- name: ListActiveRuntimeLimitPolicies :many
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
  AND (model_code IS NULL OR model_code = $2)
  AND (
    (scope_type = 'tenant' AND scope_id = $3)
    OR (scope_type = 'user' AND scope_id = $4)
    OR (scope_type = 'api_key' AND scope_id = $5)
    OR (scope_type = 'provider' AND scope_id = $6)
    OR (scope_type = 'endpoint' AND scope_id = $7)
  )
ORDER BY scope_type ASC, model_code DESC, created_at ASC;

-- name: CreateRuntimeLimitPolicy :one
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

-- name: UpdateRuntimeLimitPolicy :one
UPDATE ai_runtime_limit_policies
SET
  scope_type = $2,
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

-- name: UpdateRuntimeLimitPolicyStatus :one
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

-- name: CreateAdminAuditLog :one
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
RETURNING id;

-- name: ListAdminAuditLogs :many
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
WHERE (sqlc.narg('actor')::text IS NULL OR actor = sqlc.narg('actor'))
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('object_type')::text IS NULL OR object_type = sqlc.narg('object_type'))
  AND (sqlc.narg('object_id')::text IS NULL OR object_id = sqlc.narg('object_id'))
  AND (sqlc.narg('result')::text IS NULL OR result = sqlc.narg('result'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

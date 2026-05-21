-- name: CreateUsageLog :one
INSERT INTO ai_usage_logs (
  request_id,
  trace_id,
  api_key_id,
  key_owner_type,
  auth_method,
  request_source,
  tenant_id,
  user_id,
  external_user_id,
  model_id,
  model_code,
  capability_type,
  model_route_id,
  upstream_deployment_id,
  endpoint_id,
  credential_pool_id,
  oauth_credential_id,
  provider_code,
  upstream_model,
  provider_format,
  conversation_id,
  stream,
  prompt_tokens,
  completion_tokens,
  cache_write_tokens,
  cache_read_tokens,
  reasoning_tokens,
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
  token_usage_source,
  attempts_count,
  final_route_id,
  client_protocol,
  resolution
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
  $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
  $31, $32, $33, $34, $35, $36, $37, $38, $39, $40,
  $41, $42, $43, $44, $45, $46, $47, $48, $49
)
RETURNING id;

-- name: UpsertUsageRollupHourly :exec
INSERT INTO ai_usage_rollups_hourly (
  bucket_start,
  tenant_id,
  user_id,
  api_key_id,
  request_source,
  capability_type,
  model_code,
  provider_code,
  request_status,
  billable_unit_type,
  request_count,
  success_count,
  failed_count,
  prompt_tokens,
  completion_tokens,
  cache_write_tokens,
  cache_read_tokens,
  reasoning_tokens,
  total_tokens,
  billable_units,
  provider_cost,
  platform_cost,
  user_cost,
  api_key_quota_cost,
  latency_success_sum_ms,
  latency_success_count
) VALUES (
  date_trunc('hour', now()),
  sqlc.arg('tenant_id'),
  COALESCE(sqlc.narg('user_id')::text, ''),
  sqlc.arg('api_key_id')::uuid,
  sqlc.arg('request_source'),
  sqlc.arg('capability_type'),
  sqlc.arg('model_code'),
  COALESCE(sqlc.narg('provider_code')::text, ''),
  sqlc.arg('request_status'),
  sqlc.arg('billable_unit_type'),
  1,
  CASE WHEN sqlc.arg('request_status') = 'success' THEN 1 ELSE 0 END,
  CASE WHEN sqlc.arg('request_status') = 'failed'  THEN 1 ELSE 0 END,
  sqlc.arg('prompt_tokens')::bigint,
  sqlc.arg('completion_tokens')::bigint,
  sqlc.arg('cache_write_tokens')::bigint,
  sqlc.arg('cache_read_tokens')::bigint,
  sqlc.arg('reasoning_tokens')::bigint,
  sqlc.arg('total_tokens')::bigint,
  sqlc.arg('billable_units')::bigint,
  sqlc.arg('provider_cost')::bigint,
  sqlc.arg('platform_cost')::bigint,
  sqlc.arg('user_cost')::bigint,
  sqlc.arg('api_key_quota_cost')::bigint,
  CASE
    WHEN sqlc.arg('request_status') = 'success' AND sqlc.narg('latency_ms')::integer IS NOT NULL
      THEN sqlc.narg('latency_ms')::bigint
    ELSE 0
  END,
  CASE
    WHEN sqlc.arg('request_status') = 'success' AND sqlc.narg('latency_ms')::integer IS NOT NULL
      THEN 1
    ELSE 0
  END
)
ON CONFLICT (
  bucket_start, tenant_id, user_id, api_key_id, request_source,
  model_code, provider_code, request_status, billable_unit_type
) DO UPDATE SET
  request_count           = ai_usage_rollups_hourly.request_count           + EXCLUDED.request_count,
  success_count           = ai_usage_rollups_hourly.success_count           + EXCLUDED.success_count,
  failed_count            = ai_usage_rollups_hourly.failed_count            + EXCLUDED.failed_count,
  prompt_tokens           = ai_usage_rollups_hourly.prompt_tokens           + EXCLUDED.prompt_tokens,
  completion_tokens       = ai_usage_rollups_hourly.completion_tokens       + EXCLUDED.completion_tokens,
  cache_write_tokens      = ai_usage_rollups_hourly.cache_write_tokens      + EXCLUDED.cache_write_tokens,
  cache_read_tokens       = ai_usage_rollups_hourly.cache_read_tokens       + EXCLUDED.cache_read_tokens,
  reasoning_tokens        = ai_usage_rollups_hourly.reasoning_tokens        + EXCLUDED.reasoning_tokens,
  total_tokens            = ai_usage_rollups_hourly.total_tokens            + EXCLUDED.total_tokens,
  billable_units          = ai_usage_rollups_hourly.billable_units          + EXCLUDED.billable_units,
  provider_cost           = ai_usage_rollups_hourly.provider_cost           + EXCLUDED.provider_cost,
  platform_cost           = ai_usage_rollups_hourly.platform_cost           + EXCLUDED.platform_cost,
  user_cost               = ai_usage_rollups_hourly.user_cost               + EXCLUDED.user_cost,
  api_key_quota_cost      = ai_usage_rollups_hourly.api_key_quota_cost      + EXCLUDED.api_key_quota_cost,
  latency_success_sum_ms  = ai_usage_rollups_hourly.latency_success_sum_ms  + EXCLUDED.latency_success_sum_ms,
  latency_success_count   = ai_usage_rollups_hourly.latency_success_count   + EXCLUDED.latency_success_count,
  updated_at = now();

-- name: GetActiveModelPrice :one
SELECT
  input_price_per_1m,
  output_price_per_1m,
  cache_write_price_per_1m,
  cache_read_price_per_1m,
  reasoning_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute
FROM ai_model_prices
WHERE model_id = $1;

-- name: GetTenantModelPriceOverrideForRuntime :one
SELECT
  input_price_per_1m,
  output_price_per_1m,
  cache_write_price_per_1m,
  cache_read_price_per_1m,
  reasoning_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute
FROM ai_tenant_model_price_overrides
WHERE tenant_id = $1
  AND model_id = $2;

-- name: GetTenantUserPriceForRuntime :one
SELECT
  input_price_per_1m,
  output_price_per_1m,
  cache_write_price_per_1m,
  cache_read_price_per_1m,
  reasoning_price_per_1m,
  image_prices,
  video_prices,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute
FROM ai_tenant_user_prices
WHERE tenant_id = $1
  AND model_id = $2;

-- name: ConfirmAPIKeyQuotaUsage :exec
UPDATE ai_api_keys
SET
  quota_used     = quota_used + $2,
  quota_reserved = GREATEST(0, quota_reserved - $3),
  updated_at     = now()
WHERE id = $1;

-- name: ReserveAPIKeyQuota :execrows
UPDATE ai_api_keys
SET
  quota_reserved = quota_reserved + $2,
  updated_at     = now()
WHERE id = $1
  AND (quota_limit IS NULL OR quota_limit - quota_used - quota_reserved >= $2);

-- name: ReleaseAPIKeyQuotaReserve :exec
UPDATE ai_api_keys
SET
  quota_reserved = GREATEST(0, quota_reserved - $2),
  updated_at     = now()
WHERE id = $1;

-- name: UpdateUsageLogBillingStatus :exec
UPDATE ai_usage_logs
SET
  urm_transaction_id = $2,
  billing_status     = $3,
  provider_cost      = $4,
  platform_cost      = $5,
  user_cost          = $6,
  api_key_quota_cost = $7,
  billable_units     = $8
WHERE request_id = $1;

-- name: UpdateUsageLogCompletion :exec
UPDATE ai_usage_logs
SET
  prompt_tokens          = $2,
  completion_tokens      = $3,
  cache_write_tokens     = $4,
  cache_read_tokens      = $5,
  reasoning_tokens       = $6,
  total_tokens           = $7,
  request_status         = $8,
  http_status            = $9,
  latency_ms             = $10,
  first_token_latency_ms = $11,
  error_code             = $12,
  error_message          = $13
WHERE request_id = $1;

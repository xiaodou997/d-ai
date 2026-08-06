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
  client_user_agent,
  external_user_id,
  group_id,
  group_name_snapshot,
  group_default_user_multiplier_snapshot,
  user_multiplier_override_snapshot,
  effective_user_multiplier_snapshot,
  billing_group_label_snapshot,
  model_code,
  requested_model,
  matched_dispatch_rule_id,
  matched_dispatch_rule_summary,
  resolved_logical_model,
  resolved_provider_family,
  capability_type,
  group_target_id,
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
  reasoning_effort,
  total_tokens,
  billable_unit_type,
  billable_units,
  catalog_base,
  tenant_payable,
  retail_base,
  user_payable,
  user_charged,
  api_key_quota_cost,
  service_tier,
  billing_breakdown,
  billing_event_id,
  billing_status,
  request_status,
  http_status,
  upstream_status,
  latency_ms,
  first_token_latency_ms,
  request_total_ms,
  request_setup_ms,
  first_response_byte_ms,
  response_tail_ms,
  final_attempt_header_ms,
  final_attempt_total_ms,
  error_code,
  error_message,
  usage_estimated,
  token_usage_source,
  attempts_count,
  final_route_id,
  client_protocol,
  resolution,
  protocol_conversion_enabled,
  upstream_model_mapping_applied,
  public_response_model,
  app_id,
  app_name_snapshot,
  app_owner_type_snapshot,
  app_owner_tenant_id_snapshot,
  app_owner_user_id_snapshot,
  billing_source,
  subscription_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
  $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
  $31, $32, $33, $34, $35, $36, $37, $38, $39, $40,
  $41, $42, $43, $44, $45, $46, $47, $48, $49, $50,
  $51, $52, $53, $54, $55, $56, $57, $58, $59, $60,
  $61, $62, $63, $64, $65, $66, $67, $68, $69, $70,
	$71, $72, $73, $74, $75, $76, $77, $78, $79, $80
)
ON CONFLICT (request_id) DO NOTHING
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
  catalog_base,
  tenant_payable,
  retail_base,
  user_payable,
  user_charged,
  api_key_quota_cost,
  latency_success_sum_ms,
  latency_success_count,
  request_total_success_sum_ms,
  request_total_success_count,
  first_response_byte_success_sum_ms,
  first_response_byte_success_count
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
  sqlc.arg('catalog_base')::bigint,
  sqlc.arg('tenant_payable')::bigint,
  sqlc.arg('retail_base')::bigint,
  sqlc.arg('user_payable')::bigint,
  sqlc.arg('user_charged')::bigint,
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
  END,
  CASE
    WHEN sqlc.arg('request_status') = 'success' AND sqlc.narg('request_total_ms')::integer IS NOT NULL
      THEN sqlc.narg('request_total_ms')::bigint
    ELSE 0
  END,
  CASE
    WHEN sqlc.arg('request_status') = 'success' AND sqlc.narg('request_total_ms')::integer IS NOT NULL
      THEN 1
    ELSE 0
  END,
  CASE
    WHEN sqlc.arg('request_status') = 'success' AND sqlc.narg('first_response_byte_ms')::integer IS NOT NULL
      THEN sqlc.narg('first_response_byte_ms')::bigint
    ELSE 0
  END,
  CASE
    WHEN sqlc.arg('request_status') = 'success' AND sqlc.narg('first_response_byte_ms')::integer IS NOT NULL
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
  catalog_base            = ai_usage_rollups_hourly.catalog_base            + EXCLUDED.catalog_base,
  tenant_payable           = ai_usage_rollups_hourly.tenant_payable           + EXCLUDED.tenant_payable,
  retail_base             = ai_usage_rollups_hourly.retail_base             + EXCLUDED.retail_base,
  user_payable             = ai_usage_rollups_hourly.user_payable             + EXCLUDED.user_payable,
  user_charged              = ai_usage_rollups_hourly.user_charged              + EXCLUDED.user_charged,
  api_key_quota_cost      = ai_usage_rollups_hourly.api_key_quota_cost      + EXCLUDED.api_key_quota_cost,
  latency_success_sum_ms  = ai_usage_rollups_hourly.latency_success_sum_ms  + EXCLUDED.latency_success_sum_ms,
  latency_success_count   = ai_usage_rollups_hourly.latency_success_count   + EXCLUDED.latency_success_count,
  request_total_success_sum_ms = ai_usage_rollups_hourly.request_total_success_sum_ms + EXCLUDED.request_total_success_sum_ms,
  request_total_success_count  = ai_usage_rollups_hourly.request_total_success_count  + EXCLUDED.request_total_success_count,
  first_response_byte_success_sum_ms = ai_usage_rollups_hourly.first_response_byte_success_sum_ms + EXCLUDED.first_response_byte_success_sum_ms,
  first_response_byte_success_count  = ai_usage_rollups_hourly.first_response_byte_success_count  + EXCLUDED.first_response_byte_success_count,
  updated_at = now();

-- name: ConfirmAPIKeyQuotaUsage :execrows
UPDATE ai_api_keys
SET
	quota_used = quota_used + $2,
	updated_at = now()
WHERE id = $1;

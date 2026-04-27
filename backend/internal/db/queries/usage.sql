-- name: CreateUsageLog :one
INSERT INTO ai_usage_logs (
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
  usage_source
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
  $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
  $31, $32, $33, $34, $35
)
RETURNING id;

-- name: GetActiveModelPrice :one
SELECT
  platform_input_price_per_1m,
  platform_output_price_per_1m,
  platform_image_price,
  tenant_input_price_per_1m,
  tenant_output_price_per_1m,
  tenant_image_price
FROM ai_model_prices
WHERE model_id = $1
  AND status = 'active'
  AND effective_from <= now()
ORDER BY effective_from DESC
LIMIT 1;

-- name: GetActiveProviderModelPrice :one
SELECT
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  video_cost_per_second
FROM ai_provider_model_prices
WHERE provider_id = $1
  AND (endpoint_id = $2 OR endpoint_id IS NULL)
  AND upstream_model = $3
  AND capability_type = $4
  AND status = 'active'
  AND effective_from <= now()
ORDER BY
  CASE WHEN endpoint_id = $2 THEN 0 ELSE 1 END,
  effective_from DESC
LIMIT 1;

-- name: ConfirmAPIKeyQuotaUsage :exec
UPDATE ai_api_keys
SET
  quota_used = quota_used + $2,
  updated_at = now()
WHERE id = $1;

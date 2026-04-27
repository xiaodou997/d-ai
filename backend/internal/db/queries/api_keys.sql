-- name: GetAPIKeyByHash :one
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
WHERE key_hash = $1
  AND status = 'active'
  AND (expires_at IS NULL OR expires_at > now());


-- name: ConfirmAPIKeyQuotaUsage :execrows
UPDATE ai_api_keys
SET
	quota_used = quota_used + $2,
	updated_at = now()
WHERE id = $1;

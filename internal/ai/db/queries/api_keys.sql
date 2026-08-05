-- name: CreateAPIKey :one
INSERT INTO ai_api_keys (
  owner_type,
  tenant_id,
  user_id,
  group_id,
  key_hash,
  key_ciphertext,
  last_four,
  name,
  quota_limit,
  status,
  expires_at,
  created_by
) VALUES (
  $1, $2, sqlc.narg('user_id'), $3, $4, $5, $6, $7, sqlc.narg('quota_limit'), $8, sqlc.narg('expires_at'), sqlc.narg('created_by')
)
RETURNING
  id, owner_type, tenant_id, user_id, group_id, last_four, name,
  quota_limit, quota_used,
  status, expires_at, last_used_at, created_by, created_at, updated_at;

-- name: ListAPIKeys :many
SELECT
  id, owner_type, tenant_id, user_id, group_id, last_four, name,
  quota_limit, quota_used,
  status, expires_at, last_used_at, created_by, created_at, updated_at
FROM ai_api_keys
WHERE tenant_id = $1
  AND (sqlc.narg('owner_type')::text IS NULL OR owner_type = sqlc.narg('owner_type')::text)
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text)
ORDER BY created_at DESC;

-- name: GetAPIKeyByID :one
SELECT
  id, owner_type, tenant_id, user_id, group_id, key_hash, key_ciphertext, last_four, name,
  quota_limit, quota_used,
  status, expires_at, last_used_at, created_by, created_at, updated_at
FROM ai_api_keys
WHERE id = $1
  AND tenant_id = $2;

-- name: GetAPIKeySecretByID :one
SELECT key_ciphertext
FROM ai_api_keys
WHERE id = $1
  AND tenant_id = $2;

-- name: GetAPIKeyByHash :one
SELECT
  id, owner_type, tenant_id, user_id, group_id, last_four, name,
  quota_limit, quota_used,
  status, expires_at, last_used_at, created_by, created_at, updated_at
FROM ai_api_keys
WHERE key_hash = $1;

-- name: UpdateAPIKey :one
UPDATE ai_api_keys
SET group_id       = $3,
    name           = $4,
    quota_limit    = sqlc.narg('quota_limit'),
    status         = $5,
    expires_at     = sqlc.narg('expires_at'),
    updated_at     = now()
WHERE id = $1
  AND tenant_id = $2
RETURNING
  key_hash, id, owner_type, tenant_id, user_id, group_id, last_four, name,
  quota_limit, quota_used,
  status, expires_at, last_used_at, created_by, created_at, updated_at;

-- name: UpdateAPIKeyStatus :one
UPDATE ai_api_keys
SET status     = $3,
    updated_at = now()
WHERE id = $1
  AND tenant_id = $2
RETURNING
  key_hash, id, owner_type, tenant_id, user_id, group_id, last_four, name,
  quota_limit, quota_used,
  status, expires_at, last_used_at, created_by, created_at, updated_at;

-- name: RotateAPIKey :one
UPDATE ai_api_keys
SET key_hash   = $3,
    key_ciphertext = $4,
    last_four  = $5,
    updated_at = now()
WHERE id = $1
  AND tenant_id = $2
RETURNING
  key_hash, id, owner_type, tenant_id, user_id, group_id, last_four, name,
  quota_limit, quota_used,
  status, expires_at, last_used_at, created_by, created_at, updated_at;

-- name: DeleteAPIKey :one
DELETE FROM ai_api_keys
WHERE id = $1
  AND tenant_id = $2
RETURNING key_hash;

-- name: TouchLastUsedAt :exec
UPDATE ai_api_keys
SET last_used_at = now()
WHERE id = $1
  AND (last_used_at IS NULL OR last_used_at < now() - interval '5 minutes');

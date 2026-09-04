-- ============================================================================
-- 风控中心（内容安全审核）查询。配置本身走通用 ai_settings.GetSetting /
-- UpsertSetting（见 pricing.sql），此处只覆盖检测日志与风险事件。
-- ============================================================================

-- name: InsertContentModerationLog :one
INSERT INTO ai_content_moderation_logs (
  request_id, tenant_id, user_id, api_key_id, model_code, capability_type,
  mode, action, flagged, matched_keyword, highest_category, highest_score,
  category_scores, threshold_snapshot, input_excerpt, input_hash, upstream_latency_ms, error,
  hit_layer
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
) RETURNING id, created_at;

-- name: ListContentModerationLogs :many
SELECT
  id, request_id, tenant_id, user_id, api_key_id, model_code, capability_type,
  mode, action, flagged, matched_keyword, highest_category, highest_score,
  category_scores, threshold_snapshot, input_excerpt, input_hash, upstream_latency_ms, error,
  hit_layer, created_at
FROM ai_content_moderation_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id')::text)
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text)
  AND (sqlc.narg('mode')::text IS NULL OR mode = sqlc.narg('mode')::text)
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('flagged')::boolean IS NULL OR flagged = sqlc.narg('flagged')::boolean)
  AND (sqlc.narg('hit_layer')::text IS NULL OR hit_layer = sqlc.narg('hit_layer')::text)
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountContentModerationLogs :one
SELECT COUNT(*) AS count
FROM ai_content_moderation_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id')::text)
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text)
  AND (sqlc.narg('mode')::text IS NULL OR mode = sqlc.narg('mode')::text)
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('flagged')::boolean IS NULL OR flagged = sqlc.narg('flagged')::boolean)
  AND (sqlc.narg('hit_layer')::text IS NULL OR hit_layer = sqlc.narg('hit_layer')::text)
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz);

-- name: CountFlaggedModerationLogsSince :one
-- 用于滚动窗口违规计数：某用户自 since 起累计命中(flagged=true)次数。
SELECT COUNT(*) AS count
FROM ai_content_moderation_logs
WHERE user_id = $1 AND flagged = true AND created_at >= $2;

-- name: InsertRiskEvent :one
INSERT INTO ai_risk_events (
  event_type, severity, tenant_id, user_id, source_log_id, summary, detail
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
) RETURNING id, created_at;

-- name: ListRiskEvents :many
SELECT
  id, event_type, severity, tenant_id, user_id, source_log_id, summary, detail,
  status, resolved_by, resolved_at, resolution_note, created_at
FROM ai_risk_events
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id')::text)
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountRiskEvents :one
SELECT COUNT(*) AS count
FROM ai_risk_events
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id')::text)
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text);

-- name: ResolveRiskEvent :one
UPDATE ai_risk_events
SET status = $2, resolved_by = $3, resolved_at = now(), resolution_note = $4
WHERE id = $1
RETURNING id, event_type, severity, tenant_id, user_id, source_log_id, summary, detail,
  status, resolved_by, resolved_at, resolution_note, created_at;

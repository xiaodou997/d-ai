-- ============================================================================
-- 定价体系（Price Book 统一定价）查询
-- 详见 docs/PRICING_REFACTOR_PLAN.md。单价单位：USD per token。
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 价格表 Price Book CRUD
-- ----------------------------------------------------------------------------

-- name: CreatePriceBook :one
INSERT INTO ai_price_books (owner_type, owner_tenant_id, name, description)
VALUES ($1, $2, $3, $4)
RETURNING id, owner_type, owner_tenant_id, name, description, status, revision, created_at, updated_at;

-- name: GetPriceBook :one
SELECT id, owner_type, owner_tenant_id, name, description, status, revision, created_at, updated_at
FROM ai_price_books
WHERE id = $1;

-- name: ListPriceBooks :many
SELECT id, owner_type, owner_tenant_id, name, description, status, revision, created_at, updated_at
FROM ai_price_books
WHERE owner_type = 'platform'
ORDER BY created_at ASC;

-- name: ListVisiblePriceBooks :many
SELECT id, owner_type, owner_tenant_id, name, description, status, revision, created_at, updated_at
FROM ai_price_books
WHERE status = 'active'
  AND (owner_type = 'platform' OR (owner_type = 'tenant' AND owner_tenant_id = $1))
ORDER BY owner_type ASC, created_at ASC;

-- name: UpdatePriceBook :one
UPDATE ai_price_books
SET name        = $2,
    description = $3,
    status      = $4,
    revision    = revision + 1,
    updated_at  = now()
WHERE id = $1
RETURNING id, owner_type, owner_tenant_id, name, description, status, revision, created_at, updated_at;

-- name: DeletePriceBook :exec
DELETE FROM ai_price_books WHERE id = $1;

-- ----------------------------------------------------------------------------
-- 价格表条目 Price Book Entry
-- ----------------------------------------------------------------------------

-- name: UpsertPriceBookEntry :one
-- 管理员手动 upsert：置 source='manual'、manually_edited=true，使后续 LiteLLM 导入跳过。
INSERT INTO ai_price_book_entries (
  price_book_id, model_code, capability_type,
  token_price_tiers,
  image_default_price, video_default_price,
  image_prices, video_prices,
  audio_tts_per_char, audio_stt_per_minute,
  source, manually_edited
) VALUES (
  $1, $2, $3,
  $4,
  $5, $6,
  $7, $8,
  $9, $10,
  'manual', true
)
ON CONFLICT (price_book_id, model_code, capability_type) DO UPDATE SET
  token_price_tiers     = EXCLUDED.token_price_tiers,
  image_default_price   = EXCLUDED.image_default_price,
  video_default_price   = EXCLUDED.video_default_price,
  image_prices          = EXCLUDED.image_prices,
  video_prices          = EXCLUDED.video_prices,
  audio_tts_per_char    = EXCLUDED.audio_tts_per_char,
  audio_stt_per_minute  = EXCLUDED.audio_stt_per_minute,
  source                = 'manual',
  manually_edited       = true,
  updated_at            = now()
RETURNING id, price_book_id, model_code, capability_type,
  token_price_tiers,
  image_default_price, video_default_price,
  image_prices, video_prices,
  audio_tts_per_char, audio_stt_per_minute,
  source, manually_edited, created_at, updated_at;

-- name: ImportLiteLLMEntry :execrows
-- LiteLLM「仅填空」导入：新增条目；已存在且 manually_edited=false 时刷新，手改条目跳过。
INSERT INTO ai_price_book_entries (
  price_book_id, model_code, capability_type,
  token_price_tiers,
  source, manually_edited
) VALUES (
  $1, $2, $3,
  $4,
  'litellm', false
)
ON CONFLICT (price_book_id, model_code, capability_type) DO UPDATE SET
  token_price_tiers     = EXCLUDED.token_price_tiers,
  source                = 'litellm',
  updated_at            = now()
WHERE ai_price_book_entries.manually_edited = false
  AND (ai_price_book_entries.token_price_tiers IS DISTINCT FROM EXCLUDED.token_price_tiers
       OR ai_price_book_entries.source IS DISTINCT FROM EXCLUDED.source);

-- name: GetPriceBookEntry :one
-- 运行时按租户可见模型代码和能力类型取价。
SELECT id, price_book_id, model_code, capability_type,
  token_price_tiers,
  image_default_price, video_default_price,
  image_prices, video_prices,
  audio_tts_per_char, audio_stt_per_minute,
  source, manually_edited, created_at, updated_at
FROM ai_price_book_entries
WHERE price_book_id = $1 AND model_code = $2 AND capability_type = $3;

-- name: ListPriceBookEntries :many
SELECT id, price_book_id, model_code, capability_type,
  token_price_tiers,
  image_default_price, video_default_price,
  image_prices, video_prices,
  audio_tts_per_char, audio_stt_per_minute,
  source, manually_edited, created_at, updated_at
FROM ai_price_book_entries
WHERE price_book_id = $1
ORDER BY model_code ASC, capability_type ASC;

-- name: DeletePriceBookEntry :exec
DELETE FROM ai_price_book_entries
WHERE price_book_id = $1 AND model_code = $2 AND capability_type = $3;

-- ----------------------------------------------------------------------------
-- 全局设置 Settings
-- ----------------------------------------------------------------------------

-- name: GetSetting :one
SELECT key, value, updated_at FROM ai_settings WHERE key = $1;

-- name: UpsertSetting :exec
INSERT INTO ai_settings (key, value, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (key) DO UPDATE SET
  value      = EXCLUDED.value,
  updated_at = now();

-- ----------------------------------------------------------------------------
-- 分组零售定价：ai_groups.retail_price_book_id + default_user_multiplier；
-- ai_user_groups.user_multiplier_override 可直接覆盖分组默认倍率；关系存在即授权。
-- ----------------------------------------------------------------------------

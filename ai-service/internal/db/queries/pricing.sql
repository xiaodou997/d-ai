-- ============================================================================
-- 定价体系（Price Book 统一定价）查询
-- 详见 docs/PRICING_REFACTOR_PLAN.md。单价单位：USD per token。
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 价格表 Price Book CRUD
-- ----------------------------------------------------------------------------

-- name: CreatePriceBook :one
INSERT INTO ai_price_books (name, description)
VALUES ($1, $2)
RETURNING id, name, description, status, created_at, updated_at;

-- name: GetPriceBook :one
SELECT id, name, description, status, created_at, updated_at
FROM ai_price_books
WHERE id = $1;

-- name: ListPriceBooks :many
SELECT id, name, description, status, created_at, updated_at
FROM ai_price_books
ORDER BY created_at ASC;

-- name: UpdatePriceBook :one
UPDATE ai_price_books
SET name        = $2,
    description = $3,
    status      = $4,
    updated_at  = now()
WHERE id = $1
RETURNING id, name, description, status, created_at, updated_at;

-- name: DeletePriceBook :exec
DELETE FROM ai_price_books WHERE id = $1;

-- ----------------------------------------------------------------------------
-- 价格表条目 Price Book Entry
-- ----------------------------------------------------------------------------

-- name: UpsertPriceBookEntry :one
-- 管理员手动 upsert：置 source='manual'、manually_edited=true，使后续 LiteLLM 导入跳过。
INSERT INTO ai_price_book_entries (
  price_book_id, model_code, capability_type,
  input_per_token, output_per_token,
  cache_write_per_token, cache_read_per_token, reasoning_per_token,
  image_prices, video_prices,
  audio_tts_per_char, audio_stt_per_minute,
  source, manually_edited
) VALUES (
  $1, $2, $3,
  $4, $5,
  $6, $7, $8,
  $9, $10,
  $11, $12,
  'manual', true
)
ON CONFLICT (price_book_id, model_code) DO UPDATE SET
  capability_type       = EXCLUDED.capability_type,
  input_per_token       = EXCLUDED.input_per_token,
  output_per_token      = EXCLUDED.output_per_token,
  cache_write_per_token = EXCLUDED.cache_write_per_token,
  cache_read_per_token  = EXCLUDED.cache_read_per_token,
  reasoning_per_token   = EXCLUDED.reasoning_per_token,
  image_prices          = EXCLUDED.image_prices,
  video_prices          = EXCLUDED.video_prices,
  audio_tts_per_char    = EXCLUDED.audio_tts_per_char,
  audio_stt_per_minute  = EXCLUDED.audio_stt_per_minute,
  source                = 'manual',
  manually_edited       = true,
  updated_at            = now()
RETURNING id, price_book_id, model_code, capability_type,
  input_per_token, output_per_token,
  cache_write_per_token, cache_read_per_token, reasoning_per_token,
  image_prices, video_prices,
  audio_tts_per_char, audio_stt_per_minute,
  source, manually_edited, created_at, updated_at;

-- name: ImportLiteLLMEntry :exec
-- LiteLLM「仅填空」导入：新增条目；已存在且 manually_edited=false 时刷新，手改条目跳过。
INSERT INTO ai_price_book_entries (
  price_book_id, model_code, capability_type,
  input_per_token, output_per_token,
  cache_write_per_token, cache_read_per_token, reasoning_per_token,
  source, manually_edited
) VALUES (
  $1, $2, $3,
  $4, $5,
  $6, $7, $8,
  'litellm', false
)
ON CONFLICT (price_book_id, model_code) DO UPDATE SET
  capability_type       = EXCLUDED.capability_type,
  input_per_token       = EXCLUDED.input_per_token,
  output_per_token      = EXCLUDED.output_per_token,
  cache_write_per_token = EXCLUDED.cache_write_per_token,
  cache_read_per_token  = EXCLUDED.cache_read_per_token,
  reasoning_per_token   = EXCLUDED.reasoning_per_token,
  source                = 'litellm',
  updated_at            = now()
WHERE ai_price_book_entries.manually_edited = false;

-- name: GetPriceBookEntry :one
-- 运行时按 (price_book, model_code) 取价。
SELECT id, price_book_id, model_code, capability_type,
  input_per_token, output_per_token,
  cache_write_per_token, cache_read_per_token, reasoning_per_token,
  image_prices, video_prices,
  audio_tts_per_char, audio_stt_per_minute,
  source, manually_edited, created_at, updated_at
FROM ai_price_book_entries
WHERE price_book_id = $1 AND model_code = $2;

-- name: ListPriceBookEntries :many
SELECT id, price_book_id, model_code, capability_type,
  input_per_token, output_per_token,
  cache_write_per_token, cache_read_per_token, reasoning_per_token,
  image_prices, video_prices,
  audio_tts_per_char, audio_stt_per_minute,
  source, manually_edited, created_at, updated_at
FROM ai_price_book_entries
WHERE price_book_id = $1
ORDER BY model_code ASC;

-- name: DeletePriceBookEntry :exec
DELETE FROM ai_price_book_entries
WHERE price_book_id = $1 AND model_code = $2;

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
-- 平台→租户售价绑定
-- ----------------------------------------------------------------------------

-- name: UpsertTenantSellBinding :one
INSERT INTO ai_tenant_sell_bindings (
  tenant_id, price_book_id, sell_multiplier, cache_billing_enabled
) VALUES ($1, $2, $3, $4)
ON CONFLICT (tenant_id) DO UPDATE SET
  price_book_id         = EXCLUDED.price_book_id,
  sell_multiplier       = EXCLUDED.sell_multiplier,
  cache_billing_enabled = EXCLUDED.cache_billing_enabled,
  updated_at            = now()
RETURNING id, tenant_id, price_book_id, sell_multiplier, cache_billing_enabled, created_at, updated_at;

-- name: GetTenantSellBinding :one
SELECT id, tenant_id, price_book_id, sell_multiplier, cache_billing_enabled, created_at, updated_at
FROM ai_tenant_sell_bindings
WHERE tenant_id = $1;

-- name: ListTenantSellBindings :many
SELECT b.id, b.tenant_id, b.price_book_id, b.sell_multiplier, b.cache_billing_enabled,
       b.created_at, b.updated_at, pb.name AS price_book_name
FROM ai_tenant_sell_bindings b
JOIN ai_price_books pb ON pb.id = b.price_book_id
ORDER BY b.created_at ASC;

-- name: DeleteTenantSellBinding :exec
DELETE FROM ai_tenant_sell_bindings WHERE tenant_id = $1;

-- ----------------------------------------------------------------------------
-- 租户→用户售价绑定（级联）
-- ----------------------------------------------------------------------------

-- name: UpsertUserSellBinding :one
INSERT INTO ai_user_sell_bindings (
  tenant_id, user_multiplier, cache_billing_enabled
) VALUES ($1, $2, $3)
ON CONFLICT (tenant_id) DO UPDATE SET
  user_multiplier       = EXCLUDED.user_multiplier,
  cache_billing_enabled = EXCLUDED.cache_billing_enabled,
  updated_at            = now()
RETURNING id, tenant_id, user_multiplier, cache_billing_enabled, created_at, updated_at;

-- name: GetUserSellBinding :one
SELECT id, tenant_id, user_multiplier, cache_billing_enabled, created_at, updated_at
FROM ai_user_sell_bindings
WHERE tenant_id = $1;

-- name: DeleteUserSellBinding :exec
DELETE FROM ai_user_sell_bindings WHERE tenant_id = $1;

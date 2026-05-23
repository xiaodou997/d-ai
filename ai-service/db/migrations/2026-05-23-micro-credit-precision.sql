-- ============================================================================
-- 2026-05-23: 计费精度升级 —— 积分 → 微积分（micro-credit）
--
-- 背景：
--   原方案中 *_price_per_1m / *_cost 等列的单位是「积分」(int64，1 积分=1 分钱)。
--   整数除法导致 300 token 短请求 × 100 积分/M token 的精算结果为 0.03 积分，
--   被强制取整到 1 积分（adapters/postgres/billing.go 的 floor-to-1 兜底），
--   水分高达 30 倍。Phase 0 把内部单位升级为「微积分」(1 积分 = 10000 微积分)，
--   精度从 1 分钱细化到 0.0001 分钱，足以承载所有便宜模型的真实定价。
--
-- 影响范围：
--   - ai_model_prices / ai_tenant_model_price_overrides / ai_tenant_user_prices
--     上的所有 *_price_per_1m 列与 image_prices / video_prices JSONB 中的 price 字段
--   - ai_usage_logs 上的 provider_cost / platform_cost / user_cost / api_key_quota_cost
--     与 billable_units（仅 unit_type='token' 时不动；其他 unit_type 不变）
--
-- 执行方式：
--   全部包在单事务里，× 10000 平移。列类型不变（BIGINT 足够，int64 上限 9.2e18，
--   即使按 micro 也撑得住 9.2e14 元的累计金额）。
--
-- 回滚：将所有 UPDATE 中的 *10000 改为 /10000 即可（不损失精度，因为原数据本就是
-- 整数积分；但若 Phase 0 上线后已产生小数 micro 残值，回滚会丢精度）。
-- ============================================================================

BEGIN;

-- 平台基准定价
UPDATE ai_model_prices SET
  input_price_per_1m           = input_price_per_1m * 10000,
  output_price_per_1m          = output_price_per_1m * 10000,
  cache_write_price_per_1m     = cache_write_price_per_1m * 10000,
  cache_read_price_per_1m      = cache_read_price_per_1m * 10000,
  reasoning_price_per_1m       = reasoning_price_per_1m * 10000,
  audio_tts_price_per_1m_chars = audio_tts_price_per_1m_chars * 10000,
  audio_stt_price_per_minute   = audio_stt_price_per_minute * 10000,
  updated_at                   = now();

-- 租户级覆盖
UPDATE ai_tenant_model_price_overrides SET
  input_price_per_1m           = input_price_per_1m * 10000,
  output_price_per_1m          = output_price_per_1m * 10000,
  cache_write_price_per_1m     = cache_write_price_per_1m * 10000,
  cache_read_price_per_1m      = cache_read_price_per_1m * 10000,
  reasoning_price_per_1m       = reasoning_price_per_1m * 10000,
  audio_tts_price_per_1m_chars = audio_tts_price_per_1m_chars * 10000,
  audio_stt_price_per_minute   = audio_stt_price_per_minute * 10000,
  updated_at                   = now();

-- 租户对用户的定价
UPDATE ai_tenant_user_prices SET
  input_price_per_1m           = input_price_per_1m * 10000,
  output_price_per_1m          = output_price_per_1m * 10000,
  cache_write_price_per_1m     = cache_write_price_per_1m * 10000,
  cache_read_price_per_1m      = cache_read_price_per_1m * 10000,
  reasoning_price_per_1m       = reasoning_price_per_1m * 10000,
  audio_tts_price_per_1m_chars = audio_tts_price_per_1m_chars * 10000,
  audio_stt_price_per_minute   = audio_stt_price_per_minute * 10000,
  updated_at                   = now();

-- image_prices / video_prices JSONB 中的 price 字段：递归更新每个元素的 price
-- JSONB 数组结构：[{"resolution": "1024x1024", "price": 100}, ...]
UPDATE ai_model_prices
SET image_prices = (
  SELECT COALESCE(jsonb_agg(jsonb_set(elem, '{price}', to_jsonb(((elem->>'price')::bigint) * 10000))), '[]'::jsonb)
  FROM jsonb_array_elements(image_prices) AS elem
)
WHERE jsonb_array_length(image_prices) > 0;

UPDATE ai_model_prices
SET video_prices = (
  SELECT COALESCE(jsonb_agg(jsonb_set(elem, '{price}', to_jsonb(((elem->>'price')::bigint) * 10000))), '[]'::jsonb)
  FROM jsonb_array_elements(video_prices) AS elem
)
WHERE jsonb_array_length(video_prices) > 0;

UPDATE ai_tenant_model_price_overrides
SET image_prices = (
  SELECT COALESCE(jsonb_agg(jsonb_set(elem, '{price}', to_jsonb(((elem->>'price')::bigint) * 10000))), '[]'::jsonb)
  FROM jsonb_array_elements(image_prices) AS elem
)
WHERE jsonb_array_length(image_prices) > 0;

UPDATE ai_tenant_model_price_overrides
SET video_prices = (
  SELECT COALESCE(jsonb_agg(jsonb_set(elem, '{price}', to_jsonb(((elem->>'price')::bigint) * 10000))), '[]'::jsonb)
  FROM jsonb_array_elements(video_prices) AS elem
)
WHERE jsonb_array_length(video_prices) > 0;

UPDATE ai_tenant_user_prices
SET image_prices = (
  SELECT COALESCE(jsonb_agg(jsonb_set(elem, '{price}', to_jsonb(((elem->>'price')::bigint) * 10000))), '[]'::jsonb)
  FROM jsonb_array_elements(image_prices) AS elem
)
WHERE jsonb_array_length(image_prices) > 0;

UPDATE ai_tenant_user_prices
SET video_prices = (
  SELECT COALESCE(jsonb_agg(jsonb_set(elem, '{price}', to_jsonb(((elem->>'price')::bigint) * 10000))), '[]'::jsonb)
  FROM jsonb_array_elements(video_prices) AS elem
)
WHERE jsonb_array_length(video_prices) > 0;

-- 历史 usage_log 金额列：把已有的「积分」量平移到「微积分」量
-- 注意：旧数据本来就有 floor-to-1 水分，平移后仍保留这个历史误差（× 10000 = 10000 micro = 1 整积分），
-- 不影响新数据精度。
UPDATE ai_usage_logs SET
  provider_cost      = provider_cost * 10000,
  platform_cost      = platform_cost * 10000,
  user_cost          = user_cost * 10000,
  api_key_quota_cost = api_key_quota_cost * 10000;

-- API Key 本地配额：quota_used 由 api_key_quota_cost 累加，需同步升级单位；
-- quota_limit / quota_reserved 同步乘 10000 以保持「相对额度」语义不变。
UPDATE ai_api_keys SET
  quota_limit    = quota_limit * 10000,
  quota_used     = quota_used * 10000,
  quota_reserved = quota_reserved * 10000
WHERE quota_limit IS NOT NULL OR quota_used > 0 OR quota_reserved > 0;

COMMIT;

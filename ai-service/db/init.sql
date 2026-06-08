-- ============================================================================
-- Uni AI API 数据库初始化脚本
-- 可直接在 Navicat、DBeaver 等数据库工具中执行
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================================
-- 定价体系（Price Book 统一定价）
-- 一套 USD 价格表，上游成本与对外售价共用同一目录；各处引用时设单倍率。
-- 详见 docs/PRICING_REFACTOR_PLAN.md。
-- 注意：定义在所有引用方（endpoints / deployments）之前，因为它们的 price_book_id 外键引用本表。
-- ============================================================================

-- 价格表：USD 目录。可手建，也可从 LiteLLM JSON 自动填充。
CREATE TABLE IF NOT EXISTS ai_price_books (
  id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT        NOT NULL UNIQUE,        -- "标准价" / "中转便宜价"
  description TEXT        NOT NULL DEFAULT '',
  status      TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 价格表条目：每 (price_book, model_code) 一行。
-- 单位约定：所有 *_per_token 为「USD / token」（与 LiteLLM 原生 input_cost_per_token 一致）。
--   image_prices/video_prices JSON 内 price 为 USD（每图 / 每秒）。
--   audio_tts_per_char 为 USD/字符，audio_stt_per_minute 为 USD/分钟。
-- 0 值语义：cache_write/cache_read 为 0 → 按 input 计；reasoning 为 0 → 按 output 计。
-- model_code 不外键 ai_models：成本价格表可包含未对外转售的上游模型。
-- manually_edited=true 时 LiteLLM 导入「仅填空」会跳过该条目，不覆盖手改价。
CREATE TABLE IF NOT EXISTS ai_price_book_entries (
  id                    UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
  price_book_id         UUID           NOT NULL REFERENCES ai_price_books(id) ON DELETE CASCADE,
  model_code            TEXT           NOT NULL,
  capability_type       TEXT           NOT NULL DEFAULT 'chat'
                        CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank')),
  input_per_token       NUMERIC(20,12) NOT NULL DEFAULT 0,
  output_per_token      NUMERIC(20,12) NOT NULL DEFAULT 0,
  cache_write_per_token NUMERIC(20,12) NOT NULL DEFAULT 0,
  cache_read_per_token  NUMERIC(20,12) NOT NULL DEFAULT 0,
  reasoning_per_token   NUMERIC(20,12) NOT NULL DEFAULT 0,
  image_prices          JSONB          NOT NULL DEFAULT '[]',   -- [{"resolution":"1024x1024","price":0.04}]
  video_prices          JSONB          NOT NULL DEFAULT '[]',   -- [{"resolution":"720p","price":0.05}]
  audio_tts_per_char    NUMERIC(20,12) NOT NULL DEFAULT 0,
  audio_stt_per_minute  NUMERIC(20,12) NOT NULL DEFAULT 0,
  source                TEXT           NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'litellm')),
  manually_edited       BOOLEAN        NOT NULL DEFAULT false,
  created_at            TIMESTAMPTZ    NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ    NOT NULL DEFAULT now(),
  UNIQUE (price_book_id, model_code),
  CONSTRAINT ai_price_book_entries_nonnegative CHECK (
    input_per_token >= 0 AND output_per_token >= 0
    AND cache_write_per_token >= 0 AND cache_read_per_token >= 0 AND reasoning_per_token >= 0
    AND audio_tts_per_char >= 0 AND audio_stt_per_minute >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_price_book_entries_book  ON ai_price_book_entries (price_book_id);
CREATE INDEX IF NOT EXISTS idx_ai_price_book_entries_model ON ai_price_book_entries (price_book_id, model_code);

-- 全局设置：key-value。当前用途：USD→积分汇率 credits_per_usd（默认 7，可动态调）。
CREATE TABLE IF NOT EXISTS ai_settings (
  key        TEXT        PRIMARY KEY,
  value      JSONB       NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO ai_settings (key, value) VALUES ('credits_per_usd', '7'::jsonb)
  ON CONFLICT (key) DO NOTHING;

-- 平台→租户售价绑定（一租户一倍率，覆盖该价格表所有模型）。
-- 租户售价(积分/token) = entry × sell_multiplier × credits_per_usd。
-- 缺绑定或缺条目 → 拒绝请求（fail-closed）。
-- cache_billing_enabled=false（默认）→ 缓存 token 按 input 价计。
CREATE TABLE IF NOT EXISTS ai_tenant_sell_bindings (
  id                    UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id             TEXT          NOT NULL UNIQUE,
  price_book_id         UUID          NOT NULL REFERENCES ai_price_books(id),
  sell_multiplier       NUMERIC(10,4) NOT NULL DEFAULT 1 CHECK (sell_multiplier >= 0),
  cache_billing_enabled BOOLEAN       NOT NULL DEFAULT false,
  created_at            TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- 租户→用户售价绑定（级联：基准=平台给该租户的售价，再×user_multiplier）。
-- 用户售价(积分/token) = entry × sell_multiplier × user_multiplier × credits_per_usd。
-- 租户不另选价格表，跟随平台为其选定的价格表。
CREATE TABLE IF NOT EXISTS ai_user_sell_bindings (
  id                    UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id             TEXT          NOT NULL UNIQUE,
  user_multiplier       NUMERIC(10,4) NOT NULL DEFAULT 1 CHECK (user_multiplier >= 0),
  cache_billing_enabled BOOLEAN       NOT NULL DEFAULT false,
  created_at            TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ   NOT NULL DEFAULT now()
);


-- ============================================================================
-- AI API Keys
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_api_keys (
  id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type     TEXT        NOT NULL CHECK (owner_type IN ('user', 'tenant')),
  tenant_id      TEXT        NOT NULL,
  user_id        TEXT,
  key_hash       TEXT        NOT NULL,
  last_four      CHAR(4),
  name           TEXT        NOT NULL,
  -- quota_* 列单位：微积分 (micro-credits)。NULL = 无上限。
  -- quota_used 由 ai_usage_logs.api_key_quota_cost 累加；预扣 quota_reserved 也是同单位。
  quota_limit    BIGINT,
  quota_used     BIGINT      NOT NULL DEFAULT 0,
  quota_reserved BIGINT      NOT NULL DEFAULT 0,
  allowed_models JSONB       NOT NULL DEFAULT '[]',
  status         TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  expires_at     TIMESTAMPTZ,
  last_used_at   TIMESTAMPTZ,
  created_by     TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((owner_type = 'tenant' AND user_id IS NULL) OR (owner_type = 'user' AND user_id IS NOT NULL)),
  CONSTRAINT ai_api_keys_quota_nonnegative CHECK (
    quota_used >= 0 AND quota_reserved >= 0
    AND (quota_limit IS NULL OR quota_limit >= 0)
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_api_keys_key_hash     ON ai_api_keys (key_hash);
CREATE INDEX IF NOT EXISTS idx_ai_api_keys_tenant              ON ai_api_keys (tenant_id);
CREATE INDEX IF NOT EXISTS idx_ai_api_keys_user                ON ai_api_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_ai_api_keys_tenant_status       ON ai_api_keys (tenant_id, status);

-- ============================================================================
-- AI Providers (厂商)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_providers (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  code       TEXT        NOT NULL UNIQUE,
  name       TEXT        NOT NULL,
  config     JSONB       NOT NULL DEFAULT '{}',
  status     TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================================
-- AI Provider Endpoints (接入点，API Key 型上游)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_provider_endpoints (
  id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id        UUID        NOT NULL,
  name               TEXT        NOT NULL,
  base_url           TEXT        NOT NULL,
  api_key_ciphertext TEXT        NOT NULL,
  extra_headers      JSONB       NOT NULL DEFAULT '{}',
  weight             INTEGER     NOT NULL DEFAULT 100 CHECK (weight >= 0),
  timeout_ms         INTEGER     NOT NULL DEFAULT 30000 CHECK (timeout_ms > 0),
  default_protocol   TEXT        NOT NULL DEFAULT 'openai_compatible' CHECK (default_protocol IN ('openai_compatible', 'anthropic', 'gemini')),
  -- 账户级上游成本绑定：该账户下所有 deployment 默认继承此价格表/倍率。
  -- deployment 可在自身上覆盖（见 ai_upstream_deployments）。NULL = 未绑定。
  price_book_id      UUID        REFERENCES ai_price_books(id),
  cost_multiplier    NUMERIC(10,4) CHECK (cost_multiplier IS NULL OR cost_multiplier >= 0),
  status             TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (provider_id, name)
);

CREATE INDEX IF NOT EXISTS idx_ai_provider_endpoints_provider ON ai_provider_endpoints (provider_id);
CREATE INDEX IF NOT EXISTS idx_ai_provider_endpoints_status   ON ai_provider_endpoints (status);

-- ============================================================================
-- AI Credential Pools (OAuth 账号池，对应 Codex/Claude OAuth/Gemini CLI 等固定厂商)
-- 一个 Pool 绑定一种 fixed_provider_type，池内有多个 OAuth Token 账号。
-- Pool 与 ModelRoute 是多对多关系（多条 Route 可共享同一 Pool）。
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_credential_pools (
  id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  name                TEXT        NOT NULL,
  fixed_provider_type TEXT        NOT NULL
    CHECK (fixed_provider_type IN ('codex', 'claude_oauth', 'gemini_cli', 'antigravity')),
  oauth_strategy      TEXT        NOT NULL DEFAULT 'round_robin'
    CHECK (oauth_strategy IN ('round_robin', 'weighted')),
  sticky_granularity  TEXT        NOT NULL DEFAULT 'credential'
    CHECK (sticky_granularity IN ('credential', 'pool')),
  notes               TEXT,
  status              TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_credential_pools_type   ON ai_credential_pools (fixed_provider_type);
CREATE INDEX IF NOT EXISTS idx_ai_credential_pools_status ON ai_credential_pools (status);

-- ============================================================================
-- AI Provider OAuth Credentials (Pool 内的 OAuth Token 账号)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_provider_oauth_credentials (
  id                       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  pool_id                  UUID        NOT NULL,
  name                     TEXT        NOT NULL,
  provider_type            TEXT        NOT NULL
    CHECK (provider_type IN ('codex', 'claude_oauth', 'gemini_cli', 'antigravity')),
  email                    TEXT,
  access_token_ciphertext  TEXT        NOT NULL,
  refresh_token_ciphertext TEXT,
  token_type               TEXT        NOT NULL DEFAULT 'bearer',
  scope                    TEXT,
  expires_at               TIMESTAMPTZ,
  auth_metadata            JSONB       NOT NULL DEFAULT '{}',
  weight                   INTEGER     NOT NULL DEFAULT 100 CHECK (weight >= 0),
  status                   TEXT        NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'invalid', 'disabled')),
  invalid_reason           TEXT,
  last_used_at             TIMESTAMPTZ,
  last_refreshed_at        TIMESTAMPTZ,
  last_failed_at           TIMESTAMPTZ,
  consecutive_fail_count   INTEGER     NOT NULL DEFAULT 0 CHECK (consecutive_fail_count >= 0),
  success_count            BIGINT      NOT NULL DEFAULT 0 CHECK (success_count >= 0),
  fail_count               BIGINT      NOT NULL DEFAULT 0 CHECK (fail_count >= 0),
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_oauth_cred_pool_status
  ON ai_provider_oauth_credentials (pool_id, status);
CREATE INDEX IF NOT EXISTS idx_oauth_cred_expires
  ON ai_provider_oauth_credentials (expires_at)
  WHERE status = 'active' AND refresh_token_ciphertext IS NOT NULL;

-- ============================================================================
-- AI Upstream Deployments (上游模型部署)
-- upstream_protocol 决定格式转换路径和 transport 行为
-- credential_source: endpoint_id (API Key型) XOR credential_pool_id (OAuth池型) 恰好一个
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_upstream_deployments (
  id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  endpoint_id          UUID,                   -- API Key型上游；与 credential_pool_id 互斥
  credential_pool_id   UUID        REFERENCES ai_credential_pools(id),
  upstream_model       TEXT        NOT NULL,
  capability_type      TEXT        NOT NULL DEFAULT 'chat',
  upstream_protocol    TEXT        NOT NULL DEFAULT 'openai_chat',
  request_path         TEXT,
  upstream_parameters  JSONB       NOT NULL DEFAULT '{}',
  -- 上游成本绑定（覆盖层）：留空则继承所属 endpoint 的绑定。
  -- 有效成本 = entry(COALESCE(此处, endpoint) price_book, upstream_model) × COALESCE(此处, endpoint, 1) 倍率。
  -- 二者最终都为 NULL（含 endpoint）或条目缺失 → 成本记 0 + 告警（不阻断请求）。
  price_book_id        UUID        REFERENCES ai_price_books(id),
  cost_multiplier      NUMERIC(10,4) CHECK (cost_multiplier IS NULL OR cost_multiplier >= 0),
  health_status        TEXT        NOT NULL DEFAULT 'unknown',
  last_health_check_at TIMESTAMPTZ,
  last_health_error    TEXT,
  status               TEXT        NOT NULL DEFAULT 'active',
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chk_deployment_credential_source CHECK (
    (endpoint_id IS NOT NULL AND credential_pool_id IS NULL) OR
    (endpoint_id IS NULL AND credential_pool_id IS NOT NULL)
  ),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank')),
  CHECK (upstream_protocol IN (
    'openai_chat',        -- OpenAI Chat Completions 及所有 OpenAI 兼容上游（国内模型）
    'openai_responses',   -- OpenAI Responses API（原生，参考 Aether）
    'openai_completions', -- Legacy Codex completions
    'openai_embeddings',  -- OpenAI Embeddings
    'openai_images',      -- OpenAI Image Generation
    'anthropic_messages', -- Anthropic Claude Messages API（原生）
    'gemini_generate',    -- Google Gemini GenerateContent（原生）
    'gemini_embeddings'   -- Google Gemini Embeddings（原生）
  )),
  CHECK (health_status IN ('healthy', 'unhealthy', 'unknown')),
  CHECK (status IN ('active', 'disabled')),
  UNIQUE NULLS NOT DISTINCT (endpoint_id, credential_pool_id, upstream_model, upstream_protocol)
);

CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_endpoint   ON ai_upstream_deployments (endpoint_id) WHERE endpoint_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_pool       ON ai_upstream_deployments (credential_pool_id) WHERE credential_pool_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_capability ON ai_upstream_deployments (capability_type, status);
CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_health     ON ai_upstream_deployments (health_status, status);

-- ============================================================================
-- AI Models (对外公开模型)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_models (
  id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  model_code                TEXT        NOT NULL UNIQUE,
  capability_type           TEXT        NOT NULL DEFAULT 'chat',
  context_window            INTEGER,
  default_max_output_tokens INTEGER     NOT NULL DEFAULT 2048,
  max_output_tokens         INTEGER,
  -- 请求服务三段式超时（model 级覆盖；NULL=继承全局 config）。解析优先级 route > model > 全局。
  connect_timeout_ms        INTEGER     CHECK (connect_timeout_ms    IS NULL OR connect_timeout_ms    > 0),
  first_byte_timeout_ms     INTEGER     CHECK (first_byte_timeout_ms IS NULL OR first_byte_timeout_ms > 0),
  idle_timeout_ms           INTEGER     CHECK (idle_timeout_ms       IS NULL OR idle_timeout_ms       > 0),
  max_duration_ms           INTEGER     CHECK (max_duration_ms       IS NULL OR max_duration_ms       > 0),
  status                    TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank'))
);

CREATE INDEX IF NOT EXISTS idx_ai_models_status     ON ai_models (status);
CREATE INDEX IF NOT EXISTS idx_ai_models_capability ON ai_models (capability_type, status);

-- ============================================================================
-- AI Model Routes (模型 → 上游部署 路由映射)
-- 统一只认 upstream_deployment_id，Pool 路由也通过 ai_upstream_deployments 中的池部署记录绑定
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_model_routes (
  id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id               UUID        NOT NULL,
  upstream_deployment_id UUID        NOT NULL,
  priority               INTEGER     NOT NULL DEFAULT 100 CHECK (priority >= 0),
  weight                 INTEGER     NOT NULL DEFAULT 100 CHECK (weight >= 0),
  supports_stream        BOOLEAN     NOT NULL DEFAULT true,
  cost_per_1k_tokens     NUMERIC(10,6) NOT NULL DEFAULT 0,
  score_weights_override JSONB,
  sticky_enabled         BOOLEAN     NOT NULL DEFAULT true,
  -- 请求服务三段式超时（route 级覆盖；NULL=继承 model，再继承全局 config）。
  connect_timeout_ms     INTEGER     CHECK (connect_timeout_ms    IS NULL OR connect_timeout_ms    > 0),
  first_byte_timeout_ms  INTEGER     CHECK (first_byte_timeout_ms IS NULL OR first_byte_timeout_ms > 0),
  idle_timeout_ms        INTEGER     CHECK (idle_timeout_ms       IS NULL OR idle_timeout_ms       > 0),
  max_duration_ms        INTEGER     CHECK (max_duration_ms       IS NULL OR max_duration_ms       > 0),
  status                 TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (model_id, upstream_deployment_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_model_routes_model      ON ai_model_routes (model_id, status);
CREATE INDEX IF NOT EXISTS idx_ai_model_routes_deployment ON ai_model_routes (upstream_deployment_id);

-- ============================================================================
-- Console Chat Sessions (网页聊天)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_console_sessions (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         TEXT        NOT NULL,
  user_id           TEXT,
  owner_type        TEXT        NOT NULL CHECK (owner_type IN ('tenant', 'user')),
  title             TEXT        NOT NULL DEFAULT '新对话',
  model_code        TEXT        NOT NULL DEFAULT '',
  selected_protocol TEXT,
  selected_route_id UUID,
  status            TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleted')),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (selected_protocol IS NULL OR selected_protocol IN (
    'openai_chat',
    'openai_responses',
    'anthropic_messages',
    'gemini_generate'
  ))
);

CREATE INDEX IF NOT EXISTS idx_ai_console_sessions_owner
  ON ai_console_sessions (tenant_id, owner_type, COALESCE(user_id, ''), updated_at DESC)
  WHERE status <> 'deleted';

CREATE TABLE IF NOT EXISTS ai_console_messages (
  id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id   UUID        NOT NULL REFERENCES ai_console_sessions(id) ON DELETE CASCADE,
  role         TEXT        NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
  content      TEXT        NOT NULL DEFAULT '',
  protocol     TEXT,
  route_id     UUID,
  usage_json   JSONB       NOT NULL DEFAULT '{}',
  error_json   JSONB       NOT NULL DEFAULT '{}',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_console_messages_session
  ON ai_console_messages (session_id, created_at ASC);

-- ============================================================================
-- AI Tenant Model Grants (租户级模型授权)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tenant_model_grants (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  TEXT        NOT NULL,
  model_id   UUID        NOT NULL,
  status     TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_tenant_model_grants_tenant
  ON ai_tenant_model_grants (tenant_id, status);

-- ============================================================================
-- AI User Model Grants (用户级模型授权，必须是租户授权的子集)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_user_model_grants (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  TEXT        NOT NULL,
  user_id    TEXT        NOT NULL,
  model_id   UUID        NOT NULL,
  status     TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, user_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_user_model_grants_lookup
  ON ai_user_model_grants (tenant_id, user_id, status);

-- ============================================================================
-- AI Usage Logs (请求明细日志)
-- provider_format: 实际使用的上游协议（同 upstream_protocol）
-- cache_write / cache_read / reasoning tokens 独立记录，计费按 input 价格
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_usage_logs (
  id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id             TEXT        NOT NULL UNIQUE,
  trace_id               TEXT,
  api_key_id             UUID,
  key_owner_type         TEXT        NOT NULL,
  auth_method            TEXT        NOT NULL DEFAULT 'api_key' CHECK (auth_method IN ('api_key', 'jwt')),
  request_source         TEXT        NOT NULL DEFAULT 'api_key' CHECK (request_source IN ('api_key', 'web_chat')),
  tenant_id              TEXT        NOT NULL,
  user_id                TEXT,
  external_user_id       TEXT,
  model_id               UUID,
  model_code             TEXT        NOT NULL,
  capability_type        TEXT        NOT NULL DEFAULT 'chat',
  model_route_id         UUID,
  upstream_deployment_id UUID,
  endpoint_id            UUID,
  provider_code          TEXT,
  upstream_model         TEXT,
  provider_format        TEXT,
  conversation_id        TEXT,
  stream                 BOOLEAN     NOT NULL DEFAULT false,
  prompt_tokens          INTEGER     NOT NULL DEFAULT 0,
  completion_tokens      INTEGER     NOT NULL DEFAULT 0,
  cache_write_tokens     INTEGER     NOT NULL DEFAULT 0,
  cache_read_tokens      INTEGER     NOT NULL DEFAULT 0,
  reasoning_tokens       INTEGER     NOT NULL DEFAULT 0,
  total_tokens           INTEGER     NOT NULL DEFAULT 0,
  billable_unit_type     TEXT        NOT NULL DEFAULT 'token',
  billable_units         BIGINT      NOT NULL DEFAULT 0,
  -- 自 2026-05 起，以下 *_cost 列单位均为「微积分」(micro-credits)。
  -- 1 积分 = 10000 微积分 = 1 分人民币。报表展示时除以 10000 即得小数积分。
  provider_cost          BIGINT      NOT NULL DEFAULT 0,
  platform_cost          BIGINT      NOT NULL DEFAULT 0,
  user_cost              BIGINT      NOT NULL DEFAULT 0,
  api_key_quota_cost     BIGINT      NOT NULL DEFAULT 0,
  urm_transaction_id     TEXT,
  -- Phase 2 起：分账层聚合扣款后回填这两列，关联到 URM bill_events.event_id
  settled_event_id       TEXT,
  settled_at             TIMESTAMPTZ,
  billing_status         TEXT        NOT NULL,
  request_status         TEXT        NOT NULL,
  http_status            INTEGER,
  upstream_status        INTEGER,
  latency_ms             INTEGER,
  first_token_latency_ms INTEGER,
  error_code             TEXT,
  error_message          TEXT,
  oauth_credential_id    UUID,
  credential_pool_id     UUID,
  attempts_count         INT         NOT NULL DEFAULT 1,
  final_route_id         UUID,
  client_protocol        TEXT        NOT NULL DEFAULT 'openai_chat',
  resolution             TEXT,
  usage_estimated        BOOLEAN     NOT NULL DEFAULT false,
  token_usage_source     TEXT        NOT NULL DEFAULT 'upstream',
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (billable_unit_type IN ('token', 'input_token', 'output_token', 'image', 'second', 'request')),
  CONSTRAINT ai_usage_logs_nonnegative CHECK (
    prompt_tokens >= 0 AND completion_tokens >= 0
    AND cache_write_tokens >= 0 AND cache_read_tokens >= 0 AND reasoning_tokens >= 0
    AND total_tokens >= 0 AND billable_units >= 0
    AND provider_cost >= 0 AND platform_cost >= 0
    AND user_cost >= 0 AND api_key_quota_cost >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_tenant_time       ON ai_usage_logs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_user_time         ON ai_usage_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_key_time          ON ai_usage_logs (api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_model_time        ON ai_usage_logs (model_code, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_source_time       ON ai_usage_logs (request_source, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_urm_transaction   ON ai_usage_logs (urm_transaction_id);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_route             ON ai_usage_logs (model_route_id);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_deployment        ON ai_usage_logs (upstream_deployment_id);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_tenant_error_time ON ai_usage_logs (tenant_id, created_at DESC)
  WHERE request_status = 'failed';
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_oauth_cred ON ai_usage_logs (oauth_credential_id)
  WHERE oauth_credential_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_pool ON ai_usage_logs (credential_pool_id)
  WHERE credential_pool_id IS NOT NULL;

-- ============================================================================
-- AI Usage Hourly Rollups (小时级预聚合，含 cache token 字段)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_usage_rollups_hourly (
  bucket_start           TIMESTAMPTZ NOT NULL,
  tenant_id              TEXT        NOT NULL,
  user_id                TEXT        NOT NULL DEFAULT '',
  api_key_id             UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  request_source         TEXT        NOT NULL DEFAULT 'api_key',
  capability_type        TEXT        NOT NULL DEFAULT 'chat',
  model_code             TEXT        NOT NULL,
  provider_code          TEXT        NOT NULL DEFAULT '',
  request_status         TEXT        NOT NULL,
  billable_unit_type     TEXT        NOT NULL,
  request_count          BIGINT      NOT NULL DEFAULT 0,
  success_count          BIGINT      NOT NULL DEFAULT 0,
  failed_count           BIGINT      NOT NULL DEFAULT 0,
  prompt_tokens          BIGINT      NOT NULL DEFAULT 0,
  completion_tokens      BIGINT      NOT NULL DEFAULT 0,
  cache_write_tokens     BIGINT      NOT NULL DEFAULT 0,
  cache_read_tokens      BIGINT      NOT NULL DEFAULT 0,
  reasoning_tokens       BIGINT      NOT NULL DEFAULT 0,
  total_tokens           BIGINT      NOT NULL DEFAULT 0,
  billable_units         BIGINT      NOT NULL DEFAULT 0,
  provider_cost          BIGINT      NOT NULL DEFAULT 0,
  platform_cost          BIGINT      NOT NULL DEFAULT 0,
  user_cost              BIGINT      NOT NULL DEFAULT 0,
  api_key_quota_cost     BIGINT      NOT NULL DEFAULT 0,
  latency_success_sum_ms BIGINT      NOT NULL DEFAULT 0,
  latency_success_count  BIGINT      NOT NULL DEFAULT 0,
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (
    bucket_start, tenant_id, user_id, api_key_id, request_source,
    model_code, provider_code, request_status, billable_unit_type
  ),
  CHECK (request_source IN ('api_key', 'web_chat')),
  CHECK (billable_unit_type IN ('token', 'input_token', 'output_token', 'image', 'second', 'request'))
);

CREATE INDEX IF NOT EXISTS idx_ai_usage_rollups_hourly_time
  ON ai_usage_rollups_hourly (bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_rollups_hourly_tenant_time
  ON ai_usage_rollups_hourly (tenant_id, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_rollups_hourly_tenant_user_time
  ON ai_usage_rollups_hourly (tenant_id, user_id, bucket_start DESC);

-- ============================================================================
-- AI Runtime Limit Policies (限流策略，支持 tenant/user/api_key/provider/endpoint 粒度)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_runtime_limit_policies (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  scope_type        TEXT        NOT NULL CHECK (scope_type IN ('tenant', 'user', 'api_key', 'provider', 'endpoint')),
  scope_id          TEXT        NOT NULL,
  capability_type   TEXT        NOT NULL DEFAULT 'chat' CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank')),
  model_code        TEXT,
  rpm_limit         INTEGER     CHECK (rpm_limit IS NULL OR rpm_limit > 0),
  tpm_limit         INTEGER     CHECK (tpm_limit IS NULL OR tpm_limit > 0),
  concurrency_limit INTEGER     CHECK (concurrency_limit IS NULL OR concurrency_limit > 0),
  status            TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_by        TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_runtime_limit_policies_lookup
  ON ai_runtime_limit_policies (scope_type, scope_id, capability_type, model_code, status);

-- ============================================================================
-- AI Conversation Bindings (会话粘性路由)
-- 保证同 conversation_id 的请求路由到同一 upstream deployment
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_conversation_bindings (
  conversation_id        TEXT        NOT NULL,
  tenant_id              TEXT        NOT NULL,
  identity_id            TEXT        NOT NULL,
  model_id               UUID        NOT NULL,
  upstream_deployment_id UUID        NOT NULL,
  endpoint_id            UUID        NOT NULL,
  expires_at             TIMESTAMPTZ NOT NULL,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (conversation_id, tenant_id, identity_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_conversation_bindings_expires ON ai_conversation_bindings (expires_at);

-- ============================================================================
-- AI Async Tasks (异步任务队列：视频生成、批量推理等)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_async_tasks (
  id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  task_type          TEXT        NOT NULL,
  tenant_id          TEXT        NOT NULL,
  user_id            TEXT,
  api_key_id         UUID,
  model_code         TEXT        NOT NULL,
  input_payload      JSONB       NOT NULL,
  status             TEXT        NOT NULL DEFAULT 'pending',
  result_payload     JSONB,
  error_code         TEXT,
  error_message      TEXT,
  urm_transaction_id TEXT,
  estimated_cost     BIGINT      NOT NULL DEFAULT 0,
  actual_cost        BIGINT      NOT NULL DEFAULT 0,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at         TIMESTAMPTZ,
  completed_at       TIMESTAMPTZ,
  expires_at         TIMESTAMPTZ,
  CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
  CONSTRAINT ai_async_tasks_cost_nonnegative CHECK (estimated_cost >= 0 AND actual_cost >= 0)
);

CREATE INDEX IF NOT EXISTS idx_ai_async_tasks_tenant_status ON ai_async_tasks (tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_async_tasks_pending       ON ai_async_tasks (status, created_at ASC)
  WHERE status = 'pending';

-- ============================================================================
-- AI Admin Audit Logs (管理操作审计)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_admin_audit_logs (
  id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  actor           TEXT,
  actor_tenant_id TEXT,
  action          TEXT        NOT NULL,
  object_type     TEXT,
  object_id       TEXT,
  request_summary JSONB       NOT NULL DEFAULT '{}',
  result          TEXT        NOT NULL,
  http_status     INTEGER,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_admin_audit_logs_time   ON ai_admin_audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_admin_audit_logs_object ON ai_admin_audit_logs (object_type, object_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_admin_audit_logs_actor  ON ai_admin_audit_logs (actor, created_at DESC);

-- ============================================================================
-- AI Route Score Weights (多维评分路由权重配置)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_route_score_weights (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  scope      TEXT        NOT NULL,   -- 'global' | 'tenant:<id>' | 'model:<id>'
  weights    JSONB       NOT NULL,   -- {"cost":0.4,"latency":0.3,"load":0.2,"health":0.1}
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (scope)
);

INSERT INTO ai_route_score_weights (scope, weights)
  VALUES ('global', '{"cost":0.4,"latency":0.3,"load":0.2,"health":0.1}'::jsonb)
  ON CONFLICT (scope) DO NOTHING;

-- ============================================================================
-- AI Request Payloads v3 (普通表，业务无感知；归档由存储过程 + 定时任务处理)
-- 只存 usage_log 没有的 wire 层信息 + 正文 JSONB + 早期失败自带的结果字段。
-- 归档策略：每月将 6 个月前的数据搬到 ai_request_payloads_archive_YYYY_MM，
--           然后从主表删除，保持主表始终只保留近期数据。
-- ============================================================================

CREATE TABLE IF NOT EXISTS ai_request_payloads (
  id               UUID        NOT NULL DEFAULT gen_random_uuid(),
  request_id       TEXT        NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  client_protocol  TEXT        NOT NULL,
  client_ip        TEXT,
  user_agent       TEXT,
  request_path     TEXT        NOT NULL,
  auth_masked      TEXT,
  request_model    TEXT        NOT NULL,
  request_messages JSONB,
  request_params   JSONB,
  response_message JSONB,
  media_refs       JSONB,
  request_status   TEXT        NOT NULL,
  http_status      INT,
  error_code       TEXT,
  PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_arp_request_id ON ai_request_payloads (request_id);
CREATE INDEX IF NOT EXISTS idx_arp_created_at ON ai_request_payloads (created_at);

-- ============================================================================
-- 归档存储过程：将 N 个月前的数据搬到归档表，然后从主表删除。
-- 归档表命名格式：ai_request_payloads_archive_YYYY_MM
-- 用法：SELECT archive_request_payloads(6);  -- 保留最近 6 个月
-- ============================================================================
CREATE OR REPLACE FUNCTION archive_request_payloads(retain_months INT DEFAULT 6)
RETURNS TABLE(archived_table TEXT, rows_moved BIGINT) AS $$
DECLARE
  cutoff        TIMESTAMPTZ;
  archive_name  TEXT;
  moved_count   BIGINT;
BEGIN
  -- 算出归档分界点：保留 retain_months 个月的数据
  cutoff := date_trunc('month', NOW() - (retain_months || ' months')::INTERVAL);

  -- 如果没有需要归档的数据，直接返回
  IF NOT EXISTS (SELECT 1 FROM ai_request_payloads WHERE created_at < cutoff) THEN
    archived_table := NULL;
    rows_moved := 0;
    RETURN NEXT;
    RETURN;
  END IF;

  -- 逐月归档：找出所有早于 cutoff 的月份
  FOR archive_name IN
    SELECT DISTINCT 'ai_request_payloads_archive_' || to_char(date_trunc('month', created_at), 'YYYY_MM')
    FROM ai_request_payloads
    WHERE created_at < cutoff
    ORDER BY 1
  LOOP
    -- 提取月份用于过滤
    DECLARE
      month_start  TIMESTAMPTZ;
      month_end    TIMESTAMPTZ;
      month_label  TEXT;
    BEGIN
      -- 从归档表名解析月份，如 'ai_request_payloads_archive_2026_01' → '2026_01'
      month_label := substring(archive_name from '(\d{4}_\d{2})$');
      month_start := to_timestamp(replace(month_label, '_', '-'), 'YYYY-MM')::TIMESTAMPTZ;
      month_end   := month_start + INTERVAL '1 month';

      -- 创建归档表（如果不存在），只继承默认值，不继承索引
      EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I (LIKE ai_request_payloads INCLUDING DEFAULTS)',
        archive_name
      );

      -- 搬数据
      EXECUTE format(
        'INSERT INTO %I SELECT * FROM ai_request_payloads WHERE created_at >= %L AND created_at < %L',
        archive_name, month_start, month_end
      );

      GET DIAGNOSTICS moved_count = ROW_COUNT;

      -- 从主表删除已归档的数据
      DELETE FROM ai_request_payloads WHERE created_at >= month_start AND created_at < month_end;

      -- 返回本次归档结果
      archived_table := archive_name;
      rows_moved := moved_count;
      RETURN NEXT;
    END;
  END LOOP;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- AI Audit Blobs (content-addressed media store; sha256 dedup via ON CONFLICT)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_audit_blobs (
  sha256       TEXT        PRIMARY KEY,
  content      BYTEA       NOT NULL,
  content_type TEXT        NOT NULL,
  size_bytes   INT         NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- Local Credit Ledger (Phase 2, 2026-05)
-- 每 (owner_type, tenant_id, user_id) 一行的本地账本。
-- pending_micro 累计未结算微积分，达到阈值或定时器触发时聚合调 URM Consume。
-- settle_window_id 是当前结算窗口的 ULID 幂等键。
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_user_credit_ledger (
  id                          BIGSERIAL    PRIMARY KEY,
  owner_type                  TEXT         NOT NULL CHECK (owner_type IN ('user', 'tenant')),
  tenant_id                   TEXT         NOT NULL,
  user_id                     TEXT         NOT NULL DEFAULT '',
  pending_tenant_micro        BIGINT       NOT NULL DEFAULT 0 CHECK (pending_tenant_micro >= 0),
  pending_user_micro          BIGINT       NOT NULL DEFAULT 0 CHECK (pending_user_micro >= 0),
  settled_tenant_micro        BIGINT       NOT NULL DEFAULT 0 CHECK (settled_tenant_micro >= 0),
  settled_user_micro          BIGINT       NOT NULL DEFAULT 0 CHECK (settled_user_micro >= 0),
  settle_window_id            TEXT,
  settle_window_tenant_micro  BIGINT       NOT NULL DEFAULT 0 CHECK (settle_window_tenant_micro >= 0),
  settle_window_user_micro    BIGINT       NOT NULL DEFAULT 0 CHECK (settle_window_user_micro >= 0),
  settle_window_opened_at     TIMESTAMPTZ,
  last_settled_at             TIMESTAMPTZ,
  created_at                  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at                  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  CONSTRAINT ai_user_credit_ledger_unique UNIQUE (owner_type, tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_user_credit_ledger_pending
  ON ai_user_credit_ledger (last_settled_at NULLS FIRST)
  WHERE (pending_tenant_micro + pending_user_micro) > 0;

CREATE INDEX IF NOT EXISTS idx_ai_user_credit_ledger_in_flight
  ON ai_user_credit_ledger (settle_window_opened_at)
  WHERE settle_window_id IS NOT NULL;

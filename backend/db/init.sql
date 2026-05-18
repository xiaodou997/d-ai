-- ============================================================================
-- Uni AI API 数据库初始化脚本
-- 可直接在 Navicat、DBeaver 等数据库工具中执行
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================================
-- AI API Keys
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_api_keys (
  id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type     TEXT        NOT NULL CHECK (owner_type IN ('user', 'tenant')),
  tenant_id      TEXT        NOT NULL,
  user_id        TEXT,
  key_hash       TEXT        NOT NULL UNIQUE,
  key_prefix     TEXT        NOT NULL,
  name           TEXT        NOT NULL,
  quota_limit    BIGINT,
  quota_used     BIGINT      NOT NULL DEFAULT 0,
  quota_reserved BIGINT      NOT NULL DEFAULT 0,
  allowed_models JSONB       NOT NULL DEFAULT '[]',
  status         TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'expired')),
  expires_at     TIMESTAMPTZ,
  created_by     TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((owner_type = 'tenant' AND user_id IS NULL) OR (owner_type = 'user' AND user_id IS NOT NULL)),
  CONSTRAINT ai_api_keys_quota_nonnegative CHECK (
    quota_used >= 0 AND quota_reserved >= 0
    AND (quota_limit IS NULL OR quota_limit >= 0)
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_api_keys_tenant ON ai_api_keys (tenant_id);
CREATE INDEX IF NOT EXISTS idx_ai_api_keys_user   ON ai_api_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_ai_api_keys_status ON ai_api_keys (status);

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
  provider_id        UUID        NOT NULL REFERENCES ai_providers (id),
  name               TEXT        NOT NULL,
  base_url           TEXT        NOT NULL,
  api_key_ciphertext TEXT        NOT NULL,
  extra_headers      JSONB       NOT NULL DEFAULT '{}',
  weight             INTEGER     NOT NULL DEFAULT 100 CHECK (weight >= 0),
  timeout_ms         INTEGER     NOT NULL DEFAULT 30000 CHECK (timeout_ms > 0),
  default_protocol   TEXT        NOT NULL DEFAULT 'openai_compatible' CHECK (default_protocol IN ('openai_compatible', 'anthropic', 'gemini')),
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
  pool_id                  UUID        NOT NULL REFERENCES ai_credential_pools (id) ON DELETE CASCADE,
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
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_upstream_deployments (
  id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  endpoint_id          UUID        NOT NULL REFERENCES ai_provider_endpoints (id),
  name                 TEXT        NOT NULL,
  upstream_model       TEXT        NOT NULL,
  capability_type      TEXT        NOT NULL DEFAULT 'chat',
  upstream_protocol    TEXT        NOT NULL DEFAULT 'openai_chat',
  request_path         TEXT,
  upstream_parameters  JSONB       NOT NULL DEFAULT '{}',
  tags                 JSONB       NOT NULL DEFAULT '{}',
  health_status        TEXT        NOT NULL DEFAULT 'unknown',
  last_health_check_at TIMESTAMPTZ,
  last_health_error    TEXT,
  status               TEXT        NOT NULL DEFAULT 'active',
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
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
  UNIQUE (endpoint_id, upstream_model, upstream_protocol)
);

CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_endpoint   ON ai_upstream_deployments (endpoint_id);
CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_capability ON ai_upstream_deployments (capability_type, status);
CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_health     ON ai_upstream_deployments (health_status, status);

-- ============================================================================
-- AI Models (对外公开模型)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_models (
  id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  model_code                TEXT        NOT NULL UNIQUE,
  display_name              TEXT        NOT NULL,
  capability_type           TEXT        NOT NULL DEFAULT 'chat',
  context_window            INTEGER,
  default_max_output_tokens INTEGER     NOT NULL DEFAULT 2048,
  max_output_tokens         INTEGER,
  status                    TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank'))
);

CREATE INDEX IF NOT EXISTS idx_ai_models_status     ON ai_models (status);
CREATE INDEX IF NOT EXISTS idx_ai_models_capability ON ai_models (capability_type, status);

-- ============================================================================
-- AI Model Routes (模型 → 上游部署 路由映射)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_model_routes (
  id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id               UUID        NOT NULL REFERENCES ai_models (id),
  -- Deployment-based route (API Key 型上游)
  upstream_deployment_id UUID        REFERENCES ai_upstream_deployments (id),
  -- Pool-based route (OAuth Fixed Provider)
  credential_pool_id     UUID        REFERENCES ai_credential_pools (id),
  pool_upstream_model    TEXT,       -- 打到上游的模型名（pool 路由时必填）
  priority               INTEGER     NOT NULL DEFAULT 100 CHECK (priority >= 0),
  weight                 INTEGER     NOT NULL DEFAULT 100 CHECK (weight >= 0),
  supports_stream        BOOLEAN     NOT NULL DEFAULT true,
  status                 TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- 两种路由 target 互斥
  CONSTRAINT route_target_xor CHECK (
    (upstream_deployment_id IS NOT NULL AND credential_pool_id IS NULL)
    OR
    (credential_pool_id IS NOT NULL AND pool_upstream_model IS NOT NULL AND upstream_deployment_id IS NULL)
  ),
  UNIQUE NULLS NOT DISTINCT (model_id, upstream_deployment_id),
  UNIQUE NULLS NOT DISTINCT (model_id, credential_pool_id, pool_upstream_model)
);

CREATE INDEX IF NOT EXISTS idx_ai_model_routes_model      ON ai_model_routes (model_id, status);
CREATE INDEX IF NOT EXISTS idx_ai_model_routes_deployment ON ai_model_routes (upstream_deployment_id) WHERE upstream_deployment_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ai_model_routes_pool       ON ai_model_routes (credential_pool_id) WHERE credential_pool_id IS NOT NULL;

-- ============================================================================
-- AI Model Prices (平台对外售价)
-- cache_write / cache_read / reasoning 价格列预留：默认 0 表示按 input_price 计费。
-- 盈利点：provider 对 cache_read 打折，平台按 input 原价向客户收取。
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_model_prices (
  id                           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id                     UUID        NOT NULL UNIQUE REFERENCES ai_models (id),
  input_price_per_1m           BIGINT      NOT NULL DEFAULT 0,
  output_price_per_1m          BIGINT      NOT NULL DEFAULT 0,
  cache_write_price_per_1m     BIGINT      NOT NULL DEFAULT 0,
  cache_read_price_per_1m      BIGINT      NOT NULL DEFAULT 0,
  reasoning_price_per_1m       BIGINT      NOT NULL DEFAULT 0,
  image_size_prices            JSONB       NOT NULL DEFAULT '{}',
  video_price_per_second       BIGINT      NOT NULL DEFAULT 0,
  audio_tts_price_per_1m_chars BIGINT      NOT NULL DEFAULT 0,
  audio_stt_price_per_minute   BIGINT      NOT NULL DEFAULT 0,
  created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ai_model_prices_nonnegative CHECK (
    input_price_per_1m >= 0 AND output_price_per_1m >= 0
    AND cache_write_price_per_1m >= 0 AND cache_read_price_per_1m >= 0
    AND reasoning_price_per_1m >= 0
    AND video_price_per_second >= 0
    AND audio_tts_price_per_1m_chars >= 0 AND audio_stt_price_per_minute >= 0
  )
);

-- ============================================================================
-- AI Tenant Model Price Overrides (平台给租户的特殊定价)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tenant_model_price_overrides (
  id                           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id                    TEXT        NOT NULL,
  model_id                     UUID        NOT NULL REFERENCES ai_models (id),
  input_price_per_1m           BIGINT      NOT NULL DEFAULT 0,
  output_price_per_1m          BIGINT      NOT NULL DEFAULT 0,
  cache_write_price_per_1m     BIGINT      NOT NULL DEFAULT 0,
  cache_read_price_per_1m      BIGINT      NOT NULL DEFAULT 0,
  reasoning_price_per_1m       BIGINT      NOT NULL DEFAULT 0,
  image_size_prices            JSONB       NOT NULL DEFAULT '{}',
  video_price_per_second       BIGINT      NOT NULL DEFAULT 0,
  audio_tts_price_per_1m_chars BIGINT      NOT NULL DEFAULT 0,
  audio_stt_price_per_minute   BIGINT      NOT NULL DEFAULT 0,
  created_by                   TEXT,
  created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id),
  CONSTRAINT ai_tenant_model_price_overrides_nonnegative CHECK (
    input_price_per_1m >= 0 AND output_price_per_1m >= 0
    AND cache_write_price_per_1m >= 0 AND cache_read_price_per_1m >= 0
    AND reasoning_price_per_1m >= 0
    AND video_price_per_second >= 0
    AND audio_tts_price_per_1m_chars >= 0 AND audio_stt_price_per_minute >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_tenant_model_price_overrides_lookup
  ON ai_tenant_model_price_overrides (tenant_id, model_id);

-- ============================================================================
-- AI Tenant User Prices (租户对其用户的定价)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tenant_user_prices (
  id                           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id                    TEXT        NOT NULL,
  model_id                     UUID        NOT NULL REFERENCES ai_models (id),
  input_price_per_1m           BIGINT      NOT NULL DEFAULT 0,
  output_price_per_1m          BIGINT      NOT NULL DEFAULT 0,
  cache_write_price_per_1m     BIGINT      NOT NULL DEFAULT 0,
  cache_read_price_per_1m      BIGINT      NOT NULL DEFAULT 0,
  reasoning_price_per_1m       BIGINT      NOT NULL DEFAULT 0,
  image_size_prices            JSONB       NOT NULL DEFAULT '{}',
  video_price_per_second       BIGINT      NOT NULL DEFAULT 0,
  audio_tts_price_per_1m_chars BIGINT      NOT NULL DEFAULT 0,
  audio_stt_price_per_minute   BIGINT      NOT NULL DEFAULT 0,
  created_by                   TEXT,
  created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id),
  CONSTRAINT ai_tenant_user_prices_nonnegative CHECK (
    input_price_per_1m >= 0 AND output_price_per_1m >= 0
    AND cache_write_price_per_1m >= 0 AND cache_read_price_per_1m >= 0
    AND reasoning_price_per_1m >= 0
    AND video_price_per_second >= 0 AND audio_tts_price_per_1m_chars >= 0
    AND audio_stt_price_per_minute >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_tenant_user_prices_lookup
  ON ai_tenant_user_prices (tenant_id, model_id);

-- ============================================================================
-- AI Upstream Deployment Cost Prices (上游成本价，用于利润核算)
-- cache_read_cost 远低于 input_cost，差价是平台盈利来源
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_upstream_deployment_cost_prices (
  id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  upstream_deployment_id  UUID        NOT NULL REFERENCES ai_upstream_deployments (id),
  capability_type         TEXT        NOT NULL DEFAULT 'chat',
  currency                TEXT        NOT NULL DEFAULT 'CNY_CREDITS',
  input_cost_per_1m       BIGINT      NOT NULL DEFAULT 0,
  output_cost_per_1m      BIGINT      NOT NULL DEFAULT 0,
  cache_write_cost_per_1m BIGINT      NOT NULL DEFAULT 0,
  cache_read_cost_per_1m  BIGINT      NOT NULL DEFAULT 0,
  reasoning_cost_per_1m   BIGINT      NOT NULL DEFAULT 0,
  request_cost            BIGINT      NOT NULL DEFAULT 0,
  image_cost              BIGINT      NOT NULL DEFAULT 0,
  image_size_prices       JSONB       NOT NULL DEFAULT '{}',
  video_cost_per_second   BIGINT      NOT NULL DEFAULT 0,
  effective_from          TIMESTAMPTZ NOT NULL DEFAULT now(),
  status                  TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank')),
  CONSTRAINT ai_upstream_deployment_cost_prices_nonnegative CHECK (
    input_cost_per_1m >= 0 AND output_cost_per_1m >= 0
    AND cache_write_cost_per_1m >= 0 AND cache_read_cost_per_1m >= 0
    AND reasoning_cost_per_1m >= 0
    AND request_cost >= 0 AND image_cost >= 0 AND video_cost_per_second >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployment_cost_prices_lookup
  ON ai_upstream_deployment_cost_prices (upstream_deployment_id, status, effective_from DESC);

-- ============================================================================
-- AI Tenant Model Grants (租户级模型授权)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tenant_model_grants (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  TEXT        NOT NULL,
  model_id   UUID        NOT NULL REFERENCES ai_models (id),
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
  model_id   UUID        NOT NULL REFERENCES ai_models (id),
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
  tenant_id              TEXT        NOT NULL,
  user_id                TEXT,
  external_user_id       TEXT,
  model_id               UUID,
  model_code             TEXT        NOT NULL,
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
  provider_cost          BIGINT      NOT NULL DEFAULT 0,
  platform_cost          BIGINT      NOT NULL DEFAULT 0,
  user_cost              BIGINT      NOT NULL DEFAULT 0,
  api_key_quota_cost     BIGINT      NOT NULL DEFAULT 0,
  urm_transaction_id     TEXT,
  billing_status         TEXT        NOT NULL,
  request_status         TEXT        NOT NULL,
  http_status            INTEGER,
  upstream_status        INTEGER,
  latency_ms             INTEGER,
  first_token_latency_ms INTEGER,
  error_code             TEXT,
  error_message          TEXT,
  oauth_credential_id    UUID        REFERENCES ai_provider_oauth_credentials (id),
  credential_pool_id     UUID        REFERENCES ai_credential_pools (id),
  usage_estimated        BOOLEAN     NOT NULL DEFAULT false,
  usage_source           TEXT        NOT NULL DEFAULT 'upstream',
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
    bucket_start, tenant_id, user_id, api_key_id,
    model_code, provider_code, request_status, billable_unit_type
  ),
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
  upstream_deployment_id UUID        NOT NULL REFERENCES ai_upstream_deployments (id),
  endpoint_id            UUID        NOT NULL REFERENCES ai_provider_endpoints (id),
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
-- P3: 多维评分路由 (route scorer weights + per-route cost hint)
-- ============================================================================

-- Route-level scoring cost hint (optional; 0 = Pool/free, >0 = paid upstream)
ALTER TABLE ai_model_routes
  ADD COLUMN IF NOT EXISTS cost_per_1k_tokens    NUMERIC(10,6) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS score_weights_override JSONB;

-- Global (and future per-tenant/per-model) scorer weight config
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
-- P4: Sticky 路由 + 可观测 + Payload 落盘
-- ============================================================================

-- Sticky routing: allow per-route opt-out and per-pool granularity override.
ALTER TABLE ai_model_routes
  ADD COLUMN IF NOT EXISTS sticky_enabled BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE ai_credential_pools
  ADD COLUMN IF NOT EXISTS sticky_granularity TEXT NOT NULL DEFAULT 'credential'
    CHECK (sticky_granularity IN ('credential', 'pool'));

-- Usage log: multi-attempt metadata.
ALTER TABLE ai_usage_logs
  ADD COLUMN IF NOT EXISTS attempts_count  INT  NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS final_route_id  UUID,
  ADD COLUMN IF NOT EXISTS client_protocol TEXT NOT NULL DEFAULT 'openai_chat';

-- Request payload table: failed requests (required) + sampled successes.
-- upstream_body / raw_client_body are AES-GCM encrypted (BYTEA).
CREATE TABLE IF NOT EXISTS ai_request_payloads (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  usage_log_id      UUID        REFERENCES ai_usage_logs(id) ON DELETE CASCADE,
  upstream_body     BYTEA,
  upstream_response BYTEA,
  raw_client_body   BYTEA,
  route_attempts    JSONB       NOT NULL DEFAULT '[]',
  sampled           BOOLEAN     NOT NULL DEFAULT false,
  client_protocol   TEXT        NOT NULL DEFAULT 'openai_chat',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at        TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ai_request_payloads_expires    ON ai_request_payloads (expires_at);
CREATE INDEX IF NOT EXISTS idx_ai_request_payloads_usage_log  ON ai_request_payloads (usage_log_id);
CREATE INDEX IF NOT EXISTS idx_ai_request_payloads_created    ON ai_request_payloads (created_at DESC);

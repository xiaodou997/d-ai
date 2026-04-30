-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================================
-- AI API Keys
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type TEXT NOT NULL CHECK (owner_type IN ('user', 'tenant')),
  tenant_id TEXT NOT NULL,
  user_id TEXT,
  key_hash TEXT NOT NULL UNIQUE,
  key_prefix TEXT NOT NULL,
  name TEXT NOT NULL,
  quota_limit BIGINT,
  quota_used BIGINT NOT NULL DEFAULT 0,
  quota_reserved BIGINT NOT NULL DEFAULT 0,
  allowed_models JSONB NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'active',
  expires_at TIMESTAMPTZ,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((owner_type = 'tenant' AND user_id IS NULL) OR (owner_type = 'user' AND user_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_ai_api_keys_tenant ON ai_api_keys (tenant_id);
CREATE INDEX IF NOT EXISTS idx_ai_api_keys_user ON ai_api_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_ai_api_keys_status ON ai_api_keys (status);

-- ============================================================================
-- AI Providers (厂商)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================================
-- AI Provider Endpoints (接入点)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_provider_endpoints (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id UUID NOT NULL,
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key_ciphertext TEXT NOT NULL,
  extra_headers JSONB NOT NULL DEFAULT '{}',
  weight INTEGER NOT NULL DEFAULT 100,
  timeout_ms INTEGER NOT NULL DEFAULT 30000,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_provider_endpoints_provider ON ai_provider_endpoints (provider_id);
CREATE INDEX IF NOT EXISTS idx_ai_provider_endpoints_status ON ai_provider_endpoints (status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_provider_endpoints_provider_name ON ai_provider_endpoints (provider_id, name);

-- ============================================================================
-- AI Upstream Deployments (上游部署)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_upstream_deployments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  endpoint_id UUID NOT NULL,
  name TEXT NOT NULL,
  upstream_model TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  upstream_protocol TEXT NOT NULL DEFAULT 'openai_chat_completions',
  request_path TEXT,
  upstream_parameters JSONB NOT NULL DEFAULT '{}',
  tags JSONB NOT NULL DEFAULT '{}',
  health_status TEXT NOT NULL DEFAULT 'unknown',
  last_health_check_at TIMESTAMPTZ,
  last_health_error TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank')),
  CHECK (upstream_protocol IN ('openai_chat_completions', 'openai_images_generations', 'openai_responses', 'openai_embeddings', 'anthropic_messages')),
  CHECK (health_status IN ('healthy', 'unhealthy', 'unknown')),
  UNIQUE (endpoint_id, upstream_model, upstream_protocol)
);

CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_endpoint ON ai_upstream_deployments (endpoint_id);
CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_capability ON ai_upstream_deployments (capability_type, status);
CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_health ON ai_upstream_deployments (health_status, status);
CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_status ON ai_upstream_deployments (status);

-- ============================================================================
-- AI Models (对外模型 / Public Model)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_models (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_code TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  context_window INTEGER,
  default_max_output_tokens INTEGER NOT NULL DEFAULT 2048,
  max_output_tokens INTEGER,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank'))
);

CREATE INDEX IF NOT EXISTS idx_ai_models_status ON ai_models (status);
CREATE INDEX IF NOT EXISTS idx_ai_models_capability ON ai_models (capability_type, status);

-- ============================================================================
-- AI Model Routes (模型路由映射)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_model_routes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id UUID NOT NULL,
  upstream_deployment_id UUID NOT NULL,
  priority INTEGER NOT NULL DEFAULT 100,
  weight INTEGER NOT NULL DEFAULT 100,
  supports_stream BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (priority >= 0),
  CHECK (weight >= 0),
  UNIQUE (model_id, upstream_deployment_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_model_routes_model ON ai_model_routes (model_id, status);
CREATE INDEX IF NOT EXISTS idx_ai_model_routes_deployment ON ai_model_routes (upstream_deployment_id);
CREATE INDEX IF NOT EXISTS idx_ai_model_routes_priority ON ai_model_routes (priority, status);

-- ============================================================================
-- AI Model Prices (对外销售价，1:1 with ai_models)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_model_prices (
  id                           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id                     UUID NOT NULL UNIQUE,
  input_price_per_1m           BIGINT NOT NULL DEFAULT 0,
  output_price_per_1m          BIGINT NOT NULL DEFAULT 0,
  image_size_prices            JSONB  NOT NULL DEFAULT '{}',
  video_price_per_second       BIGINT NOT NULL DEFAULT 0,
  audio_tts_price_per_1m_chars BIGINT NOT NULL DEFAULT 0,
  audio_stt_price_per_minute   BIGINT NOT NULL DEFAULT 0,
  created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ai_model_prices_nonnegative CHECK (
    input_price_per_1m >= 0
    AND output_price_per_1m >= 0
    AND video_price_per_second >= 0
    AND audio_tts_price_per_1m_chars >= 0
    AND audio_stt_price_per_minute >= 0
  )
);

-- ============================================================================
-- AI Tenant Model Price Overrides (租户自定义价格)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tenant_model_price_overrides (
  id                           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id                    TEXT NOT NULL,
  model_id                     UUID NOT NULL,
  input_price_per_1m           BIGINT NOT NULL DEFAULT 0,
  output_price_per_1m          BIGINT NOT NULL DEFAULT 0,
  image_size_prices            JSONB  NOT NULL DEFAULT '{}',
  video_price_per_second       BIGINT NOT NULL DEFAULT 0,
  audio_tts_price_per_1m_chars BIGINT NOT NULL DEFAULT 0,
  audio_stt_price_per_minute   BIGINT NOT NULL DEFAULT 0,
  created_by                   TEXT,
  created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id),
  CONSTRAINT ai_tenant_model_price_overrides_nonnegative CHECK (
    input_price_per_1m >= 0
    AND output_price_per_1m >= 0
    AND video_price_per_second >= 0
    AND audio_tts_price_per_1m_chars >= 0
    AND audio_stt_price_per_minute >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_tenant_model_price_overrides_lookup
  ON ai_tenant_model_price_overrides (tenant_id, model_id);

-- ============================================================================
-- AI Tenant User Prices (租户售价 - 租户对用户的定价)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tenant_user_prices (
  id                           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id                    TEXT NOT NULL,
  model_id                     UUID NOT NULL,
  input_price_per_1m           BIGINT NOT NULL DEFAULT 0,
  output_price_per_1m          BIGINT NOT NULL DEFAULT 0,
  image_size_prices            JSONB  NOT NULL DEFAULT '{}',
  video_price_per_second       BIGINT NOT NULL DEFAULT 0,
  audio_tts_price_per_1m_chars BIGINT NOT NULL DEFAULT 0,
  audio_stt_price_per_minute   BIGINT NOT NULL DEFAULT 0,
  created_by                   TEXT,
  created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id),
  CONSTRAINT ai_tenant_user_prices_nonnegative CHECK (
    input_price_per_1m >= 0
    AND output_price_per_1m >= 0
    AND video_price_per_second >= 0
    AND audio_tts_price_per_1m_chars >= 0
    AND audio_stt_price_per_minute >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_tenant_user_prices_lookup
  ON ai_tenant_user_prices (tenant_id, model_id);

-- ============================================================================
-- AI Upstream Deployment Cost Prices (上游成本价)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_upstream_deployment_cost_prices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  upstream_deployment_id UUID NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  currency TEXT NOT NULL DEFAULT 'CNY_CREDITS',
  input_cost_per_1m BIGINT NOT NULL DEFAULT 0,
  output_cost_per_1m BIGINT NOT NULL DEFAULT 0,
  request_cost BIGINT NOT NULL DEFAULT 0,
  image_cost BIGINT NOT NULL DEFAULT 0,
  image_size_prices JSONB NOT NULL DEFAULT '{}',
  video_cost_per_second BIGINT NOT NULL DEFAULT 0,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank')),
  CONSTRAINT ai_upstream_deployment_cost_prices_nonnegative_credits CHECK (
    input_cost_per_1m >= 0
    AND output_cost_per_1m >= 0
    AND request_cost >= 0
    AND image_cost >= 0
    AND video_cost_per_second >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployment_cost_prices_lookup
  ON ai_upstream_deployment_cost_prices (upstream_deployment_id, status, effective_from DESC);

-- ============================================================================
-- AI Tenant Model Grants (租户模型授权)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tenant_model_grants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  model_id UUID NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_tenant_model_grants_tenant ON ai_tenant_model_grants (tenant_id, status);

-- ============================================================================
-- AI Usage Logs (使用日志)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_usage_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id TEXT NOT NULL UNIQUE,
  trace_id TEXT,
  api_key_id UUID,
  key_owner_type TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  user_id TEXT,
  external_user_id TEXT,
  model_id UUID,
  model_code TEXT NOT NULL,
  model_route_id UUID,
  upstream_deployment_id UUID,
  endpoint_id UUID,
  provider_code TEXT,
  upstream_model TEXT,
  conversation_id TEXT,
  stream BOOLEAN NOT NULL DEFAULT false,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  billable_unit_type TEXT NOT NULL DEFAULT 'token',
  billable_units BIGINT NOT NULL DEFAULT 0,
  provider_cost BIGINT NOT NULL DEFAULT 0,
  platform_cost BIGINT NOT NULL DEFAULT 0,
  user_cost BIGINT NOT NULL DEFAULT 0,
  api_key_quota_cost BIGINT NOT NULL DEFAULT 0,
  urm_transaction_id TEXT,
  billing_status TEXT NOT NULL,
  request_status TEXT NOT NULL,
  http_status INTEGER,
  upstream_status INTEGER,
  latency_ms INTEGER,
  first_token_latency_ms INTEGER,
  error_code TEXT,
  error_message TEXT,
  usage_estimated BOOLEAN NOT NULL DEFAULT false,
  usage_source TEXT NOT NULL DEFAULT 'upstream',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (billable_unit_type IN ('token', 'input_token', 'output_token', 'image', 'second', 'request')),
  CONSTRAINT ai_usage_logs_nonnegative_billing CHECK (
    billable_units >= 0
    AND provider_cost >= 0
    AND platform_cost >= 0
    AND user_cost >= 0
    AND api_key_quota_cost >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_tenant_time ON ai_usage_logs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_user_time ON ai_usage_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_key_time ON ai_usage_logs (api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_model_time ON ai_usage_logs (model_code, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_urm_transaction ON ai_usage_logs (urm_transaction_id);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_route ON ai_usage_logs (model_route_id);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_deployment ON ai_usage_logs (upstream_deployment_id);

-- ============================================================================
-- AI Runtime Limit Policies (限流策略)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_runtime_limit_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  scope_type TEXT NOT NULL CHECK (scope_type IN ('tenant', 'user', 'api_key', 'provider', 'endpoint')),
  scope_id TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat' CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio_tts', 'audio_stt', 'rerank')),
  model_code TEXT,
  rpm_limit INTEGER,
  tpm_limit INTEGER,
  concurrency_limit INTEGER,
  status TEXT NOT NULL DEFAULT 'active',
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (rpm_limit IS NULL OR rpm_limit > 0),
  CHECK (tpm_limit IS NULL OR tpm_limit > 0),
  CHECK (concurrency_limit IS NULL OR concurrency_limit > 0)
);

CREATE INDEX IF NOT EXISTS idx_ai_runtime_limit_policies_lookup
  ON ai_runtime_limit_policies (scope_type, scope_id, capability_type, model_code, status);

-- ============================================================================
-- AI Admin Audit Logs (审计日志)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_admin_audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor TEXT,
  action TEXT NOT NULL,
  object_type TEXT,
  object_id TEXT,
  request_summary JSONB NOT NULL DEFAULT '{}',
  result TEXT NOT NULL,
  http_status INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_admin_audit_logs_time ON ai_admin_audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_admin_audit_logs_object ON ai_admin_audit_logs (object_type, object_id, created_at DESC);

-- ============================================================================
-- AI Conversation Bindings (会话粘性路由)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_conversation_bindings (
  conversation_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  identity_id TEXT NOT NULL,
  model_id UUID NOT NULL,
  upstream_deployment_id UUID NOT NULL,
  endpoint_id UUID NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (conversation_id, tenant_id, identity_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_conversation_bindings_expires ON ai_conversation_bindings (expires_at);

-- +goose Down
DROP TABLE IF EXISTS ai_conversation_bindings;
DROP TABLE IF EXISTS ai_admin_audit_logs;
DROP TABLE IF EXISTS ai_runtime_limit_policies;
DROP TABLE IF EXISTS ai_usage_logs;
DROP TABLE IF EXISTS ai_tenant_model_grants;
DROP TABLE IF EXISTS ai_tenant_model_price_overrides;
DROP TABLE IF EXISTS ai_upstream_deployment_cost_prices;
DROP TABLE IF EXISTS ai_model_prices;
DROP TABLE IF EXISTS ai_model_routes;
DROP TABLE IF EXISTS ai_models;
DROP TABLE IF EXISTS ai_upstream_deployments;
DROP TABLE IF EXISTS ai_provider_endpoints;
DROP TABLE IF EXISTS ai_providers;
DROP TABLE IF EXISTS ai_api_keys;
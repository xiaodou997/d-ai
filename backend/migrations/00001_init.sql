-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

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

CREATE TABLE IF NOT EXISTS ai_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  provider_type TEXT NOT NULL,
  is_custom BOOLEAN NOT NULL DEFAULT false,
  config JSONB NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (provider_type IN ('official', 'compatible', 'private', 'custom'))
);

CREATE TABLE IF NOT EXISTS ai_provider_endpoints (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id UUID NOT NULL REFERENCES ai_providers(id),
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  protocol_type TEXT NOT NULL DEFAULT 'openai_chat_completions',
  api_key_ciphertext TEXT NOT NULL,
  extra_headers JSONB NOT NULL DEFAULT '{}',
  custom_path TEXT,
  protocol_overrides JSONB NOT NULL DEFAULT '{}',
  weight INTEGER NOT NULL DEFAULT 100,
  timeout_ms INTEGER NOT NULL DEFAULT 30000,
  status TEXT NOT NULL DEFAULT 'active',
  health_status TEXT NOT NULL DEFAULT 'unknown',
  last_health_check_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (protocol_type IN ('openai_chat_completions', 'openai_images_generations', 'openai_responses', 'openai_embeddings', 'anthropic_messages'))
);

CREATE INDEX IF NOT EXISTS idx_ai_provider_endpoints_provider ON ai_provider_endpoints (provider_id);
CREATE INDEX IF NOT EXISTS idx_ai_provider_endpoints_status ON ai_provider_endpoints (status, health_status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_provider_endpoints_provider_name ON ai_provider_endpoints (provider_id, name);

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
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio', 'rerank'))
);

CREATE INDEX IF NOT EXISTS idx_ai_models_status ON ai_models (status);
CREATE INDEX IF NOT EXISTS idx_ai_models_capability ON ai_models (capability_type, status);

CREATE TABLE IF NOT EXISTS ai_model_deployments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id UUID NOT NULL REFERENCES ai_models(id),
  endpoint_id UUID NOT NULL REFERENCES ai_provider_endpoints(id),
  upstream_model TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  upstream_protocol TEXT NOT NULL DEFAULT 'openai_chat_completions',
  upstream_parameters JSONB NOT NULL DEFAULT '{}',
  priority INTEGER NOT NULL DEFAULT 100,
  weight INTEGER NOT NULL DEFAULT 100,
  supports_stream BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio', 'rerank')),
  CHECK (upstream_protocol IN ('openai_chat_completions', 'openai_images_generations', 'openai_responses', 'openai_embeddings', 'anthropic_messages')),
  UNIQUE (model_id, endpoint_id, upstream_model)
);

CREATE INDEX IF NOT EXISTS idx_ai_model_deployments_model ON ai_model_deployments (model_id, status);
CREATE INDEX IF NOT EXISTS idx_ai_model_deployments_endpoint ON ai_model_deployments (endpoint_id);
CREATE INDEX IF NOT EXISTS idx_ai_model_deployments_capability ON ai_model_deployments (capability_type, status);

CREATE TABLE IF NOT EXISTS ai_provider_model_prices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id UUID NOT NULL REFERENCES ai_providers(id),
  endpoint_id UUID REFERENCES ai_provider_endpoints(id),
  upstream_model TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  currency TEXT NOT NULL DEFAULT 'CNY_CREDITS',
  input_cost_per_1m BIGINT NOT NULL DEFAULT 0,
  output_cost_per_1m BIGINT NOT NULL DEFAULT 0,
  request_cost BIGINT NOT NULL DEFAULT 0,
  image_cost BIGINT NOT NULL DEFAULT 0,
  video_cost_per_second BIGINT NOT NULL DEFAULT 0,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio', 'rerank')),
  CONSTRAINT ai_provider_model_prices_nonnegative_credits CHECK (
    input_cost_per_1m >= 0
    AND output_cost_per_1m >= 0
    AND request_cost >= 0
    AND image_cost >= 0
    AND video_cost_per_second >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_provider_model_prices_lookup
  ON ai_provider_model_prices (provider_id, endpoint_id, upstream_model, capability_type, status, effective_from DESC);

CREATE TABLE IF NOT EXISTS ai_model_prices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id UUID NOT NULL REFERENCES ai_models(id),
  platform_input_price_per_1m BIGINT NOT NULL DEFAULT 0,
  platform_output_price_per_1m BIGINT NOT NULL DEFAULT 0,
  platform_image_price BIGINT NOT NULL DEFAULT 0,
  tenant_input_price_per_1m BIGINT NOT NULL DEFAULT 0,
  tenant_output_price_per_1m BIGINT NOT NULL DEFAULT 0,
  tenant_image_price BIGINT NOT NULL DEFAULT 0,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ai_model_prices_nonnegative_credits CHECK (
    platform_input_price_per_1m >= 0
    AND platform_output_price_per_1m >= 0
    AND platform_image_price >= 0
    AND tenant_input_price_per_1m >= 0
    AND tenant_output_price_per_1m >= 0
    AND tenant_image_price >= 0
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_model_prices_model_effective ON ai_model_prices (model_id, status, effective_from DESC);

CREATE TABLE IF NOT EXISTS ai_tenant_model_grants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  status TEXT NOT NULL DEFAULT 'active',
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_tenant_model_grants_tenant ON ai_tenant_model_grants (tenant_id, status);

CREATE TABLE IF NOT EXISTS ai_user_model_grants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  status TEXT NOT NULL DEFAULT 'active',
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_user_model_grants_user ON ai_user_model_grants (user_id, status);
CREATE INDEX IF NOT EXISTS idx_ai_user_model_grants_tenant ON ai_user_model_grants (tenant_id);

CREATE TABLE IF NOT EXISTS ai_usage_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id TEXT NOT NULL UNIQUE,
  trace_id TEXT,
  api_key_id UUID REFERENCES ai_api_keys(id),
  key_owner_type TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  user_id TEXT,
  external_user_id TEXT,
  model_code TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat',
  deployment_id UUID,
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

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'ai_provider_model_prices_nonnegative_credits'
  ) THEN
    ALTER TABLE ai_provider_model_prices
      ADD CONSTRAINT ai_provider_model_prices_nonnegative_credits CHECK (
        input_cost_per_1m >= 0
        AND output_cost_per_1m >= 0
        AND request_cost >= 0
        AND image_cost >= 0
        AND video_cost_per_second >= 0
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'ai_model_prices_nonnegative_credits'
  ) THEN
    ALTER TABLE ai_model_prices
      ADD CONSTRAINT ai_model_prices_nonnegative_credits CHECK (
        platform_input_price_per_1m >= 0
        AND platform_output_price_per_1m >= 0
        AND platform_image_price >= 0
        AND tenant_input_price_per_1m >= 0
        AND tenant_output_price_per_1m >= 0
        AND tenant_image_price >= 0
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'ai_usage_logs_nonnegative_billing'
  ) THEN
    ALTER TABLE ai_usage_logs
      ADD CONSTRAINT ai_usage_logs_nonnegative_billing CHECK (
        billable_units >= 0
        AND provider_cost >= 0
        AND platform_cost >= 0
        AND user_cost >= 0
        AND api_key_quota_cost >= 0
      );
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ai_runtime_limit_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  scope_type TEXT NOT NULL CHECK (scope_type IN ('tenant', 'user', 'api_key', 'provider', 'endpoint')),
  scope_id TEXT NOT NULL,
  capability_type TEXT NOT NULL DEFAULT 'chat' CHECK (capability_type IN ('chat', 'image', 'video', 'embedding', 'audio', 'rerank')),
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

CREATE TABLE IF NOT EXISTS ai_conversation_bindings (
  conversation_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  identity_id TEXT NOT NULL,
  model_id UUID NOT NULL REFERENCES ai_models(id),
  deployment_id UUID NOT NULL REFERENCES ai_model_deployments(id),
  endpoint_id UUID NOT NULL REFERENCES ai_provider_endpoints(id),
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
DROP TABLE IF EXISTS ai_user_model_grants;
DROP TABLE IF EXISTS ai_tenant_model_grants;
DROP TABLE IF EXISTS ai_model_prices;
DROP TABLE IF EXISTS ai_provider_model_prices;
DROP TABLE IF EXISTS ai_model_deployments;
DROP TABLE IF EXISTS ai_models;
DROP TABLE IF EXISTS ai_provider_endpoints;
DROP TABLE IF EXISTS ai_providers;
DROP TABLE IF EXISTS ai_api_keys;

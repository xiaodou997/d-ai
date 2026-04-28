-- Local E2E seed data for Navicat or psql.
--
-- Runtime key:
--   sk-ai-local-dev
--
-- Default local route:
--   openai_compatible -> http://127.0.0.1:18080/v1
--
-- Data Model:
--   Provider -> Endpoint -> Upstream Deployment -> (Model Route -> Model)
--   Cost prices bind to upstream deployment, sale prices bind to model.

BEGIN;

-- ============================================================================
-- 1. Providers
-- ============================================================================
INSERT INTO ai_providers (
  code,
  name,
  provider_type,
  is_custom,
  config,
  status
) VALUES
  ('deepseek', 'DeepSeek', 'official', false, '{}', 'active'),
  ('openai_compatible', 'OpenAI Compatible Local Fake', 'compatible', false, '{}', 'active')
ON CONFLICT (code) DO UPDATE SET
  name = EXCLUDED.name,
  provider_type = EXCLUDED.provider_type,
  is_custom = EXCLUDED.is_custom,
  config = EXCLUDED.config,
  status = EXCLUDED.status,
  updated_at = now();

-- ============================================================================
-- 2. Endpoints
-- ============================================================================
INSERT INTO ai_provider_endpoints (
  provider_id,
  name,
  base_url,
  api_key_ciphertext,
  extra_headers,
  weight,
  timeout_ms,
  status
) VALUES
  (
    (SELECT id FROM ai_providers WHERE code = 'deepseek'),
    'DeepSeek OpenAI Chat',
    'https://api.deepseek.com',
    'plain:REPLACE_ME_DEEPSEEK_API_KEY',
    '{}',
    100,
    60000,
    'active'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    'Local Fake Endpoint',
    'http://127.0.0.1:18080/v1',
    'plain:fake-local-token',
    '{}',
    100,
    30000,
    'active'
  )
ON CONFLICT (provider_id, name) DO UPDATE SET
  base_url = EXCLUDED.base_url,
  api_key_ciphertext = EXCLUDED.api_key_ciphertext,
  extra_headers = EXCLUDED.extra_headers,
  weight = EXCLUDED.weight,
  timeout_ms = EXCLUDED.timeout_ms,
  status = EXCLUDED.status,
  updated_at = now();

-- ============================================================================
-- 3. Upstream Deployments
-- ============================================================================
-- Clean up old deployments first
DELETE FROM ai_upstream_deployment_cost_prices
WHERE upstream_deployment_id IN (
  SELECT ud.id
  FROM ai_upstream_deployments ud
  JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
  WHERE e.name IN ('DeepSeek OpenAI Chat', 'Local Fake Endpoint')
);

DELETE FROM ai_model_routes
WHERE upstream_deployment_id IN (
  SELECT ud.id
  FROM ai_upstream_deployments ud
  JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
  WHERE e.name IN ('DeepSeek OpenAI Chat', 'Local Fake Endpoint')
);

DELETE FROM ai_upstream_deployments ud
USING ai_provider_endpoints e
WHERE ud.endpoint_id = e.id
  AND e.name IN ('DeepSeek OpenAI Chat', 'Local Fake Endpoint');

INSERT INTO ai_upstream_deployments (
  endpoint_id,
  name,
  upstream_model,
  capability_type,
  upstream_protocol,
  request_path,
  upstream_parameters,
  tags,
  health_status,
  status
) VALUES
  (
    (SELECT id FROM ai_provider_endpoints WHERE name = 'DeepSeek OpenAI Chat'),
    'DeepSeek V4 Pro',
    'deepseek-v4-pro',
    'chat',
    'openai_chat_completions',
    NULL,
    '{}',
    '{"tier": "premium"}',
    'unknown',
    'active'
  ),
  (
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Endpoint'),
    'Local Fake Chat',
    'fake-chat-model',
    'chat',
    'openai_chat_completions',
    NULL,
    '{}',
    '{"tier": "test"}',
    'unknown',
    'active'
  ),
  (
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Endpoint'),
    'Local Fake Responses',
    'fake-responses-model',
    'chat',
    'openai_responses',
    NULL,
    '{}',
    '{"tier": "test"}',
    'unknown',
    'active'
  ),
  (
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Endpoint'),
    'Local Fake Embeddings',
    'fake-embedding-model',
    'embedding',
    'openai_embeddings',
    NULL,
    '{}',
    '{"tier": "test"}',
    'unknown',
    'active'
  ),
  (
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Endpoint'),
    'Local Fake Images',
    'fake-image-model',
    'image',
    'openai_images_generations',
    NULL,
    '{}',
    '{"tier": "test"}',
    'unknown',
    'active'
  )
ON CONFLICT (endpoint_id, upstream_model, upstream_protocol) DO UPDATE SET
  name = EXCLUDED.name,
  capability_type = EXCLUDED.capability_type,
  request_path = EXCLUDED.request_path,
  upstream_parameters = EXCLUDED.upstream_parameters,
  tags = EXCLUDED.tags,
  health_status = EXCLUDED.health_status,
  status = EXCLUDED.status,
  updated_at = now();

-- ============================================================================
-- 4. Public Models
-- ============================================================================
INSERT INTO ai_models (
  model_code,
  display_name,
  capability_type,
  context_window,
  default_max_output_tokens,
  status
) VALUES
  ('deepseek-v4-pro', 'DeepSeek V4 Pro', 'chat', 128000, 4096, 'active'),
  ('local-chat-test', 'Local Fake Chat', 'chat', 128000, 4096, 'active'),
  ('local-responses-test', 'Local Fake Responses', 'chat', 128000, 4096, 'active'),
  ('local-embedding-test', 'Local Fake Embedding', 'embedding', 8192, 1, 'active'),
  ('local-image-test', 'Local Fake Image', 'image', NULL, 1, 'active')
ON CONFLICT (model_code) DO UPDATE SET
  display_name = EXCLUDED.display_name,
  capability_type = EXCLUDED.capability_type,
  context_window = EXCLUDED.context_window,
  default_max_output_tokens = EXCLUDED.default_max_output_tokens,
  status = EXCLUDED.status,
  updated_at = now();

-- ============================================================================
-- 5. Model Routes (Public Model -> Upstream Deployment)
-- ============================================================================
-- Clean up old routes first
DELETE FROM ai_model_routes
WHERE model_id IN (
  SELECT id FROM ai_models
  WHERE model_code IN (
    'deepseek-v4-pro',
    'local-chat-test',
    'local-responses-test',
    'local-embedding-test',
    'local-image-test'
  )
);

INSERT INTO ai_model_routes (
  model_id,
  upstream_deployment_id,
  priority,
  weight,
  supports_stream,
  status
) VALUES
  (
    (SELECT id FROM ai_models WHERE model_code = 'deepseek-v4-pro'),
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'deepseek-v4-pro' AND e.name = 'DeepSeek OpenAI Chat'),
    100,
    100,
    true,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'local-chat-test'),
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'fake-chat-model' AND e.name = 'Local Fake Endpoint'),
    100,
    100,
    true,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'local-responses-test'),
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'fake-responses-model' AND e.name = 'Local Fake Endpoint'),
    100,
    100,
    true,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'local-embedding-test'),
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'fake-embedding-model' AND e.name = 'Local Fake Endpoint'),
    100,
    100,
    false,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'local-image-test'),
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'fake-image-model' AND e.name = 'Local Fake Endpoint'),
    100,
    100,
    false,
    'active'
  )
ON CONFLICT (model_id, upstream_deployment_id) DO UPDATE SET
  priority = EXCLUDED.priority,
  weight = EXCLUDED.weight,
  supports_stream = EXCLUDED.supports_stream,
  status = EXCLUDED.status,
  updated_at = now();

-- ============================================================================
-- 6. Model Prices (Sale Prices - bind to Model, 1:1 upsert)
-- ============================================================================
INSERT INTO ai_model_prices (
  model_id,
  input_price_per_1m,
  output_price_per_1m,
  image_size_prices,
  video_price_per_second,
  audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute
)
SELECT
  id,
  CASE WHEN capability_type IN ('chat', 'embedding') THEN 1 ELSE 0 END,
  CASE WHEN capability_type = 'chat' THEN 1 ELSE 0 END,
  CASE WHEN capability_type = 'image' THEN '{"256x256": 1, "512x512": 2, "1024x1024": 4}'::jsonb ELSE '{}'::jsonb END,
  0,
  0,
  0
FROM ai_models
WHERE model_code IN (
  'deepseek-v4-pro',
  'local-chat-test',
  'local-responses-test',
  'local-embedding-test',
  'local-image-test'
)
ON CONFLICT (model_id) DO UPDATE SET
  input_price_per_1m           = EXCLUDED.input_price_per_1m,
  output_price_per_1m          = EXCLUDED.output_price_per_1m,
  image_size_prices            = EXCLUDED.image_size_prices,
  video_price_per_second       = EXCLUDED.video_price_per_second,
  audio_tts_price_per_1m_chars = EXCLUDED.audio_tts_price_per_1m_chars,
  audio_stt_price_per_minute   = EXCLUDED.audio_stt_price_per_minute,
  updated_at                   = now();

-- ============================================================================
-- 7. Upstream Deployment Cost Prices (bind to Upstream Deployment)
-- ============================================================================
INSERT INTO ai_upstream_deployment_cost_prices (
  upstream_deployment_id,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  image_size_prices,
  video_cost_per_second,
  status
) VALUES
  (
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'deepseek-v4-pro' AND e.name = 'DeepSeek OpenAI Chat'),
    'chat',
    'CNY_CREDITS',
    1,
    1,
    0,
    0,
    '{}',
    0,
    'active'
  ),
  (
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'fake-chat-model' AND e.name = 'Local Fake Endpoint'),
    'chat',
    'CNY_CREDITS',
    1,
    1,
    0,
    0,
    '{}',
    0,
    'active'
  ),
  (
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'fake-responses-model' AND e.name = 'Local Fake Endpoint'),
    'chat',
    'CNY_CREDITS',
    1,
    1,
    0,
    0,
    '{}',
    0,
    'active'
  ),
  (
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'fake-embedding-model' AND e.name = 'Local Fake Endpoint'),
    'embedding',
    'CNY_CREDITS',
    1,
    0,
    0,
    0,
    '{}',
    0,
    'active'
  ),
  (
    (SELECT ud.id FROM ai_upstream_deployments ud JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id WHERE ud.upstream_model = 'fake-image-model' AND e.name = 'Local Fake Endpoint'),
    'image',
    'CNY_CREDITS',
    0,
    0,
    0,
    1,
    '{"256x256": 1, "512x512": 2, "1024x1024": 4}',
    0,
    'active'
  );

-- ============================================================================
-- 8. Tenant Model Grants
-- ============================================================================
INSERT INTO ai_tenant_model_grants (
  tenant_id,
  model_id,
  status,
  created_by
)
SELECT 'tenant-local', id, 'active', 'local_seed'
FROM ai_models
WHERE model_code IN (
  'deepseek-v4-pro',
  'local-chat-test',
  'local-responses-test',
  'local-embedding-test',
  'local-image-test'
)
ON CONFLICT (tenant_id, model_id) DO UPDATE SET
  status = EXCLUDED.status;

-- ============================================================================
-- 9. API Keys
-- ============================================================================
INSERT INTO ai_api_keys (
  owner_type,
  tenant_id,
  user_id,
  key_hash,
  key_prefix,
  name,
  quota_limit,
  allowed_models,
  status,
  created_by
) VALUES (
  'tenant',
  'tenant-local',
  NULL,
  '62d55f3f491606f677561e3b8cbc2774f6e01b935d03fec25484cb0f090c3e1f',
  'sk-ai-local-d',
  'Local development tenant key',
  1000000000,
  '["deepseek-v4-pro","local-chat-test","local-responses-test","local-embedding-test","local-image-test"]',
  'active',
  'local_seed'
)
ON CONFLICT (key_hash) DO UPDATE SET
  quota_limit = EXCLUDED.quota_limit,
  allowed_models = EXCLUDED.allowed_models,
  status = EXCLUDED.status,
  updated_at = now();

-- ============================================================================
-- 10. Limit Policies
-- ============================================================================
DELETE FROM ai_runtime_limit_policies
WHERE created_by = 'local_seed';

INSERT INTO ai_runtime_limit_policies (
  scope_type,
  scope_id,
  capability_type,
  model_code,
  rpm_limit,
  tpm_limit,
  concurrency_limit,
  status,
  created_by
) VALUES
  ('tenant', 'tenant-local', 'chat', NULL, 120, 200000, 20, 'active', 'local_seed'),
  ('api_key', (SELECT id::text FROM ai_api_keys WHERE key_hash = '62d55f3f491606f677561e3b8cbc2774f6e01b935d03fec25484cb0f090c3e1f'), 'embedding', NULL, 300, 300000, 20, 'active', 'local_seed'),
  ('provider', (SELECT id::text FROM ai_providers WHERE code = 'openai_compatible'), 'image', NULL, 20, NULL, 5, 'active', 'local_seed');

COMMIT;
-- Local E2E seed data for Navicat or psql.
--
-- Runtime key:
--   sk-ai-local-dev
--
-- Default local route:
--   openai_compatible -> http://127.0.0.1:18080/v1
--
-- DeepSeek is included as a real-provider sample but is not required for
-- the local smoke path.

BEGIN;

INSERT INTO ai_providers (
  code,
  name,
  provider_type,
  protocol_type,
  is_custom,
  config,
  status
) VALUES
  ('deepseek', 'DeepSeek', 'deepseek', 'openai_chat_completions', false, '{}', 'active'),
  ('openai_compatible', 'OpenAI Compatible Local Fake', 'openai_compatible', 'openai_chat_completions', false, '{}', 'active')
ON CONFLICT (code) DO UPDATE SET
  name = EXCLUDED.name,
  provider_type = EXCLUDED.provider_type,
  protocol_type = EXCLUDED.protocol_type,
  is_custom = EXCLUDED.is_custom,
  config = EXCLUDED.config,
  status = EXCLUDED.status,
  updated_at = now();

INSERT INTO ai_provider_endpoints (
  provider_id,
  name,
  base_url,
  protocol_type,
  api_key_ciphertext,
  extra_headers,
  custom_path,
  weight,
  timeout_ms,
  status,
  health_status
) VALUES
  (
    (SELECT id FROM ai_providers WHERE code = 'deepseek'),
    'DeepSeek OpenAI Chat',
    'https://api.deepseek.com',
    'openai_chat_completions',
    'plain:REPLACE_ME_DEEPSEEK_API_KEY',
    '{}',
    NULL,
    100,
    60000,
    'active',
    'unknown'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    'Local Fake Chat',
    'http://127.0.0.1:18080/v1',
    'openai_chat_completions',
    'plain:fake-local-token',
    '{}',
    NULL,
    100,
    30000,
    'active',
    'unknown'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    'Local Fake Responses',
    'http://127.0.0.1:18080/v1',
    'openai_responses',
    'plain:fake-local-token',
    '{}',
    NULL,
    100,
    30000,
    'active',
    'unknown'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    'Local Fake Embeddings',
    'http://127.0.0.1:18080/v1',
    'openai_embeddings',
    'plain:fake-local-token',
    '{}',
    NULL,
    100,
    30000,
    'active',
    'unknown'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    'Local Fake Images',
    'http://127.0.0.1:18080/v1',
    'openai_images_generations',
    'plain:fake-local-token',
    '{}',
    NULL,
    100,
    30000,
    'active',
    'unknown'
  )
ON CONFLICT (provider_id, name) DO UPDATE SET
  base_url = EXCLUDED.base_url,
  protocol_type = EXCLUDED.protocol_type,
  api_key_ciphertext = EXCLUDED.api_key_ciphertext,
  extra_headers = EXCLUDED.extra_headers,
  custom_path = EXCLUDED.custom_path,
  weight = EXCLUDED.weight,
  timeout_ms = EXCLUDED.timeout_ms,
  status = EXCLUDED.status,
  health_status = EXCLUDED.health_status,
  updated_at = now();

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

DELETE FROM ai_provider_model_prices
WHERE provider_id IN (
  SELECT id FROM ai_providers WHERE code IN ('deepseek', 'openai_compatible')
)
AND upstream_model IN (
  'deepseek-v4-pro',
  'fake-chat-model',
  'fake-responses-model',
  'fake-embedding-model',
  'fake-image-model'
);

DELETE FROM ai_model_prices
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

DELETE FROM ai_runtime_limit_policies
WHERE created_by = 'local_seed'
AND (
  scope_id = 'tenant-local'
  OR scope_id IN (SELECT id::text FROM ai_providers WHERE code = 'openai_compatible')
  OR scope_id IN (SELECT id::text FROM ai_api_keys WHERE key_hash = '62d55f3f491606f677561e3b8cbc2774f6e01b935d03fec25484cb0f090c3e1f')
);

DELETE FROM ai_model_deployments d
USING ai_models m, ai_provider_endpoints e
WHERE d.model_id = m.id
  AND d.endpoint_id = e.id
  AND m.model_code IN (
    'deepseek-v4-pro',
    'local-chat-test',
    'local-responses-test',
    'local-embedding-test',
    'local-image-test'
  )
  AND e.name IN (
    'DeepSeek OpenAI Chat',
    'Local Fake Chat',
    'Local Fake Responses',
    'Local Fake Embeddings',
    'Local Fake Images'
  );

INSERT INTO ai_model_deployments (
  model_id,
  endpoint_id,
  upstream_model,
  capability_type,
  upstream_protocol,
  upstream_parameters,
  priority,
  weight,
  supports_stream,
  status
) VALUES
  (
    (SELECT id FROM ai_models WHERE model_code = 'deepseek-v4-pro'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'DeepSeek OpenAI Chat'),
    'deepseek-v4-pro',
    'chat',
    'openai_chat_completions',
    '{}',
    100,
    100,
    true,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'local-chat-test'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Chat'),
    'fake-chat-model',
    'chat',
    'openai_chat_completions',
    '{}',
    100,
    100,
    true,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'local-responses-test'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Responses'),
    'fake-responses-model',
    'chat',
    'openai_responses',
    '{}',
    100,
    100,
    true,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'local-embedding-test'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Embeddings'),
    'fake-embedding-model',
    'embedding',
    'openai_embeddings',
    '{}',
    100,
    100,
    false,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'local-image-test'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Images'),
    'fake-image-model',
    'image',
    'openai_images_generations',
    '{}',
    100,
    100,
    false,
    'active'
  )
ON CONFLICT (model_id, endpoint_id, upstream_model) DO UPDATE SET
  capability_type = EXCLUDED.capability_type,
  upstream_protocol = EXCLUDED.upstream_protocol,
  upstream_parameters = EXCLUDED.upstream_parameters,
  priority = EXCLUDED.priority,
  weight = EXCLUDED.weight,
  supports_stream = EXCLUDED.supports_stream,
  status = EXCLUDED.status,
  updated_at = now();

INSERT INTO ai_model_prices (
  model_id,
  platform_input_price_per_1m,
  platform_output_price_per_1m,
  platform_image_price,
  tenant_input_price_per_1m,
  tenant_output_price_per_1m,
  tenant_image_price,
  status
)
SELECT
  id,
  CASE WHEN capability_type IN ('chat', 'embedding') THEN 1 ELSE 0 END,
  CASE WHEN capability_type = 'chat' THEN 1 ELSE 0 END,
  CASE WHEN capability_type = 'image' THEN 1 ELSE 0 END,
  CASE WHEN capability_type IN ('chat', 'embedding') THEN 1 ELSE 0 END,
  CASE WHEN capability_type = 'chat' THEN 1 ELSE 0 END,
  CASE WHEN capability_type = 'image' THEN 1 ELSE 0 END,
  'active'
FROM ai_models
WHERE model_code IN (
  'deepseek-v4-pro',
  'local-chat-test',
  'local-responses-test',
  'local-embedding-test',
  'local-image-test'
);

INSERT INTO ai_provider_model_prices (
  provider_id,
  endpoint_id,
  upstream_model,
  capability_type,
  currency,
  input_cost_per_1m,
  output_cost_per_1m,
  request_cost,
  image_cost,
  status
) VALUES
  (
    (SELECT id FROM ai_providers WHERE code = 'deepseek'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'DeepSeek OpenAI Chat'),
    'deepseek-v4-pro',
    'chat',
    'CNY_CREDITS',
    1,
    1,
    0,
    0,
    'active'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Chat'),
    'fake-chat-model',
    'chat',
    'CNY_CREDITS',
    1,
    1,
    0,
    0,
    'active'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Responses'),
    'fake-responses-model',
    'chat',
    'CNY_CREDITS',
    1,
    1,
    0,
    0,
    'active'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Embeddings'),
    'fake-embedding-model',
    'embedding',
    'CNY_CREDITS',
    1,
    0,
    0,
    0,
    'active'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'openai_compatible'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'Local Fake Images'),
    'fake-image-model',
    'image',
    'CNY_CREDITS',
    0,
    0,
    0,
    1,
    'active'
  );

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

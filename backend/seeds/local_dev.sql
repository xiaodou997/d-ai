-- Local development seed data.
--
-- Before running this file, replace:
--   REPLACE_ME_DEEPSEEK_API_KEY
--   REPLACE_ME_ALI_CODING_PLAN_API_KEY
--   REPLACE_ME_ALI_TOKEN_PLAN_API_KEY
--   REPLACE_ME_ALI_CODING_PLAN_MODEL
--   REPLACE_ME_ALI_TOKEN_PLAN_MODEL
--
-- Runtime test API key created by this seed:
--   sk-ai-local-dev

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
  ('dashscope_coding_plan', 'DashScope Coding Plan', 'dashscope', 'openai_chat_completions', false, '{}', 'active'),
  ('dashscope_token_plan', 'DashScope Token Plan', 'dashscope', 'openai_chat_completions', false, '{}', 'active')
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
    (SELECT id FROM ai_providers WHERE code = 'dashscope_coding_plan'),
    'DashScope Coding Plan OpenAI',
    'https://coding.dashscope.aliyuncs.com/v1',
    'openai_chat_completions',
    'plain:REPLACE_ME_ALI_CODING_PLAN_API_KEY',
    '{}',
    NULL,
    100,
    60000,
    'active',
    'unknown'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'dashscope_coding_plan'),
    'DashScope Coding Plan Anthropic',
    'https://coding.dashscope.aliyuncs.com/apps/anthropic',
    'anthropic_messages',
    'plain:REPLACE_ME_ALI_CODING_PLAN_API_KEY',
    '{}',
    NULL,
    100,
    60000,
    'active',
    'unknown'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'dashscope_token_plan'),
    'DashScope Token Plan OpenAI',
    'https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1',
    'openai_chat_completions',
    'plain:REPLACE_ME_ALI_TOKEN_PLAN_API_KEY',
    '{}',
    NULL,
    100,
    60000,
    'active',
    'unknown'
  ),
  (
    (SELECT id FROM ai_providers WHERE code = 'dashscope_token_plan'),
    'DashScope Token Plan Anthropic',
    'https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic',
    'anthropic_messages',
    'plain:REPLACE_ME_ALI_TOKEN_PLAN_API_KEY',
    '{}',
    NULL,
    100,
    60000,
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

UPDATE ai_provider_endpoints SET
  base_url = 'https://api.deepseek.com',
  protocol_type = 'openai_chat_completions',
  api_key_ciphertext = 'plain:REPLACE_ME_DEEPSEEK_API_KEY',
  custom_path = NULL,
  status = 'active',
  updated_at = now()
WHERE name = 'DeepSeek OpenAI Chat';

UPDATE ai_provider_endpoints SET
  base_url = 'https://coding.dashscope.aliyuncs.com/v1',
  protocol_type = 'openai_chat_completions',
  api_key_ciphertext = 'plain:REPLACE_ME_ALI_CODING_PLAN_API_KEY',
  custom_path = NULL,
  status = 'active',
  updated_at = now()
WHERE name = 'DashScope Coding Plan OpenAI';

UPDATE ai_provider_endpoints SET
  base_url = 'https://coding.dashscope.aliyuncs.com/apps/anthropic',
  protocol_type = 'anthropic_messages',
  api_key_ciphertext = 'plain:REPLACE_ME_ALI_CODING_PLAN_API_KEY',
  custom_path = NULL,
  status = 'active',
  updated_at = now()
WHERE name = 'DashScope Coding Plan Anthropic';

UPDATE ai_provider_endpoints SET
  base_url = 'https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1',
  protocol_type = 'openai_chat_completions',
  api_key_ciphertext = 'plain:REPLACE_ME_ALI_TOKEN_PLAN_API_KEY',
  custom_path = NULL,
  status = 'active',
  updated_at = now()
WHERE name = 'DashScope Token Plan OpenAI';

UPDATE ai_provider_endpoints SET
  base_url = 'https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic',
  protocol_type = 'anthropic_messages',
  api_key_ciphertext = 'plain:REPLACE_ME_ALI_TOKEN_PLAN_API_KEY',
  custom_path = NULL,
  status = 'active',
  updated_at = now()
WHERE name = 'DashScope Token Plan Anthropic';

INSERT INTO ai_models (
  model_code,
  display_name,
  capability_type,
  context_window,
  default_max_output_tokens,
  status
) VALUES
  ('deepseek-v4-pro', 'DeepSeek V4 Pro', 'chat', 128000, 4096, 'active'),
  ('deepseek-v4-flash', 'DeepSeek V4 Flash', 'chat', 128000, 4096, 'active'),
  ('ali-coding-plan-test', 'Ali Coding Plan Test Model', 'chat', NULL, 4096, 'active'),
  ('ali-token-plan-test', 'Ali Token Plan Test Model', 'chat', NULL, 4096, 'active')
ON CONFLICT (model_code) DO UPDATE SET
  display_name = EXCLUDED.display_name,
  capability_type = EXCLUDED.capability_type,
  context_window = EXCLUDED.context_window,
  default_max_output_tokens = EXCLUDED.default_max_output_tokens,
  status = EXCLUDED.status,
  updated_at = now();

DELETE FROM ai_model_deployments d
USING ai_models m, ai_provider_endpoints e
WHERE d.model_id = m.id
  AND d.endpoint_id = e.id
  AND (
    (m.model_code = 'deepseek-v4-pro' AND e.name = 'DeepSeek OpenAI Chat') OR
    (m.model_code = 'deepseek-v4-flash' AND e.name = 'DeepSeek OpenAI Chat') OR
    (m.model_code = 'ali-coding-plan-test' AND e.name = 'DashScope Coding Plan OpenAI') OR
    (m.model_code = 'ali-token-plan-test' AND e.name = 'DashScope Token Plan OpenAI')
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
    (SELECT id FROM ai_models WHERE model_code = 'deepseek-v4-flash'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'DeepSeek OpenAI Chat'),
    'deepseek-v4-flash',
    'chat',
    'openai_chat_completions',
    '{}',
    100,
    100,
    true,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'ali-coding-plan-test'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'DashScope Coding Plan OpenAI'),
    'REPLACE_ME_ALI_CODING_PLAN_MODEL',
    'chat',
    'openai_chat_completions',
    '{}',
    100,
    100,
    true,
    'active'
  ),
  (
    (SELECT id FROM ai_models WHERE model_code = 'ali-token-plan-test'),
    (SELECT id FROM ai_provider_endpoints WHERE name = 'DashScope Token Plan OpenAI'),
    'REPLACE_ME_ALI_TOKEN_PLAN_MODEL',
    'chat',
    'openai_chat_completions',
    '{}',
    100,
    100,
    true,
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
  tenant_input_price_per_1m,
  tenant_output_price_per_1m,
  status
)
SELECT id, 0, 0, 0, 0, 'active'
FROM ai_models
WHERE model_code IN ('deepseek-v4-pro', 'deepseek-v4-flash', 'ali-coding-plan-test', 'ali-token-plan-test')
ON CONFLICT DO NOTHING;

INSERT INTO ai_tenant_model_grants (
  tenant_id,
  model_id,
  status,
  created_by
)
SELECT 'tenant-local', id, 'active', 'seed'
FROM ai_models
WHERE model_code IN ('deepseek-v4-pro', 'deepseek-v4-flash', 'ali-coding-plan-test', 'ali-token-plan-test')
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
  '["deepseek-v4-pro","deepseek-v4-flash","ali-coding-plan-test","ali-token-plan-test"]',
  'active',
  'seed'
)
ON CONFLICT (key_hash) DO UPDATE SET
  quota_limit = EXCLUDED.quota_limit,
  allowed_models = EXCLUDED.allowed_models,
  status = EXCLUDED.status,
  updated_at = now();

COMMIT;

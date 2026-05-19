-- ============================================================================
-- Pool Deployment 统一重构迁移
-- 把 ai_model_routes 中的 pool 路由旁路彻底删除，
-- Pool 也成为 ai_upstream_deployments 中的一条记录。
-- ============================================================================

BEGIN;

-- Step 1: ai_upstream_deployments - endpoint_id 改可空，加 credential_pool_id
ALTER TABLE ai_upstream_deployments
  ALTER COLUMN endpoint_id DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS credential_pool_id UUID REFERENCES ai_credential_pools(id);

ALTER TABLE ai_upstream_deployments
  ADD CONSTRAINT chk_deployment_credential_source CHECK (
    (endpoint_id IS NOT NULL AND credential_pool_id IS NULL) OR
    (endpoint_id IS NULL AND credential_pool_id IS NOT NULL)
  );

-- 更新 UNIQUE 约束（旧约束只覆盖 endpoint_id）
ALTER TABLE ai_upstream_deployments
  DROP CONSTRAINT IF EXISTS ai_upstream_deployments_endpoint_id_upstream_model_upstream_key;
ALTER TABLE ai_upstream_deployments
  ADD CONSTRAINT uq_deployment_key UNIQUE NULLS NOT DISTINCT
    (endpoint_id, credential_pool_id, upstream_model, upstream_protocol);

CREATE INDEX IF NOT EXISTS idx_ai_upstream_deployments_pool
  ON ai_upstream_deployments (credential_pool_id) WHERE credential_pool_id IS NOT NULL;

-- Step 2: 数据迁移 - 为现有 pool 路由创建 pool deployment 记录
INSERT INTO ai_upstream_deployments (
  credential_pool_id, upstream_model, capability_type, upstream_protocol, status
)
SELECT DISTINCT
  r.credential_pool_id,
  r.pool_upstream_model,
  m.capability_type,
  CASE cp.fixed_provider_type
    WHEN 'claude_oauth' THEN 'anthropic_messages'
    WHEN 'gemini_cli'   THEN 'gemini_generate'
    WHEN 'codex'        THEN 'openai_completions'
    ELSE 'openai_chat'
  END,
  'active'
FROM ai_model_routes r
JOIN ai_models m ON m.id = r.model_id
JOIN ai_credential_pools cp ON cp.id = r.credential_pool_id
WHERE r.credential_pool_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Step 3: 回填 upstream_deployment_id
UPDATE ai_model_routes r
SET upstream_deployment_id = ud.id
FROM ai_upstream_deployments ud
WHERE r.credential_pool_id IS NOT NULL
  AND ud.credential_pool_id = r.credential_pool_id
  AND ud.upstream_model = r.pool_upstream_model;

-- Step 4: ai_model_routes - 清理 pool 字段
ALTER TABLE ai_model_routes
  ALTER COLUMN upstream_deployment_id SET NOT NULL;

ALTER TABLE ai_model_routes
  DROP CONSTRAINT IF EXISTS route_target_xor,
  DROP COLUMN IF EXISTS credential_pool_id,
  DROP COLUMN IF EXISTS pool_upstream_model;

ALTER TABLE ai_model_routes
  DROP CONSTRAINT IF EXISTS ai_model_routes_model_id_credential_pool_id_pool_upstream_mod;

DROP INDEX IF EXISTS idx_ai_model_routes_pool;

-- 统一 UNIQUE 约束：(model_id, upstream_deployment_id)
ALTER TABLE ai_model_routes
  DROP CONSTRAINT IF EXISTS ai_model_routes_model_id_upstream_deployment_id_key;
ALTER TABLE ai_model_routes
  ADD CONSTRAINT uq_model_route UNIQUE (model_id, upstream_deployment_id);

COMMIT;

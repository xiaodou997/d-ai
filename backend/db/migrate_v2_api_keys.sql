-- ============================================================================
-- API Keys v2 迁移脚本
-- ============================================================================
-- 警告：此迁移不可逆（DROP COLUMN）。执行前请备份数据库。
-- 请在事务中手动执行：BEGIN; <此脚本> COMMIT;
-- ============================================================================

\echo '⚠️  This migration is IRREVERSIBLE. Backup before running. Wrap in BEGIN/COMMIT manually.'
\echo '⚠️  Old key_value and key_prefix data will be PERMANENTLY DROPPED.'

-- 1. 将 expired 状态迁移为 disabled（status CHECK 即将收窄）
UPDATE ai_api_keys SET status = 'disabled' WHERE status = 'expired';

-- 2. 替换 status CHECK 约束
ALTER TABLE ai_api_keys DROP CONSTRAINT ai_api_keys_status_check;
ALTER TABLE ai_api_keys ADD CONSTRAINT ai_api_keys_status_check
  CHECK (status IN ('active', 'disabled'));

-- 3. 删除明文列
ALTER TABLE ai_api_keys DROP COLUMN IF EXISTS key_prefix;
ALTER TABLE ai_api_keys DROP COLUMN IF EXISTS key_value;

-- 4. 新增 last_four 和 last_used_at（NULL 兜底旧 key）
ALTER TABLE ai_api_keys ADD COLUMN IF NOT EXISTS last_four    CHAR(4);
ALTER TABLE ai_api_keys ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;

-- 5. 索引
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_api_keys_key_hash
  ON ai_api_keys (key_hash);
CREATE INDEX IF NOT EXISTS idx_ai_api_keys_tenant_status
  ON ai_api_keys (tenant_id, status);

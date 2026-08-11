-- from_version: 1
-- to_version: 2
-- created_at: 2026-08-11
-- description: unify login identifier (username or email); drop u_ username namespace check; add unique email index
--
-- 注意：脚本会自动清理重复的非空邮箱（每组保留最早创建的账号，其余 email 置 NULL），
-- 被清理的账号将失去邮箱登录能力，可执行以下 SQL 查看受影响的账号：
--   SELECT user_id, username, email FROM iam_accounts a
--   WHERE email IS NOT NULL AND a.id NOT IN (
--       SELECT DISTINCT ON (lower(email)) id FROM iam_accounts
--       WHERE email IS NOT NULL ORDER BY lower(email), id
--   );

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 1
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 1';
    END IF;
END
$$;

-- 终端用户不再强制 u_ 前缀，存量 u_ 用户名与新用户名共存
ALTER TABLE iam_accounts
    DROP CONSTRAINT iam_accounts_username_namespace_check;

-- 清理重复的非空邮箱：每组（大小写不敏感）保留最早创建的账号，其余置 NULL
UPDATE iam_accounts
SET email = NULL,
    updated_at = now()
WHERE email IS NOT NULL
  AND id NOT IN (
      SELECT DISTINCT ON (lower(email)) id
      FROM iam_accounts
      WHERE email IS NOT NULL
      ORDER BY lower(email), id
  );

-- 邮箱全局唯一（大小写不敏感），允许为空，作为邮箱登录的前提
CREATE UNIQUE INDEX ux_iam_accounts_email_normalized
    ON iam_accounts (lower(email))
    WHERE email IS NOT NULL;

UPDATE dai_schema_metadata
SET version = 2,
    updated_at = now()
WHERE singleton = TRUE AND version = 1;

COMMIT;

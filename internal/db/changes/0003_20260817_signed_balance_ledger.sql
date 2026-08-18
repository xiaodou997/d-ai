-- from_version: 2
-- to_version: 3
-- created_at: 2026-08-17
-- description: collapse account balance into one signed column; add settlement outbox
--
-- 背景
-- ----
-- 「余额」此前由 2 张表 3 个非负列共同表达：
--   bill_credit_packages.remaining_credits  正余额（还需按 expires_at 过滤）
--   iam_tenants.current_overdraft           负余额取反
--   iam_accounts.current_overdraft          负余额取反
-- 因为正负被拆成两个非负列，没有任何约束能表达「余额」这个概念，全仓有 11 处
-- 各自拼装该数字，其中 system/pg 与 tenant/pg 两处漏了过期过滤。本脚本把余额
-- 收敛为 bill_accounts.balance_micro 一个有符号列。
--
-- 迁移前请先执行下面的对账查询并保存结果，迁移后重跑本文件末尾的对账查询比对：
--
--   SELECT t.tenant_id AS account_id, 1 AS kind,
--          COALESCE((SELECT SUM(p.remaining_credits) FROM bill_credit_packages p
--                    WHERE p.package_type = 'tenant' AND p.tenant_id = t.tenant_id
--                      AND p.status = 'available'
--                      AND (p.expires_at IS NULL OR p.expires_at > now())), 0)
--          - COALESCE(t.current_overdraft, 0) AS balance_micro
--   FROM iam_tenants t
--   UNION ALL
--   SELECT a.user_id, 2,
--          COALESCE((SELECT SUM(p.remaining_credits) FROM bill_credit_packages p
--                    WHERE p.package_type = 'user' AND p.user_id = a.user_id
--                      AND p.status = 'available'
--                      AND (p.expires_at IS NULL OR p.expires_at > now())), 0)
--          - COALESCE(a.current_overdraft, 0)
--   FROM iam_accounts a WHERE a.user_type = 4
--   ORDER BY 2, 1;
--
-- 旧版消费流水表保留；AI 执行记录不受影响。

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 2
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 2';
    END IF;
END
$$;

-- ---------------------------------------------------------------------------
-- 1. 账户表：余额的唯一真相。刻意有符号、刻意无 CHECK —— 负数就是欠费。
-- ---------------------------------------------------------------------------

CREATE TABLE bill_accounts (
    account_id TEXT PRIMARY KEY,
    account_kind SMALLINT NOT NULL CHECK (account_kind IN (1, 2)),
    tenant_id TEXT NOT NULL,
    balance_micro BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bill_accounts_tenant ON bill_accounts (tenant_id, account_kind);
CREATE INDEX idx_bill_accounts_negative ON bill_accounts (account_id) WHERE balance_micro <= 0;

-- 开户由数据库保证，而不是靠 6 处创建代码各自记得。准入读 bill_accounts 且缺行时
-- fail-closed，所以「漏建账户」等于让该主体直接不可用；做成结构性约束比写进文档可靠。
CREATE FUNCTION bill_provision_account() RETURNS TRIGGER AS $$
BEGIN
    IF TG_TABLE_NAME = 'iam_tenants' THEN
        INSERT INTO bill_accounts (account_id, account_kind, tenant_id)
        VALUES (NEW.tenant_id, 1, NEW.tenant_id)
        ON CONFLICT (account_id) DO NOTHING;
    ELSIF NEW.user_type = 4 AND NEW.tenant_id IS NOT NULL THEN
        INSERT INTO bill_accounts (account_id, account_kind, tenant_id)
        VALUES (NEW.user_id, 2, NEW.tenant_id)
        ON CONFLICT (account_id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_bill_provision_tenant_account
    AFTER INSERT ON iam_tenants
    FOR EACH ROW EXECUTE FUNCTION bill_provision_account();

CREATE TRIGGER trg_bill_provision_user_account
    AFTER INSERT ON iam_accounts
    FOR EACH ROW EXECUTE FUNCTION bill_provision_account();

-- 租户账户：未过期可用额度之和 - 当前欠费
INSERT INTO bill_accounts (account_id, account_kind, tenant_id, balance_micro)
SELECT t.tenant_id,
       1,
       t.tenant_id,
       COALESCE((
           SELECT SUM(p.remaining_credits)
           FROM bill_credit_packages p
           WHERE p.package_type = 'tenant'
             AND p.tenant_id = t.tenant_id
             AND p.status = 'available'
             AND (p.expires_at IS NULL OR p.expires_at > now())
       ), 0) - COALESCE(t.current_overdraft, 0)
FROM iam_tenants t;

-- 终端用户账户（user_type = 4）：同构
INSERT INTO bill_accounts (account_id, account_kind, tenant_id, balance_micro)
SELECT a.user_id,
       2,
       a.tenant_id,
       COALESCE((
           SELECT SUM(p.remaining_credits)
           FROM bill_credit_packages p
           WHERE p.package_type = 'user'
             AND p.user_id = a.user_id
             AND p.status = 'available'
             AND (p.expires_at IS NULL OR p.expires_at > now())
       ), 0) - COALESCE(a.current_overdraft, 0)
FROM iam_accounts a
WHERE a.user_type = 4
  AND a.tenant_id IS NOT NULL;

-- 存量一致性诊断：旧模型允许「有可用余额的同时还欠着费」这种不可能状态。
-- 迁移把两者相加得到正确的净额，但这类账户值得人工回看一眼。
DO $$
DECLARE
    inconsistent INTEGER;
BEGIN
    SELECT COUNT(*) INTO inconsistent
    FROM (
        SELECT t.tenant_id
        FROM iam_tenants t
        WHERE COALESCE(t.current_overdraft, 0) > 0
          AND COALESCE((
              SELECT SUM(p.remaining_credits)
              FROM bill_credit_packages p
              WHERE p.package_type = 'tenant' AND p.tenant_id = t.tenant_id
                AND p.status = 'available'
                AND (p.expires_at IS NULL OR p.expires_at > now())
          ), 0) > 0
        UNION ALL
        SELECT a.user_id
        FROM iam_accounts a
        WHERE a.user_type = 4
          AND COALESCE(a.current_overdraft, 0) > 0
          AND COALESCE((
              SELECT SUM(p.remaining_credits)
              FROM bill_credit_packages p
              WHERE p.package_type = 'user' AND p.user_id = a.user_id
                AND p.status = 'available'
                AND (p.expires_at IS NULL OR p.expires_at > now())
          ), 0) > 0
    ) AS s;

    IF inconsistent > 0 THEN
        RAISE NOTICE '% 个账户在旧模型下同时持有可用余额和欠费；已按净额迁移，建议人工复核', inconsistent;
    END IF;
END
$$;

-- ---------------------------------------------------------------------------
-- 2. 额度批次：降级为分摊明细。状态不再存储，全部可从时间戳和消耗量推导。
-- ---------------------------------------------------------------------------

CREATE TABLE bill_credit_lots (
    id BIGSERIAL PRIMARY KEY,
    lot_id TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES bill_accounts (account_id),
    granted_micro BIGINT NOT NULL CHECK (granted_micro > 0),
    consumed_micro BIGINT NOT NULL DEFAULT 0 CHECK (consumed_micro >= 0),
    expires_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    source TEXT NOT NULL CONSTRAINT bill_credit_lots_source_check
        CHECK (source IN ('ADMIN_RECHARGE', 'TENANT_RECHARGE', 'REFUND', 'ONLINE_TOPUP', 'USER_TOPUP_INCOME')),
    recharge_order_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bill_credit_lots_fifo
    ON bill_credit_lots (account_id, expires_at NULLS LAST, created_at)
    WHERE expired_at IS NULL AND revoked_at IS NULL;
CREATE INDEX idx_bill_credit_lots_due
    ON bill_credit_lots (expires_at)
    WHERE expired_at IS NULL AND revoked_at IS NULL AND expires_at IS NOT NULL;
CREATE INDEX idx_bill_credit_lots_order
    ON bill_credit_lots (recharge_order_id)
    WHERE recharge_order_id IS NOT NULL;

-- 只搬运仍然计入余额的批次（available 且未过期）。granted/consumed 保留原始
-- 口径，账户页的「总额/已用/剩余」展示因此不变。终态批次（expired / depleted /
-- revoked）不再影响任何计算，不搬运。
INSERT INTO bill_credit_lots (
    lot_id, account_id, granted_micro, consumed_micro,
    expires_at, source, recharge_order_id, created_at, updated_at
)
SELECT p.package_id,
       CASE WHEN p.package_type = 'tenant' THEN p.tenant_id ELSE p.user_id END,
       p.total_credits,
       p.total_credits - p.remaining_credits,
       p.expires_at,
       p.source,
       p.recharge_order_id,
       p.created_at,
       p.updated_at
FROM bill_credit_packages p
WHERE p.status = 'available'
  AND (p.expires_at IS NULL OR p.expires_at > now())
  AND EXISTS (
      SELECT 1 FROM bill_accounts b
      WHERE b.account_id = CASE WHEN p.package_type = 'tenant' THEN p.tenant_id ELSE p.user_id END
  );

-- 归一化：旧模型允许「同时有可用额度和欠费」，净额搬进 balance 后，这类账户的
-- 批次剩余会高于可花余额。不变量是 SUM(批次未消耗) = GREATEST(balance, 0)，
-- 这里按 FIFO 把多出来的部分标记为已消耗，让批次明细从第一天起就和余额自洽。
WITH excess AS (
    SELECT b.account_id,
           COALESCE(SUM(l.granted_micro - l.consumed_micro), 0) - GREATEST(b.balance_micro, 0) AS amount
    FROM bill_accounts b
    JOIN bill_credit_lots l
      ON l.account_id = b.account_id AND l.expired_at IS NULL AND l.revoked_at IS NULL
    GROUP BY b.account_id, b.balance_micro
    HAVING COALESCE(SUM(l.granted_micro - l.consumed_micro), 0) > GREATEST(b.balance_micro, 0)
),
ordered AS (
    SELECT l.lot_id,
           l.granted_micro - l.consumed_micro AS remaining,
           e.amount,
           COALESCE(SUM(l.granted_micro - l.consumed_micro) OVER (
               PARTITION BY l.account_id
               ORDER BY l.expires_at NULLS LAST, l.created_at
               ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
           ), 0) AS consumed_before
    FROM bill_credit_lots l
    JOIN excess e ON e.account_id = l.account_id
    WHERE l.expired_at IS NULL AND l.revoked_at IS NULL
)
UPDATE bill_credit_lots l
SET consumed_micro = l.consumed_micro + LEAST(o.remaining, GREATEST(o.amount - o.consumed_before, 0)),
    updated_at = now()
FROM ordered o
WHERE l.lot_id = o.lot_id
  AND LEAST(o.remaining, GREATEST(o.amount - o.consumed_before, 0)) > 0;

-- ---------------------------------------------------------------------------
-- 3. 结算 outbox：与 ai_usage_logs 同事务写入，保证用量与扣费不会分叉。
-- ---------------------------------------------------------------------------

CREATE TABLE bill_charge_outbox (
    id BIGSERIAL PRIMARY KEY,
    request_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    tenant_micro BIGINT NOT NULL DEFAULT 0 CHECK (tenant_micro >= 0),
    user_micro BIGINT NOT NULL DEFAULT 0 CHECK (user_micro >= 0),
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at TIMESTAMPTZ
);

CREATE INDEX idx_bill_charge_outbox_pending ON bill_charge_outbox (id) WHERE status = 'pending';
CREATE INDEX idx_bill_charge_outbox_failed ON bill_charge_outbox (created_at DESC) WHERE status = 'failed';

-- ---------------------------------------------------------------------------
-- 4. 拆除旧表示。执行到这里说明 bill_accounts 已经承载了全部余额。
-- ---------------------------------------------------------------------------

DROP TABLE bill_credit_packages;

-- 信用额度上限这个概念已取消：结算永不阻断，准入只看余额是否为正。
DROP TABLE bill_overdraft_adjustments;

DROP INDEX idx_iam_tenants_has_overdraft;
DROP INDEX idx_iam_accounts_has_overdraft;

ALTER TABLE iam_tenants
    DROP COLUMN frozen_credits,
    DROP COLUMN overdraft_limit,
    DROP COLUMN current_overdraft;

ALTER TABLE iam_accounts
    DROP COLUMN frozen_credits,
    DROP COLUMN overdraft_limit,
    DROP COLUMN current_overdraft;

UPDATE dai_schema_metadata
SET version = 3,
    updated_at = now()
WHERE singleton = TRUE AND version = 2;

COMMIT;

-- 迁移后对账：与脚本头部保存的迁移前结果逐行比对，balance_micro 必须完全一致。
--
--   SELECT account_id, account_kind, balance_micro
--   FROM bill_accounts
--   ORDER BY account_kind, account_id;
--
--   SELECT account_kind, COUNT(*) AS accounts, SUM(balance_micro) AS total_micro
--   FROM bill_accounts GROUP BY account_kind ORDER BY account_kind;

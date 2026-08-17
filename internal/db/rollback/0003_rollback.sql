-- 0003 回滚：schema 3 → 2
--
-- 仅可在「已执行 0003、但业务流量尚未恢复」的维护窗口内使用。流量恢复后产生的
-- 余额变动无法映射回旧的两列表示，届时唯一正确的回滚是恢复备份。
--
-- 脚本会拒绝在检测到迁移后新增账务活动时执行。

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM dai_schema_metadata WHERE singleton = TRUE AND version = 3) THEN
        RAISE EXCEPTION 'expected D-AI schema version 3';
    END IF;
    IF EXISTS (SELECT 1 FROM bill_charge_outbox) THEN
        RAISE EXCEPTION '检测到 % 条结算记录，说明流量已恢复；回滚必须改用恢复备份',
            (SELECT COUNT(*) FROM bill_charge_outbox);
    END IF;
END
$$;

ALTER TABLE iam_tenants
    ADD COLUMN frozen_credits BIGINT NOT NULL DEFAULT 0 CHECK (frozen_credits >= 0),
    ADD COLUMN overdraft_limit BIGINT NOT NULL DEFAULT 0 CHECK (overdraft_limit >= 0),
    ADD COLUMN current_overdraft BIGINT NOT NULL DEFAULT 0 CHECK (current_overdraft >= 0);

ALTER TABLE iam_accounts
    ADD COLUMN frozen_credits BIGINT NOT NULL DEFAULT 0 CHECK (frozen_credits >= 0),
    ADD COLUMN overdraft_limit BIGINT NOT NULL DEFAULT 0 CHECK (overdraft_limit >= 0),
    ADD COLUMN current_overdraft BIGINT NOT NULL DEFAULT 0 CHECK (current_overdraft >= 0);

-- 负余额还原为欠费
UPDATE iam_tenants t SET current_overdraft = GREATEST(-b.balance_micro, 0)
FROM bill_accounts b WHERE b.account_id = t.tenant_id AND b.account_kind = 1;

UPDATE iam_accounts a SET current_overdraft = GREATEST(-b.balance_micro, 0)
FROM bill_accounts b WHERE b.account_id = a.user_id AND b.account_kind = 2;

CREATE TABLE bill_credit_packages (
    id BIGSERIAL PRIMARY KEY,
    package_id TEXT NOT NULL UNIQUE,
    package_type TEXT NOT NULL CHECK (package_type IN ('tenant', 'user')),
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    total_credits BIGINT NOT NULL CHECK (total_credits > 0),
    remaining_credits BIGINT NOT NULL CHECK (remaining_credits >= 0 AND remaining_credits <= total_credits),
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'available' CHECK (status IN ('available', 'expired', 'depleted', 'revoked')),
    source TEXT NOT NULL CONSTRAINT bill_credit_packages_source_check
        CHECK (source IN ('ADMIN_RECHARGE', 'TENANT_RECHARGE', 'REFUND', 'ONLINE_TOPUP', 'USER_TOPUP_INCOME')),
    recharge_order_id TEXT,
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (package_type = 'tenant' AND user_id IS NULL)
        OR (package_type = 'user' AND user_id IS NOT NULL)
    )
);

INSERT INTO bill_credit_packages
    (package_id, package_type, tenant_id, user_id, total_credits, remaining_credits,
     expires_at, status, source, recharge_order_id, created_at, updated_at)
SELECT l.lot_id,
       CASE WHEN b.account_kind = 1 THEN 'tenant' ELSE 'user' END,
       b.tenant_id,
       CASE WHEN b.account_kind = 2 THEN b.account_id END,
       l.granted_micro,
       l.granted_micro - l.consumed_micro,
       l.expires_at,
       'available',
       l.source,
       l.recharge_order_id,
       l.created_at,
       l.updated_at
FROM bill_credit_lots l
JOIN bill_accounts b ON b.account_id = l.account_id
WHERE l.expired_at IS NULL AND l.revoked_at IS NULL AND l.granted_micro > l.consumed_micro;

CREATE INDEX idx_bill_credit_packages_tenant_fifo
    ON bill_credit_packages (package_type, tenant_id, status, expires_at, created_at);
CREATE INDEX idx_bill_credit_packages_user_fifo
    ON bill_credit_packages (package_type, user_id, status, expires_at, created_at)
    WHERE user_id IS NOT NULL;
CREATE INDEX idx_bill_credit_packages_order
    ON bill_credit_packages (recharge_order_id)
    WHERE recharge_order_id IS NOT NULL;

CREATE TABLE bill_overdraft_adjustments (
    id BIGSERIAL PRIMARY KEY,
    account_type INTEGER NOT NULL CHECK (account_type IN (1, 2)),
    account_id TEXT NOT NULL,
    from_limit BIGINT NOT NULL CHECK (from_limit >= 0),
    to_limit BIGINT NOT NULL CHECK (to_limit >= 0),
    operator_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bill_overdraft_adjustments_account
    ON bill_overdraft_adjustments (account_type, account_id, created_at DESC);

CREATE INDEX idx_iam_tenants_has_overdraft ON iam_tenants (tenant_id) WHERE current_overdraft > 0;
CREATE INDEX idx_iam_accounts_has_overdraft ON iam_accounts (user_id)
    WHERE user_type = 4 AND current_overdraft > 0;

DROP TRIGGER trg_bill_provision_user_account ON iam_accounts;
DROP TRIGGER trg_bill_provision_tenant_account ON iam_tenants;
DROP FUNCTION bill_provision_account();
DROP TABLE bill_charge_outbox;
DROP TABLE bill_credit_lots;
DROP TABLE bill_accounts;

UPDATE dai_schema_metadata SET version = 2, updated_at = now()
WHERE singleton = TRUE AND version = 3;

COMMIT;

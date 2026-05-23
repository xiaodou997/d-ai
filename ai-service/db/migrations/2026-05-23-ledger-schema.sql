-- ============================================================================
-- 2026-05-23: 分账层（Ledger）schema
--
-- 背景：
--   Phase 0 把内部金额精度升到 micro-credit 后，单次请求的真实成本可以低至
--   0.0001 积分。每请求都调 URM 扣款不仅 RPC 量爆炸，而且 URM 端的最小扣减
--   单位是整数积分（=分），会反复发生 "扣不动" 的小数残留。
--
--   分账层把 micro-credit 的小数累积在 ai-service 本地账本里，按用户/租户
--   聚合，达到阈值或定时器触发时 floor 到整数积分一次性调 URM Consume，
--   余数继续留在账本里直到下次聚合。
--
-- 设计：
--   ai_user_credit_ledger : 每 (owner_type, tenant_id, user_id) 一行的聚合表。
--                           pending_micro 是累计未结算金额；settled_micro 是
--                           累计已结算金额（审计用）。settle_window_id 是当前
--                           结算窗口的幂等键，避免重试时双扣。
--
--   ai_usage_logs 扩展    : 加 settled_event_id / settled_at 字段，便于把每条
--                           请求明细回链到 URM 的聚合 event_id；billing_status
--                           枚举 Phase 2 起新增 'pending_settle' / 'settled'。
-- ============================================================================

BEGIN;

CREATE TABLE IF NOT EXISTS ai_user_credit_ledger (
  id                          BIGSERIAL    PRIMARY KEY,
  owner_type                  TEXT         NOT NULL CHECK (owner_type IN ('user', 'tenant')),
  tenant_id                   TEXT         NOT NULL,
  user_id                     TEXT         NOT NULL DEFAULT '',   -- owner_type='tenant' 时为 ''
  -- 拆分租户/用户两侧的未结算金额，结算时分别传给 URM Consume 的 tenantAmount/userAmount
  pending_tenant_micro        BIGINT       NOT NULL DEFAULT 0 CHECK (pending_tenant_micro >= 0),
  pending_user_micro          BIGINT       NOT NULL DEFAULT 0 CHECK (pending_user_micro >= 0),
  settled_tenant_micro        BIGINT       NOT NULL DEFAULT 0 CHECK (settled_tenant_micro >= 0),
  settled_user_micro          BIGINT       NOT NULL DEFAULT 0 CHECK (settled_user_micro >= 0),
  -- 当前正在执行的结算窗口 ULID；非空表示有结算在途，幂等键 = 'ai-settle-' || settle_window_id
  settle_window_id            TEXT,
  settle_window_tenant_micro  BIGINT       NOT NULL DEFAULT 0 CHECK (settle_window_tenant_micro >= 0),
  settle_window_user_micro    BIGINT       NOT NULL DEFAULT 0 CHECK (settle_window_user_micro >= 0),
  settle_window_opened_at     TIMESTAMPTZ,
  last_settled_at             TIMESTAMPTZ,
  created_at                  TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at                  TIMESTAMPTZ  NOT NULL DEFAULT now(),
  CONSTRAINT ai_user_credit_ledger_unique UNIQUE (owner_type, tenant_id, user_id)
);

-- 待结算扫描：定时器/阈值触发用，跳过已被锁住的行（SKIP LOCKED）
CREATE INDEX IF NOT EXISTS idx_ai_user_credit_ledger_pending
  ON ai_user_credit_ledger (last_settled_at NULLS FIRST)
  WHERE (pending_tenant_micro + pending_user_micro) > 0;

-- 在途结算监控：长时间未关闭的窗口可能意味着 URM 调用挂了，需要告警
CREATE INDEX IF NOT EXISTS idx_ai_user_credit_ledger_in_flight
  ON ai_user_credit_ledger (settle_window_opened_at)
  WHERE settle_window_id IS NOT NULL;

-- ai_usage_logs 扩展
ALTER TABLE ai_usage_logs
  ADD COLUMN IF NOT EXISTS settled_event_id TEXT,
  ADD COLUMN IF NOT EXISTS settled_at       TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_settled_event
  ON ai_usage_logs (settled_event_id) WHERE settled_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_unsettled
  ON ai_usage_logs (created_at)
  WHERE billing_status = 'pending_settle' AND settled_event_id IS NULL;

COMMIT;

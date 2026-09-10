-- from_version: 38
-- to_version: 39
-- created_at: 2026-09-10
-- description: restore group target manual routing priority

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 38
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 38';
    END IF;
END
$$;

-- 0029 曾把分组路由简化为纯策略选择并移除人工优先级。实践中纯策略无法表达
-- 「这个组里就是要先打 A、A 不行再打 B」的运营意图，现在把优先级加回来：
-- 同组内越小越优先；默认 100 表示全部平级，平级时回落到协议转换偏好 + 分组
-- 路由策略（balanced/cost/latency/stability）。0029 删除列时历史值已丢失且
-- 不可恢复，所有存量绑定从默认值 100 重新开始。
ALTER TABLE ai_group_targets
    ADD COLUMN priority INTEGER NOT NULL DEFAULT 100 CHECK (priority >= 0);

CREATE INDEX idx_ai_group_targets_priority
    ON ai_group_targets (group_id, priority);

UPDATE dai_schema_metadata
SET version = 39,
    updated_at = now()
WHERE singleton = TRUE AND version = 38;

COMMIT;

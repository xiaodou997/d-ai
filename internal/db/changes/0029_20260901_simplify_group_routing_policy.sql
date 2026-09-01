-- from_version: 28
-- to_version: 29
-- created_at: 2026-09-01
-- description: simplify group routing to one policy and remove target priority/weights

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 28
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 28';
    END IF;
END
$$;

ALTER TABLE ai_groups
    DROP CONSTRAINT IF EXISTS ai_groups_route_objective_strategy_check,
    DROP COLUMN IF EXISTS route_strategy,
    DROP COLUMN IF EXISTS route_objective,
    ADD COLUMN route_policy TEXT NOT NULL DEFAULT 'balanced'
        CHECK (route_policy IN ('balanced', 'cost', 'latency', 'stability'));

DROP INDEX IF EXISTS idx_ai_group_targets_group;

ALTER TABLE ai_group_targets
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS routing_weight;

CREATE INDEX IF NOT EXISTS idx_ai_group_targets_group
    ON ai_group_targets (group_id, status);

UPDATE dai_schema_metadata
SET version = 29,
    updated_at = now()
WHERE singleton = TRUE AND version = 28;

COMMIT;

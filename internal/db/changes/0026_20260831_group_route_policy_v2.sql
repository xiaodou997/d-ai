-- from_version: 25
-- to_version: 26
-- created_at: 2026-08-31
-- description: move routing policy ownership to groups and add target weights

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 25
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 25';
    END IF;
END
$$;

ALTER TABLE ai_groups
    ADD COLUMN route_strategy TEXT NOT NULL DEFAULT 'adaptive'
        CHECK (route_strategy IN ('failover', 'weighted', 'adaptive')),
    ADD COLUMN route_objective TEXT NOT NULL DEFAULT 'balanced'
        CHECK (route_objective IN ('balanced', 'cost', 'latency', 'stability')),
    ADD CONSTRAINT ai_groups_route_objective_strategy_check CHECK (
        route_strategy = 'adaptive' OR route_objective = 'balanced'
    );

ALTER TABLE ai_group_targets
    ADD COLUMN routing_weight NUMERIC(10,4) NOT NULL DEFAULT 1
        CHECK (routing_weight >= 0 AND routing_weight <> 'NaN'::numeric);

DROP TABLE IF EXISTS ai_route_score_weights;

UPDATE dai_schema_metadata
SET version = 26,
    updated_at = now()
WHERE singleton = TRUE AND version = 25;

COMMIT;

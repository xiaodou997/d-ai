-- from_version: 26
-- to_version: 27
-- created_at: 2026-09-01
-- description: add optimistic-concurrency version for group route configuration

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 26
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 26';
    END IF;
END
$$;

ALTER TABLE ai_groups
    ADD COLUMN route_policy_version BIGINT NOT NULL DEFAULT 1
        CHECK (route_policy_version > 0);

UPDATE dai_schema_metadata
SET version = 27,
    updated_at = now()
WHERE singleton = TRUE AND version = 26;

COMMIT;

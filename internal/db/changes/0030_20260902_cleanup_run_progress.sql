-- from_version: 29
-- to_version: 30
-- created_at: 2026-09-02
-- description: persist data cleanup progress between batches

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 29
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 29';
    END IF;
END
$$;

ALTER TABLE sys_data_cleanup_runs
    ADD COLUMN progress JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE dai_schema_metadata
SET version = 30,
    updated_at = now()
WHERE singleton = TRUE AND version = 29;

COMMIT;

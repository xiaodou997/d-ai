-- from_version: 37
-- to_version: 38
-- created_at: 2026-09-10
-- description: add upstream account descriptions

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 37
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 37';
    END IF;
END
$$;

-- Some databases were initialized from the short-lived v37 baseline that already
-- contained this column; existing upgraded databases do not have it.
ALTER TABLE ai_upstream_accounts
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

UPDATE dai_schema_metadata
SET version = 38,
    updated_at = now()
WHERE singleton = TRUE AND version = 37;

COMMIT;

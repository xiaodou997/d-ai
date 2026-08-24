-- from_version: 14
-- to_version: 15
-- created_at: 2026-08-24
-- description: add heartbeat, lease expiry, and owner fencing to data cleanup runs

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 14
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 14';
    END IF;
END
$$;

ALTER TABLE sys_data_cleanup_runs
    ADD COLUMN owner_id TEXT,
    ADD COLUMN heartbeat_at TIMESTAMPTZ,
    ADD COLUMN lease_until TIMESTAMPTZ;

CREATE INDEX idx_sys_data_cleanup_runs_lease
    ON sys_data_cleanup_runs (lease_until)
    WHERE status IN ('queued', 'running');

UPDATE dai_schema_metadata
SET version = 15,
    updated_at = now()
WHERE singleton = TRUE AND version = 14;

COMMIT;

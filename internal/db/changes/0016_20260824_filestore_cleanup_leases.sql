-- from_version: 15
-- to_version: 16
-- created_at: 2026-08-24
-- description: add owner fencing and reclaimable leases to file asset cleanup

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 15
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 15';
    END IF;
END
$$;

ALTER TABLE file_assets
    ADD COLUMN cleanup_owner TEXT,
    ADD COLUMN cleanup_lease_until TIMESTAMPTZ;

CREATE INDEX idx_file_assets_cleanup_lease
    ON file_assets (expires_at, cleanup_lease_until);

UPDATE dai_schema_metadata
SET version = 16,
    updated_at = now()
WHERE singleton = TRUE AND version = 15;

COMMIT;

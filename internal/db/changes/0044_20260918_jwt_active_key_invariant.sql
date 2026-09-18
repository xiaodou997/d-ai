-- from_version: 43
-- to_version: 44
-- created_at: 2026-09-18
-- description: enforce a single active JWT signing key

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
DECLARE
    active_count BIGINT;
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 43
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 43';
    END IF;

    SELECT COUNT(*)
    INTO active_count
    FROM auth_signing_keys
    WHERE status = 'active';

    IF active_count > 1 THEN
        RAISE EXCEPTION 'cannot enforce JWT active-key invariant: found % active keys', active_count;
    END IF;
END
$$;

CREATE UNIQUE INDEX ux_auth_signing_keys_single_active
    ON auth_signing_keys (status)
    WHERE status = 'active';

UPDATE dai_schema_metadata
SET version = 44,
    updated_at = now()
WHERE singleton = TRUE AND version = 43;

COMMIT;

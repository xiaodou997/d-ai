-- from_version: 44
-- to_version: 45
-- created_at: 2026-09-19
-- description: persist async JWT authentication references

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 44
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 44';
    END IF;
END
$$;

ALTER TABLE ai_async_tasks
    ADD COLUMN auth_user_id TEXT,
    ADD COLUMN auth_user_type INTEGER CHECK (auth_user_type IS NULL OR auth_user_type IN (1, 2, 3, 4)),
    ADD COLUMN auth_session_id UUID,
    ADD COLUMN auth_credential_version BIGINT CHECK (auth_credential_version IS NULL OR auth_credential_version > 0);

UPDATE dai_schema_metadata
SET version = 45,
    updated_at = now()
WHERE singleton = TRUE AND version = 44;

COMMIT;

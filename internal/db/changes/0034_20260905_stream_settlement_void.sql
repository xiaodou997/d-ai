-- from_version: 33
-- to_version: 34
-- created_at: 2026-09-05
-- description: allow customer-charge voids for interrupted streams

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 33
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 33';
    END IF;
END
$$;

ALTER TABLE ai_usage_logs
    DROP CONSTRAINT ai_usage_logs_billing_status_check;

ALTER TABLE ai_usage_logs
    ADD CONSTRAINT ai_usage_logs_billing_status_check
    CHECK (billing_status IN ('free', 'pending', 'settled', 'failed', 'void'));

UPDATE dai_schema_metadata
SET version = 34,
    updated_at = now()
WHERE singleton = TRUE AND version = 33;

COMMIT;

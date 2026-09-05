-- from_version: 34
-- to_version: 35
-- created_at: 2026-09-05
-- description: persist provider, delivery, cancellation and summary states

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 34
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 34';
    END IF;
END
$$;

ALTER TABLE ai_usage_logs
    ADD COLUMN provider_terminal_state TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN client_delivery_state TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN cancellation_origin TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN billing_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN response_summary_state TEXT NOT NULL DEFAULT 'unavailable';

ALTER TABLE ai_usage_logs
    ADD CONSTRAINT ai_usage_logs_provider_terminal_state_check
        CHECK (provider_terminal_state IN ('unknown', 'completed', 'failed', 'cancelled', 'incomplete')),
    ADD CONSTRAINT ai_usage_logs_client_delivery_state_check
        CHECK (client_delivery_state IN ('unknown', 'complete', 'disconnected', 'write_failed')),
    ADD CONSTRAINT ai_usage_logs_cancellation_origin_check
        CHECK (cancellation_origin IN ('none', 'client', 'provider', 'gateway', 'unknown')),
    ADD CONSTRAINT ai_usage_logs_response_summary_state_check
        CHECK (response_summary_state IN ('unavailable', 'empty', 'partial', 'complete'));

UPDATE dai_schema_metadata
SET version = 35,
    updated_at = now()
WHERE singleton = TRUE AND version = 34;

COMMIT;

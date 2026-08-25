-- from_version: 17
-- to_version: 18
-- created_at: 2026-08-25
-- description: index durable payment sweep retry health queries

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 17
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 17';
    END IF;
END
$$;

CREATE INDEX idx_pay_orders_sweep_retry_health
    ON pay_orders (sweep_last_attempt_at)
    WHERE status IN ('created', 'paying', 'expired') AND sweep_attempts > 0;

UPDATE dai_schema_metadata
SET version = 18,
    updated_at = now()
WHERE singleton = TRUE AND version = 17;

COMMIT;

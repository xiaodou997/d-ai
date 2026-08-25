-- from_version: 16
-- to_version: 17
-- created_at: 2026-08-24
-- description: persist payment sweep retry backoff and failure details

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 16
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 16';
    END IF;
END
$$;

ALTER TABLE pay_orders
    ADD COLUMN sweep_attempts INTEGER NOT NULL DEFAULT 0 CHECK (sweep_attempts >= 0),
    ADD COLUMN sweep_next_attempt_at TIMESTAMPTZ,
    ADD COLUMN sweep_last_attempt_at TIMESTAMPTZ,
    ADD COLUMN sweep_last_error TEXT;

DROP INDEX idx_pay_orders_sweep;
CREATE INDEX idx_pay_orders_sweep
    ON pay_orders (status, sweep_next_attempt_at, expires_at)
    WHERE status IN ('created', 'paying', 'expired');

UPDATE dai_schema_metadata
SET version = 17,
    updated_at = now()
WHERE singleton = TRUE AND version = 16;

COMMIT;

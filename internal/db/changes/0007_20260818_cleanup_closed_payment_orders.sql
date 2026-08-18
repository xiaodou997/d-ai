-- from_version: 6
-- to_version: 7
-- created_at: 2026-08-18
-- description: index stale closed unpaid payment-order cleanup

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 6
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 6';
    END IF;
END
$$;

CREATE INDEX idx_pay_orders_closed_cleanup ON pay_orders (updated_at)
    WHERE status = 'closed' AND fulfillment_status = 'pending';

UPDATE dai_schema_metadata
SET version = 7,
    updated_at = now()
WHERE singleton = TRUE AND version = 6;

COMMIT;

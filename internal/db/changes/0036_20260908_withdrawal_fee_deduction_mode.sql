-- from_version: 35
-- to_version: 36
-- created_at: 2026-09-08
-- description: support choosing the withdrawal fee deduction source

BEGIN;
SELECT pg_advisory_xact_lock(82624001);
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM dai_schema_metadata WHERE singleton = TRUE AND version = 35) THEN
        RAISE EXCEPTION 'expected D-AI schema version 35';
    END IF;
END
$$;
ALTER TABLE pay_withdrawals
    ADD COLUMN fee_deduction_mode TEXT NOT NULL DEFAULT 'balance',
    ADD CONSTRAINT pay_withdrawals_fee_deduction_mode_check
        CHECK (fee_deduction_mode IN ('balance', 'payout'));
UPDATE dai_schema_metadata SET version = 36, updated_at = now()
WHERE singleton = TRUE AND version = 35;
COMMIT;

-- from_version: 7
-- to_version: 8
-- created_at: 2026-08-18
-- description: fold AI financial events into usage records and remove bill_events

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 7
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 7';
    END IF;
END
$$;

-- The usage row is the durable AI request fact. These columns retain the
-- useful settlement/refund state before the old aggregate event table goes.
ALTER TABLE ai_usage_logs
    ADD COLUMN settlement_error TEXT,
    ADD COLUMN refund_status TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN refund_reason TEXT,
    ADD COLUMN refund_operator_id TEXT,
    ADD COLUMN refunded_at TIMESTAMPTZ;

ALTER TABLE ai_sub_orders
    ADD COLUMN debit_reference TEXT,
    ADD COLUMN debited_at TIMESTAMPTZ;

-- Preserve only settlement/refund state on the matching usage row. Usage log
-- amounts are the authoritative billing snapshot; do not let an inconsistent
-- legacy event amount make this destructive migration violate charge checks.
UPDATE ai_usage_logs u
SET billing_status = CASE e.status
        WHEN 'succeeded' THEN 'settled'
        WHEN 'refunded' THEN 'settled'
        WHEN 'pending' THEN 'pending'
        ELSE 'failed'
    END,
    settlement_error = CASE
        WHEN e.status IN ('cancelled', 'released') THEN NULLIF(e.terminal_note, '')
        ELSE u.settlement_error
    END,
    settled_at = CASE
        WHEN e.status IN ('succeeded', 'refunded') THEN COALESCE(u.settled_at, e.finished_at, e.created_at)
        ELSE u.settled_at
    END,
    refund_status = CASE WHEN e.status = 'refunded' THEN 'refunded' ELSE u.refund_status END,
    refund_reason = CASE WHEN e.status = 'refunded' THEN NULLIF(e.terminal_note, '') ELSE u.refund_reason END,
    refunded_at = CASE WHEN e.status = 'refunded' THEN COALESCE(e.finished_at, e.created_at) ELSE u.refunded_at END
FROM bill_events e
WHERE e.event_type = 'charge'
  AND e.event_id = COALESCE(u.billing_event_id, u.settled_event_id);

-- Normalize legacy billing states. A usage record has one request status and
-- one settlement status; refund is an independent state above it.
UPDATE ai_usage_logs SET billing_status = 'settled' WHERE billing_status IN ('confirmed', 'settled');
UPDATE ai_usage_logs SET billing_status = 'pending' WHERE billing_status IN ('pending', 'pending_settle');
UPDATE ai_usage_logs SET billing_status = 'failed' WHERE billing_status IN ('frozen', 'cancelled');
UPDATE ai_usage_logs SET billing_status = 'free'
WHERE billing_status NOT IN ('free', 'pending', 'settled', 'failed')
  AND tenant_payable = 0 AND user_charged = 0;
UPDATE ai_usage_logs SET billing_status = 'failed'
WHERE billing_status NOT IN ('free', 'pending', 'settled', 'failed');

-- A subscription order is its own business transaction. Rebuild a stable,
-- order-scoped debit reference for paid/in-flight historical orders, then drop
-- the event link. Using order_no also avoids inheriting duplicate/corrupt event
-- IDs from the disposable legacy journal.
UPDATE ai_sub_orders
SET debit_reference = 'subscription:' || order_no,
    debited_at = COALESCE(paid_at, updated_at)
WHERE status IN ('paid', 'deducting')
  AND debit_reference IS NULL;

ALTER TABLE ai_usage_logs
    ADD CONSTRAINT ai_usage_logs_billing_status_check
        CHECK (billing_status IN ('free', 'pending', 'settled', 'failed')),
    ADD CONSTRAINT ai_usage_logs_refund_status_check
        CHECK (
            (refund_status = 'none' AND refunded_at IS NULL)
            OR (refund_status = 'refunded' AND refunded_at IS NOT NULL)
        );

CREATE UNIQUE INDEX uq_ai_sub_orders_debit_reference
    ON ai_sub_orders (debit_reference)
    WHERE debit_reference IS NOT NULL;

DROP INDEX idx_ai_usage_logs_billing_event;

ALTER TABLE ledger_credit_leases DROP COLUMN settled_event_id;
ALTER TABLE ai_usage_logs DROP COLUMN billing_event_id, DROP COLUMN settled_event_id;
ALTER TABLE ai_async_tasks DROP COLUMN billing_event_id;
ALTER TABLE ai_billing_settlement_batches DROP COLUMN billing_event_id;
ALTER TABLE ai_sub_orders DROP COLUMN billing_event_id;

DROP TABLE bill_events;

UPDATE dai_schema_metadata
SET version = 8,
    updated_at = now()
WHERE singleton = TRUE AND version = 7;

COMMIT;

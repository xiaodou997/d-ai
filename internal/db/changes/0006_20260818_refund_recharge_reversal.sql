-- from_version: 5
-- to_version: 6
-- created_at: 2026-08-18
-- description: record completed refunds and reverse every linked recharge grant

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 5
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 5';
    END IF;
END
$$;

ALTER TABLE pay_orders
    ADD COLUMN refund_status TEXT NOT NULL DEFAULT 'none'
        CHECK (refund_status IN ('none', 'refunded')),
    ADD CONSTRAINT pay_orders_refund_pairing_check CHECK (
        refund_status = 'none'
        OR (refund_status = 'refunded' AND status = 'paid' AND fulfillment_status = 'reversed')
    );

-- Earlier expiry settlement overwrote consumed_micro with granted_micro. Keep
-- the new field nullable so those legacy rows remain explicitly unreconciled;
-- refund reversal refuses them instead of guessing and double-debiting funds.
ALTER TABLE bill_credit_lots
    ADD COLUMN expired_unused_micro BIGINT CHECK (
        expired_unused_micro IS NULL
        OR (expired_unused_micro >= 0 AND expired_unused_micro <= granted_micro)
    );

CREATE TABLE pay_refunds (
    id BIGSERIAL PRIMARY KEY,
    refund_id TEXT NOT NULL UNIQUE,
    payment_order_id TEXT NOT NULL UNIQUE REFERENCES pay_orders (order_id),
    refund_method TEXT NOT NULL CHECK (refund_method IN ('wechat', 'offline')),
    refund_reference TEXT NOT NULL CHECK (btrim(refund_reference) <> ''),
    channel_refund_id TEXT,
    refund_amount_minor BIGINT NOT NULL CHECK (refund_amount_minor > 0),
    status TEXT NOT NULL DEFAULT 'completed' CHECK (status = 'completed'),
    refunded_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
    note TEXT,
    operator_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pay_refunds_channel_reference_check CHECK (
        refund_method = 'offline'
        OR (channel_refund_id IS NOT NULL AND btrim(channel_refund_id) <> '')
    ),
    UNIQUE (refund_method, refund_reference)
);

CREATE UNIQUE INDEX uq_pay_refunds_channel_refund_id ON pay_refunds (channel_refund_id)
    WHERE channel_refund_id IS NOT NULL;
CREATE INDEX idx_pay_refunds_refunded_at ON pay_refunds (refunded_at DESC);

CREATE TABLE bill_refund_reversal_effects (
    id BIGSERIAL PRIMARY KEY,
    reversal_id TEXT NOT NULL UNIQUE,
    refund_id TEXT NOT NULL REFERENCES pay_refunds (refund_id),
    recharge_order_id TEXT NOT NULL UNIQUE REFERENCES bill_recharge_orders (order_id),
    account_id TEXT NOT NULL REFERENCES bill_accounts (account_id),
    credit_amount_micro BIGINT NOT NULL CHECK (credit_amount_micro > 0),
    available_reclaimed_micro BIGINT NOT NULL CHECK (available_reclaimed_micro >= 0),
    non_available_debit_micro BIGINT NOT NULL CHECK (non_available_debit_micro >= 0),
    expired_amount_micro BIGINT NOT NULL CHECK (expired_amount_micro >= 0),
    account_debit_micro BIGINT NOT NULL CHECK (account_debit_micro >= 0),
    balance_after_micro BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bill_refund_reversal_effects_components_check CHECK (
        available_reclaimed_micro + non_available_debit_micro + expired_amount_micro = credit_amount_micro
        AND available_reclaimed_micro + non_available_debit_micro = account_debit_micro
    )
);

CREATE INDEX idx_bill_refund_reversal_effects_refund ON bill_refund_reversal_effects (refund_id);

ALTER TABLE pay_cash_ledger DROP CONSTRAINT pay_cash_ledger_txn_type_check;
ALTER TABLE pay_cash_ledger
    ADD CONSTRAINT pay_cash_ledger_txn_type_check
    CHECK (txn_type IN ('topup_income', 'refund_reversal', 'consumption', 'withdraw', 'adjust'));

UPDATE dai_schema_metadata
SET version = 6,
    updated_at = now()
WHERE singleton = TRUE AND version = 5;

COMMIT;

-- from_version: 4
-- to_version: 5
-- created_at: 2026-08-18
-- description: link payment and balance orders and track fulfillment separately from payment

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 4
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 4';
    END IF;
END
$$;

ALTER TABLE bill_recharge_orders
    ADD COLUMN payment_order_id TEXT,
    ADD COLUMN reversed_amount_micro BIGINT NOT NULL DEFAULT 0 CHECK (reversed_amount_micro >= 0),
    ADD COLUMN lost_amount_micro BIGINT NOT NULL DEFAULT 0 CHECK (lost_amount_micro >= 0);

ALTER TABLE pay_orders
    ADD COLUMN fulfillment_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (fulfillment_status IN ('pending', 'credited', 'partially_reversed', 'reversed'));

-- The primary balance grant has always been recorded on pay_orders.
UPDATE bill_recharge_orders r
SET payment_order_id = p.order_id
FROM pay_orders p
WHERE p.balance_order_id = r.order_id;

-- A user payment also grants tenant income. Version 4 made transaction_id a
-- unique and validated historical link for that secondary grant.
UPDATE bill_recharge_orders r
SET payment_order_id = p.order_id
FROM pay_orders p
WHERE r.order_type = 'user_topup_income'
  AND r.payment_ref = p.transaction_id
  AND p.scene = 'user_topup'
  AND p.status = 'paid';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pay_orders p
        LEFT JOIN bill_recharge_orders r ON r.order_id = p.balance_order_id
        WHERE p.status = 'paid'
          AND (p.balance_order_id IS NULL OR r.order_id IS NULL OR r.payment_order_id <> p.order_id)
    ) THEN
        RAISE EXCEPTION 'paid payment order is missing its primary balance grant';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM bill_recharge_orders
        WHERE order_type IN ('online_user_topup', 'online_tenant_topup', 'user_topup_income')
          AND payment_order_id IS NULL
    ) THEN
        RAISE EXCEPTION 'online recharge order cannot be linked to a payment order';
    END IF;
END
$$;

UPDATE pay_orders
SET fulfillment_status = 'credited'
WHERE status = 'paid';

ALTER TABLE pay_orders
    ADD CONSTRAINT pay_orders_fulfillment_pairing_check CHECK (
        (status = 'paid' AND fulfillment_status IN ('credited', 'partially_reversed', 'reversed'))
        OR (status <> 'paid' AND fulfillment_status = 'pending')
    );

ALTER TABLE bill_recharge_orders
    ADD CONSTRAINT bill_recharge_orders_payment_order_fk
    FOREIGN KEY (payment_order_id) REFERENCES pay_orders (order_id),
    ADD CONSTRAINT bill_recharge_orders_payment_link_check CHECK (
        (order_type IN ('online_user_topup', 'online_tenant_topup', 'user_topup_income') AND payment_order_id IS NOT NULL)
        OR (order_type IN ('platform_to_tenant', 'tenant_to_user') AND payment_order_id IS NULL)
    ),
    ADD CONSTRAINT bill_recharge_orders_reversal_amount_check CHECK (
        reversed_amount_micro + lost_amount_micro <= credit_amount
    );

CREATE INDEX idx_bill_recharge_orders_payment_order ON bill_recharge_orders (payment_order_id)
    WHERE payment_order_id IS NOT NULL;

UPDATE dai_schema_metadata
SET version = 5,
    updated_at = now()
WHERE singleton = TRUE AND version = 4;

COMMIT;

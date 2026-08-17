-- from_version: 3
-- to_version: 4
-- created_at: 2026-08-17
-- description: repair legacy user-topup tenant income omitted from the unified balance
--
-- Before the unified balance model, user top-up income was written to
-- pay_cash_ledger and a retired pay_cash_accounts balance. The transition to
-- bill_accounts did not carry that retired balance forward. This migration
-- uses paid payment orders plus their immutable cash-ledger entries as the
-- evidence for repair, then creates the missing recharge order and balance lot.

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 3
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 3';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM bill_recharge_orders
        WHERE order_type = 'user_topup_income' AND payment_ref IS NOT NULL
        GROUP BY payment_ref
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'duplicate user_topup_income payment_ref values require manual reconciliation';
    END IF;

    -- Existing income records are financial history. Never guess how to repair
    -- a partial, reversed, wrong-tenant or wrong-amount record.
    IF EXISTS (
        SELECT 1
        FROM pay_orders p
        JOIN bill_recharge_orders r
          ON r.order_type = 'user_topup_income'
         AND r.payment_ref = p.transaction_id
        WHERE p.scene = 'user_topup'
          AND p.status = 'paid'
          AND p.tenant_income_micro_usd > 0
        GROUP BY p.order_id, p.tenant_id, p.tenant_income_micro_usd
        HAVING COUNT(*) <> 1
            OR COUNT(*) FILTER (WHERE r.status = 'active') <> 1
            OR SUM(r.credit_amount) <> p.tenant_income_micro_usd
            OR BOOL_OR(r.tenant_id <> p.tenant_id)
    ) THEN
        RAISE EXCEPTION 'existing user_topup_income records disagree with paid payment orders';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pay_orders p
        WHERE p.scene = 'user_topup'
          AND p.status = 'paid'
          AND p.tenant_income_micro_usd > 0
          AND (p.transaction_id IS NULL OR p.transaction_id = '')
    ) THEN
        RAISE EXCEPTION 'paid user top-up is missing transaction_id';
    END IF;

    -- A legacy cash entry proves the tenant income was recognized at payment
    -- settlement time. Missing or conflicting evidence must be reviewed rather
    -- than silently credited.
    IF EXISTS (
        SELECT 1
        FROM pay_orders p
        WHERE p.scene = 'user_topup'
          AND p.status = 'paid'
          AND p.tenant_income_micro_usd > 0
          AND NOT EXISTS (
              SELECT 1 FROM bill_recharge_orders r
              WHERE r.order_type = 'user_topup_income'
                AND r.payment_ref = p.transaction_id
          )
          AND NOT EXISTS (
              SELECT 1 FROM pay_cash_ledger l
              WHERE l.idempotency_key = 'wxpay:' || p.out_trade_no
                AND l.tenant_id = p.tenant_id
                AND l.txn_type = 'topup_income'
                AND l.amount_micro_usd = p.tenant_income_micro_usd
          )
    ) THEN
        RAISE EXCEPTION 'missing tenant cash-ledger evidence for user top-up repair';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pay_orders p
        WHERE p.scene = 'user_topup'
          AND p.status = 'paid'
          AND p.tenant_income_micro_usd > 0
          AND NOT EXISTS (
              SELECT 1 FROM bill_recharge_orders r
              WHERE r.order_type = 'user_topup_income'
                AND r.payment_ref = p.transaction_id
          )
          AND NOT EXISTS (
              SELECT 1 FROM bill_accounts b
              WHERE b.account_id = p.tenant_id AND b.account_kind = 1
          )
    ) THEN
        RAISE EXCEPTION 'tenant account is missing for user top-up repair';
    END IF;
END
$$;

DO $$
DECLARE
    item RECORD;
    repair_order_id TEXT;
    repair_lot_id TEXT;
    balance_before BIGINT;
    lot_micro BIGINT;
    repaired_orders INTEGER := 0;
    repaired_micro BIGINT := 0;
BEGIN
    FOR item IN
        SELECT p.order_id,
               p.out_trade_no,
               p.tenant_id,
               p.payment_amount_minor,
               p.tenant_income_micro_usd,
               p.transaction_id,
               COALESCE(p.paid_at, p.created_at) AS income_time
        FROM pay_orders p
        WHERE p.scene = 'user_topup'
          AND p.status = 'paid'
          AND p.tenant_income_micro_usd > 0
          AND NOT EXISTS (
              SELECT 1 FROM bill_recharge_orders r
              WHERE r.order_type = 'user_topup_income'
                AND r.payment_ref = p.transaction_id
          )
        ORDER BY COALESCE(p.paid_at, p.created_at), p.order_id
    LOOP
        repair_order_id := 'ORD_REPAIR_0004_' || SUBSTRING(md5(item.order_id), 1, 24);
        repair_lot_id := 'LOT_REPAIR_0004_' || SUBSTRING(md5(item.order_id), 1, 24);

        INSERT INTO bill_recharge_orders (
            order_id, order_type, tenant_id, user_id, credit_amount,
            paid_amount, payment_ref, expires_at, operator_id, note,
            status, created_at
        ) VALUES (
            repair_order_id, 'user_topup_income', item.tenant_id, NULL,
            item.tenant_income_micro_usd, item.payment_amount_minor,
            item.transaction_id, NULL, 'system:data-repair:0004',
            '补记历史用户在线充值租户收入，支付订单 ' || item.order_id,
            'active', item.income_time
        );

        UPDATE bill_accounts
        SET balance_micro = balance_micro + item.tenant_income_micro_usd,
            updated_at = now()
        WHERE account_id = item.tenant_id AND account_kind = 1
        RETURNING balance_micro - item.tenant_income_micro_usd
        INTO balance_before;

        IF NOT FOUND THEN
            RAISE EXCEPTION 'tenant account % disappeared during user top-up repair', item.tenant_id;
        END IF;

        -- Match ledger.Grant: the part absorbed by an existing negative balance
        -- is already consumed and must not reappear as a spendable lot.
        lot_micro := item.tenant_income_micro_usd + LEAST(balance_before, 0);
        IF lot_micro > 0 THEN
            INSERT INTO bill_credit_lots (
                lot_id, account_id, granted_micro, consumed_micro, expires_at,
                source, recharge_order_id, created_at, updated_at
            ) VALUES (
                repair_lot_id, item.tenant_id, lot_micro, 0, NULL,
                'USER_TOPUP_INCOME', repair_order_id, item.income_time, now()
            );
        END IF;

        repaired_orders := repaired_orders + 1;
        repaired_micro := repaired_micro + item.tenant_income_micro_usd;
    END LOOP;

    RAISE NOTICE 'repaired % legacy user top-up tenant-income orders, total % micro-USD',
        repaired_orders, repaired_micro;
END
$$;

-- Every paid user top-up with tenant income must now have one exact, active
-- tenant-income grant. This also makes the postcondition executable in CI.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pay_orders p
        LEFT JOIN bill_recharge_orders r
          ON r.order_type = 'user_topup_income'
         AND r.payment_ref = p.transaction_id
        WHERE p.scene = 'user_topup'
          AND p.status = 'paid'
          AND p.tenant_income_micro_usd > 0
        GROUP BY p.order_id, p.tenant_income_micro_usd
        HAVING COUNT(r.id) <> 1
            OR COUNT(r.id) FILTER (WHERE r.status = 'active') <> 1
            OR COALESCE(SUM(r.credit_amount), 0) <> p.tenant_income_micro_usd
    ) THEN
        RAISE EXCEPTION 'user top-up tenant-income repair postcondition failed';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM bill_accounts b
        LEFT JOIN bill_credit_lots l
          ON l.account_id = b.account_id
         AND l.expired_at IS NULL
         AND l.revoked_at IS NULL
        GROUP BY b.account_id, b.balance_micro
        HAVING GREATEST(b.balance_micro, 0)
            <> COALESCE(SUM(l.granted_micro - l.consumed_micro), 0)
    ) THEN
        RAISE EXCEPTION 'bill account and active lot balances disagree after repair';
    END IF;
END
$$;

CREATE UNIQUE INDEX uq_bill_recharge_orders_user_topup_income_payment_ref
    ON bill_recharge_orders (payment_ref)
    WHERE order_type = 'user_topup_income' AND payment_ref IS NOT NULL;

UPDATE dai_schema_metadata
SET version = 4,
    updated_at = now()
WHERE singleton = TRUE AND version = 3;

COMMIT;

-- Post-migration audit:
--
--   SELECT p.order_id, p.tenant_id, p.tenant_income_micro_usd,
--          r.order_id AS income_order_id, r.credit_amount
--   FROM pay_orders p
--   LEFT JOIN bill_recharge_orders r
--     ON r.order_type = 'user_topup_income' AND r.payment_ref = p.transaction_id
--   WHERE p.scene = 'user_topup' AND p.status = 'paid'
--   ORDER BY p.paid_at, p.order_id;

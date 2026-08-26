-- Database ownership contract for the split runtime/billing connections.
--
-- Apply explicitly as a database owner/superuser:
--   psql -v schema_name=public -v runtime_role=dai \
--     -v billing_role=dai_billing -f internal/db/ownership.sql
--
-- Roles must already exist. This file deliberately does not create LOGIN roles
-- or handle passwords; credentials belong in the deployment secret manager.
-- The application must use a separate billing connection before this contract
-- is applied to a live deployment.

\set ON_ERROR_STOP on

BEGIN;

SELECT set_config('dai.ownership.schema', :'schema_name', false);
SELECT set_config('dai.ownership.runtime_role', :'runtime_role', false);
SELECT set_config('dai.ownership.billing_role', :'billing_role', false);

DO $$
DECLARE
    target_schema TEXT := current_setting('dai.ownership.schema');
    runtime_name  TEXT := current_setting('dai.ownership.runtime_role');
    billing_name  TEXT := current_setting('dai.ownership.billing_role');
    table_name    TEXT;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = runtime_name) THEN
        RAISE EXCEPTION 'runtime role % does not exist', runtime_name;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = billing_name) THEN
        RAISE EXCEPTION 'billing role % does not exist', billing_name;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = target_schema) THEN
        RAISE EXCEPTION 'schema % does not exist', target_schema;
    END IF;

    FOREACH table_name IN ARRAY ARRAY[
        'bill_accounts',
        'bill_credit_lots',
        'bill_charge_outbox',
        'bill_recharge_orders',
        'bill_refund_reversal_effects',
        'pay_orders',
        'pay_refunds',
        'pay_cash_ledger',
        'pay_withdrawals',
        'pay_tenant_settings',
        'pay_wechat_config',
        'ledger_credit_leases'
    ] LOOP
        IF to_regclass(format('%I.%I', target_schema, table_name)) IS NULL THEN
            RAISE EXCEPTION 'ownership contract table %.% does not exist', target_schema, table_name;
        END IF;
    END LOOP;
END
$$;

REVOKE ALL ON SCHEMA :"schema_name" FROM PUBLIC;
GRANT USAGE ON SCHEMA :"schema_name" TO :"runtime_role", :"billing_role";

SET search_path TO :"schema_name";

-- Runtime remains able to serve read paths and write ordinary control/runtime
-- facts. The ledger/workflow tables below are removed from its DML surface.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA :"schema_name" TO :"runtime_role";
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_name" TO :"runtime_role";

-- Billing transactions lock/annotate identity and usage rows while updating
-- financial facts, so the billing connection gets read access to every table;
-- its write surface is still limited to the explicit financial list below.
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_name" TO :"billing_role";

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
    bill_accounts,
    bill_credit_lots,
    bill_charge_outbox,
    bill_recharge_orders,
    bill_refund_reversal_effects,
    pay_orders,
    pay_refunds,
    pay_cash_ledger,
    pay_withdrawals,
    pay_tenant_settings,
    pay_wechat_config,
    ledger_credit_leases
TO :"billing_role";
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_name" TO :"billing_role";

-- Runtime owns the fact creation for usage/subscription orders; billing owns
-- the settlement/refund state transitions in those same rows.
GRANT SELECT, UPDATE ON TABLE ai_usage_logs, ai_sub_orders TO :"billing_role";

REVOKE INSERT, UPDATE, DELETE ON TABLE
    bill_accounts,
    bill_credit_lots,
    bill_charge_outbox,
    bill_recharge_orders,
    bill_refund_reversal_effects,
    pay_orders,
    pay_refunds,
    pay_cash_ledger,
    pay_withdrawals,
    pay_tenant_settings,
    pay_wechat_config,
    ledger_credit_leases
FROM :"runtime_role";

-- The gateway records a usage fact and its durable settlement intent in one
-- transaction. It may enqueue, but only billing may consume or mutate it.
GRANT INSERT ON TABLE bill_charge_outbox TO :"runtime_role";

-- Ownership is the hard boundary: a future GRANT cannot accidentally make the
-- runtime role the owner of ledger state. New billing tables must be added to
-- this explicit list and re-run through the same release review.
ALTER TABLE bill_accounts OWNER TO :"billing_role";
ALTER TABLE bill_credit_lots OWNER TO :"billing_role";
ALTER TABLE bill_charge_outbox OWNER TO :"billing_role";
ALTER TABLE bill_recharge_orders OWNER TO :"billing_role";
ALTER TABLE bill_refund_reversal_effects OWNER TO :"billing_role";
ALTER TABLE pay_orders OWNER TO :"billing_role";
ALTER TABLE pay_refunds OWNER TO :"billing_role";
ALTER TABLE pay_cash_ledger OWNER TO :"billing_role";
ALTER TABLE pay_withdrawals OWNER TO :"billing_role";
ALTER TABLE pay_tenant_settings OWNER TO :"billing_role";
ALTER TABLE pay_wechat_config OWNER TO :"billing_role";
ALTER TABLE ledger_credit_leases OWNER TO :"billing_role";

COMMIT;

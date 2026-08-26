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
    view_name     TEXT;
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

    FOREACH view_name IN ARRAY ARRAY[
        'billing_recharge_order_projection',
        'payment_order_party_projection',
        'payment_admin_recharge_order_projection'
    ] LOOP
        IF to_regclass(format('%I.%I', target_schema, view_name)) IS NULL THEN
            RAISE EXCEPTION 'ownership contract view %.% does not exist', target_schema, view_name;
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

-- Billing transactions lock identity rows and settle runtime facts. These are
-- explicit transaction exceptions; reporting joins use the read-only views
-- below instead of broad control-plane table grants.
GRANT SELECT ON TABLE iam_accounts, iam_tenants TO :"billing_role";
GRANT SELECT, UPDATE ON TABLE ai_usage_logs, ai_sub_orders TO :"billing_role";
GRANT SELECT ON TABLE
    billing_recharge_order_projection,
    payment_order_party_projection,
    payment_admin_recharge_order_projection
TO :"runtime_role", :"billing_role";
REVOKE INSERT, UPDATE, DELETE ON TABLE
    billing_recharge_order_projection,
    payment_order_party_projection,
    payment_admin_recharge_order_projection
FROM :"runtime_role", :"billing_role";

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

-- The views run with their owner's privileges. Keep the owner aligned with
-- the billing role so transferring the source-table owners cannot make a
-- production read model fail after the migration account disconnects.
ALTER VIEW billing_recharge_order_projection OWNER TO :"billing_role";
ALTER VIEW payment_order_party_projection OWNER TO :"billing_role";
ALTER VIEW payment_admin_recharge_order_projection OWNER TO :"billing_role";

COMMIT;

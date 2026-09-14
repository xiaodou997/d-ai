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
        'bill_recharge_orders',
        'bill_refund_reversal_effects',
        'bill_repair_audits',
        'pay_orders',
        'pay_refunds',
        'pay_cash_ledger',
        'pay_withdrawals',
        'pay_tenant_settings',
        'pay_wechat_config'
    ] LOOP
        IF to_regclass(format('%I.%I', target_schema, table_name)) IS NULL THEN
            RAISE EXCEPTION 'ownership contract table %.% does not exist', target_schema, table_name;
        END IF;
    END LOOP;

    FOREACH view_name IN ARRAY ARRAY[
        'billing_recharge_order_projection',
        'payment_order_party_projection',
        'payment_admin_recharge_order_projection',
        'tenant_management_projection',
        'tenant_self_overview_projection',
        'tenant_usage_projection',
        'user_admin_end_user_projection',
        'system_recharge_projection',
        'system_identity_projection',
        'system_balance_projection',
        'system_usage_projection',
        'system_account_stats_projection',
        'tenant_income_projection'
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

-- Request payload purging is an explicit platform-admin maintenance action.
-- Keep these non-financial audit tables owned by the runtime role so it can
-- run VACUUM FULL after clearing their large TOAST fields (PostgreSQL 16 has
-- no separate MAINTAIN table privilege). Billing and ledger relations remain
-- owned by the billing role below.

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
    bill_accounts,
    bill_credit_lots,
    bill_recharge_orders,
    bill_refund_reversal_effects,
    bill_repair_audits,
    pay_orders,
    pay_refunds,
    pay_cash_ledger,
    pay_withdrawals,
    pay_tenant_settings,
    pay_wechat_config
TO :"billing_role";
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_name" TO :"billing_role";

GRANT SELECT, INSERT ON TABLE bill_repair_audits TO :"billing_role";

-- Billing transactions lock identity rows and settle runtime facts. These are
-- explicit transaction exceptions; reporting joins use the read-only views
-- below instead of broad control-plane table grants.
GRANT SELECT ON TABLE iam_accounts, iam_tenants TO :"billing_role";
GRANT SELECT, UPDATE ON TABLE bill_settlements, ai_async_tasks, ai_sub_orders, ai_api_keys, ai_sub_subscriptions TO :"billing_role";
GRANT SELECT ON TABLE ai_sub_subscriptions TO :"billing_role";
GRANT SELECT ON TABLE
    billing_recharge_order_projection,
    payment_order_party_projection,
    payment_admin_recharge_order_projection,
    tenant_management_projection,
    tenant_self_overview_projection,
    tenant_usage_projection,
    user_admin_end_user_projection,
    system_recharge_projection,
    system_identity_projection,
    system_balance_projection,
    system_usage_projection,
    system_account_stats_projection,
    tenant_income_projection
TO :"runtime_role", :"billing_role";
REVOKE INSERT, UPDATE, DELETE ON TABLE
    billing_recharge_order_projection,
    payment_order_party_projection,
    payment_admin_recharge_order_projection,
    tenant_management_projection,
    tenant_self_overview_projection,
    tenant_usage_projection,
    user_admin_end_user_projection,
    system_recharge_projection,
    system_identity_projection,
    system_balance_projection,
    system_usage_projection,
    system_account_stats_projection,
    tenant_income_projection
FROM :"runtime_role", :"billing_role";

REVOKE INSERT, UPDATE, DELETE ON TABLE
    bill_accounts,
    bill_credit_lots,
    bill_recharge_orders,
    bill_refund_reversal_effects,
    bill_repair_audits,
    pay_orders,
    pay_refunds,
    pay_cash_ledger,
    pay_withdrawals,
    pay_tenant_settings,
    pay_wechat_config
FROM :"runtime_role";
-- Runtime needs the account/credit-lot rows for admission and balance detail,
-- but reporting and payment workflow reads must go through owned projections.
REVOKE SELECT ON TABLE
    bill_recharge_orders,
    bill_refund_reversal_effects,
    bill_repair_audits,
    pay_orders,
    pay_refunds,
    pay_cash_ledger,
    pay_withdrawals,
    pay_tenant_settings,
    pay_wechat_config
FROM :"runtime_role";
REVOKE UPDATE, DELETE ON TABLE bill_repair_audits FROM :"runtime_role", :"billing_role";

-- The gateway records a usage fact and its durable settlement intent in one
-- transaction. It may enqueue, but only billing may consume or mutate it.

-- Ownership is the hard boundary: a future GRANT cannot accidentally make the
-- runtime role the owner of ledger state. New billing tables must be added to
-- this explicit list and re-run through the same release review.
ALTER TABLE bill_accounts OWNER TO :"billing_role";
ALTER TABLE bill_credit_lots OWNER TO :"billing_role";
ALTER TABLE bill_recharge_orders OWNER TO :"billing_role";
ALTER TABLE bill_refund_reversal_effects OWNER TO :"billing_role";
ALTER TABLE bill_repair_audits OWNER TO :"billing_role";
ALTER TABLE pay_orders OWNER TO :"billing_role";
ALTER TABLE pay_refunds OWNER TO :"billing_role";
ALTER TABLE pay_cash_ledger OWNER TO :"billing_role";
ALTER TABLE pay_withdrawals OWNER TO :"billing_role";
ALTER TABLE pay_tenant_settings OWNER TO :"billing_role";
ALTER TABLE pay_wechat_config OWNER TO :"billing_role";

ALTER FUNCTION bill_repair_audits_immutable() OWNER TO :"billing_role";
ALTER FUNCTION bill_requeue_parked_outbox(TEXT, TEXT, TEXT, TEXT, TEXT) OWNER TO :"billing_role";
ALTER FUNCTION bill_repair_audits_immutable() SET search_path TO :"schema_name", pg_catalog;
ALTER FUNCTION bill_requeue_parked_outbox(TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO :"schema_name", pg_catalog;
REVOKE ALL ON FUNCTION bill_repair_audits_immutable() FROM PUBLIC;
REVOKE ALL ON FUNCTION bill_requeue_parked_outbox(TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION bill_requeue_parked_outbox(TEXT, TEXT, TEXT, TEXT, TEXT) TO :"billing_role";

-- Account provisioning is triggered by runtime-owned identity inserts, but its
-- ledger row must be created under the billing owner. The function resolves
-- the trigger schema dynamically while keeping an immutable pg_catalog path.
ALTER FUNCTION bill_provision_account() OWNER TO :"billing_role";
ALTER FUNCTION bill_provision_account() SECURITY DEFINER;
ALTER FUNCTION bill_provision_account() SET search_path TO pg_catalog;
GRANT EXECUTE ON FUNCTION bill_provision_account() TO :"runtime_role", :"billing_role";

-- The views run with their owner's privileges. Keep the owner aligned with
-- the billing role so transferring the source-table owners cannot make a
-- production read model fail after the migration account disconnects.
ALTER VIEW billing_recharge_order_projection OWNER TO :"billing_role";
ALTER VIEW payment_order_party_projection OWNER TO :"billing_role";
ALTER VIEW payment_admin_recharge_order_projection OWNER TO :"billing_role";
ALTER VIEW tenant_management_projection OWNER TO :"runtime_role";
ALTER VIEW tenant_self_overview_projection OWNER TO :"runtime_role";
ALTER VIEW tenant_usage_projection OWNER TO :"runtime_role";
ALTER VIEW user_admin_end_user_projection OWNER TO :"runtime_role";
ALTER VIEW system_recharge_projection OWNER TO :"runtime_role";
ALTER VIEW system_identity_projection OWNER TO :"runtime_role";
ALTER VIEW system_balance_projection OWNER TO :"runtime_role";
ALTER VIEW system_usage_projection OWNER TO :"runtime_role";
ALTER VIEW system_account_stats_projection OWNER TO :"runtime_role";
ALTER VIEW tenant_income_projection OWNER TO :"billing_role";


GRANT CREATE ON SCHEMA :"schema_name" TO :"billing_role";
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE bill_settlements, bill_journal, bill_checkpoints, bill_daily_totals, bill_record_control, ai_request_keys, ai_requests, ai_request_attempts, ai_request_errors, ai_debug_sessions, ai_request_debug_payloads TO :"billing_role";
REVOKE INSERT, UPDATE, DELETE ON TABLE bill_journal, bill_checkpoints, bill_record_control FROM :"runtime_role";
REVOKE UPDATE, DELETE ON TABLE bill_settlements FROM :"runtime_role";
GRANT SELECT, INSERT ON TABLE bill_settlements TO :"runtime_role";
GRANT UPDATE(delivery_interrupted) ON TABLE bill_settlements TO :"runtime_role";
ALTER FUNCTION journal_account_balance() OWNER TO :"billing_role";
ALTER FUNCTION checkpoint_new_account() OWNER TO :"billing_role";
ALTER FUNCTION journal_quota_meter() OWNER TO :"billing_role";
ALTER FUNCTION ensure_request_record_partitions(timestamptz) OWNER TO :"billing_role";
ALTER FUNCTION journal_account_balance() SECURITY DEFINER;
ALTER FUNCTION checkpoint_new_account() SECURITY DEFINER;
ALTER FUNCTION journal_quota_meter() SECURITY DEFINER;
DO $$
DECLARE item record; role_name text:=current_setting('dai.ownership.billing_role');
BEGIN
 FOR item IN SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname=current_setting('dai.ownership.schema') AND c.relkind IN ('r','p')
 AND (c.relname IN ('bill_settlements','bill_journal','bill_checkpoints','bill_daily_totals','bill_record_control','ai_requests','ai_request_attempts','ai_request_errors','ai_request_debug_payloads') OR c.relispartition AND c.relname ~ '^(bill_settlements|bill_journal|ai_requests|ai_request_attempts|ai_request_errors|ai_request_debug_payloads)_[0-9]+$') LOOP
  EXECUTE format('ALTER TABLE %I.%I OWNER TO %I',current_setting('dai.ownership.schema'),item.relname,role_name);
 END LOOP;
END $$;

GRANT SELECT ON TABLE ai_upstream_runtime_metadata TO :"runtime_role", :"billing_role";
COMMIT;

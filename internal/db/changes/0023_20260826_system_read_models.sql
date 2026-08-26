-- from_version: 22
-- to_version: 23
-- created_at: 2026-08-26
-- description: add operations dashboard read models

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 22
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 22';
    END IF;
END
$$;

CREATE VIEW system_recharge_projection AS
SELECT order_type,
       paid_amount,
       credit_amount,
       status,
       created_at
FROM bill_recharge_orders;

CREATE VIEW system_identity_projection AS
SELECT 'tenant'::text AS entity_kind,
       tenant_id AS entity_id,
       status,
       created_at
FROM iam_tenants
UNION ALL
SELECT 'user'::text AS entity_kind,
       user_id AS entity_id,
       status,
       created_at
FROM iam_accounts
WHERE user_type = 4;

CREATE VIEW system_balance_projection AS
SELECT account_kind,
       balance_micro
FROM bill_accounts;

CREATE VIEW system_usage_projection AS
SELECT tenant_id,
       request_source,
       billing_status,
       created_at,
       tenant_payable,
       user_charged
FROM ai_usage_logs;

UPDATE dai_schema_metadata
SET version = 23,
    updated_at = now()
WHERE singleton = TRUE AND version = 22;

COMMIT;

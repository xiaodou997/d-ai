-- from_version: 24
-- to_version: 25
-- created_at: 2026-08-30
-- description: move account and tenant income aggregates behind read models

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 24
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 24';
    END IF;
END
$$;

CREATE OR REPLACE FUNCTION bill_provision_account() RETURNS TRIGGER AS $$
DECLARE
    target_schema TEXT := TG_TABLE_SCHEMA;
BEGIN
    IF TG_TABLE_NAME = 'iam_tenants' THEN
        EXECUTE format('
            INSERT INTO %I.bill_accounts (account_id, account_kind, tenant_id)
            VALUES ($1, 1, $1)
            ON CONFLICT (account_id) DO NOTHING', target_schema)
        USING NEW.tenant_id;
    ELSIF NEW.user_type = 4 AND NEW.tenant_id IS NOT NULL THEN
        EXECUTE format('
            INSERT INTO %I.bill_accounts (account_id, account_kind, tenant_id)
            VALUES ($1, 2, $2)
            ON CONFLICT (account_id) DO NOTHING', target_schema)
        USING NEW.user_id, NEW.tenant_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

CREATE VIEW system_account_stats_projection AS
SELECT t.tenant_id,
       (SELECT COUNT(*)
        FROM iam_accounts u
        WHERE u.tenant_id = t.tenant_id
          AND u.user_type = 4)::bigint AS end_user_count,
       (SELECT COUNT(*)
        FROM iam_invitation_codes i
        WHERE i.tenant_id = t.tenant_id)::bigint AS invite_code_count,
       COALESCE((SELECT SUM(u.user_charged)
                 FROM ai_usage_logs u
                 WHERE u.tenant_id = t.tenant_id
                   AND u.billing_status = 'settled'), 0)::bigint AS user_deduction_micro
FROM iam_tenants t;

CREATE VIEW tenant_income_projection AS
SELECT tenant_id,
       txn_type,
       amount_micro_usd,
       created_at
FROM pay_cash_ledger
WHERE txn_type = 'topup_income'
  AND amount_micro_usd > 0;

UPDATE dai_schema_metadata
SET version = 25,
    updated_at = now()
WHERE singleton = TRUE AND version = 24;

COMMIT;

-- from_version: 20
-- to_version: 21
-- created_at: 2026-08-26
-- description: add tenant management and analytics read models

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 20
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 20';
    END IF;
END
$$;

CREATE VIEW tenant_management_projection AS
SELECT t.tenant_id,
       t.tenant_name,
       t.contact_person,
       t.contact_email,
       t.status,
       t.created_at,
       COALESCE(b.balance_micro, 0)::bigint AS balance_micro,
       (SELECT COUNT(*)
        FROM iam_accounts u
        WHERE u.tenant_id = t.tenant_id
          AND u.user_type = 4
          AND u.status NOT IN ('locked', 'inherited_disabled', 'deleted'))::bigint AS user_count
FROM iam_tenants t
LEFT JOIN bill_accounts b ON b.account_id = t.tenant_id;

CREATE VIEW tenant_self_overview_projection AS
SELECT t.tenant_id,
       (SELECT COUNT(*)
        FROM iam_accounts u
        WHERE u.tenant_id = t.tenant_id
          AND u.user_type = 4
          AND u.status <> 'deleted')::bigint AS end_user_count,
       (SELECT COUNT(*)
        FROM iam_invitation_codes i
        WHERE i.tenant_id = t.tenant_id)::bigint AS invite_code_count,
       COALESCE((SELECT SUM(GREATEST(b.balance_micro, 0))
                 FROM bill_accounts b
                 WHERE b.account_kind = 2
                   AND b.tenant_id = t.tenant_id), 0)::bigint AS user_total_balance_micro
FROM iam_tenants t;

CREATE VIEW tenant_usage_projection AS
SELECT e.tenant_id,
       e.user_id,
       COALESCE(NULLIF(u.username, ''), '已删除用户') AS username,
       COALESCE(e.request_source, '') AS request_source,
       e.created_at,
       e.billing_status,
       COALESCE(e.user_charged, 0)::bigint AS user_charged
FROM ai_usage_logs e
LEFT JOIN iam_accounts u ON u.user_id = e.user_id AND u.user_type = 4;

UPDATE dai_schema_metadata
SET version = 21,
    updated_at = now()
WHERE singleton = TRUE AND version = 20;

COMMIT;

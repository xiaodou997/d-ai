-- from_version: 21
-- to_version: 22
-- created_at: 2026-08-26
-- description: add admin end-user read model

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 21
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 21';
    END IF;
END
$$;

CREATE VIEW user_admin_end_user_projection AS
SELECT eu.user_id,
       eu.tenant_id,
       eu.username,
       eu.email,
       eu.phone,
       eu.internal_note,
       eu.nickname,
       eu.avatar,
       eu.status,
       eu.credential_state,
       eu.last_login_at,
       eu.created_at,
       COALESCE(t.tenant_name, '') AS tenant_name,
       COALESCE(b.balance_micro, 0)::bigint AS balance_micro
FROM iam_accounts eu
LEFT JOIN iam_tenants t ON eu.tenant_id = t.tenant_id
LEFT JOIN bill_accounts b ON b.account_id = eu.user_id
WHERE eu.user_type = 4
  AND eu.status <> 'deleted';

UPDATE dai_schema_metadata
SET version = 22,
    updated_at = now()
WHERE singleton = TRUE AND version = 21;

COMMIT;

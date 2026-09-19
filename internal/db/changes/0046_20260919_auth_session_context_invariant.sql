-- from_version: 45
-- to_version: 46
-- created_at: 2026-09-19
-- description: revoke auth sessions when role or tenant scope changes

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 45
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 45';
    END IF;
END
$$;

CREATE OR REPLACE FUNCTION auth_revoke_sessions_on_account_change() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status IS DISTINCT FROM OLD.status
       OR NEW.credential_version IS DISTINCT FROM OLD.credential_version
       OR NEW.user_type IS DISTINCT FROM OLD.user_type
       OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id THEN
        UPDATE auth_sessions
        SET status = 'revoked',
            revoked_at = COALESCE(revoked_at, now()),
            revoke_reason = CASE
                WHEN NEW.status IS DISTINCT FROM OLD.status THEN 'account_status_changed'
                WHEN NEW.credential_version IS DISTINCT FROM OLD.credential_version THEN 'credential_changed'
                WHEN NEW.user_type IS DISTINCT FROM OLD.user_type THEN 'account_role_changed'
                ELSE 'tenant_scope_changed'
            END,
            updated_at = now()
        WHERE user_id = NEW.user_id AND status = 'active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER trg_auth_revoke_sessions_on_account_change ON iam_accounts;
CREATE TRIGGER trg_auth_revoke_sessions_on_account_change
    AFTER UPDATE OF status, credential_version, user_type, tenant_id ON iam_accounts
    FOR EACH ROW EXECUTE FUNCTION auth_revoke_sessions_on_account_change();

UPDATE dai_schema_metadata
SET version = 46,
    updated_at = now()
WHERE singleton = TRUE AND version = 45;

COMMIT;

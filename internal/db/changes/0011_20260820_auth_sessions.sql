-- Schema 10 -> 11: stateful refresh-token sessions with rotation and replay revocation.
BEGIN;

DO $$
BEGIN
    IF (SELECT version FROM dai_schema_metadata WHERE singleton = TRUE FOR UPDATE) IS DISTINCT FROM 10 THEN
        RAISE EXCEPTION 'migration 0011 requires schema version 10';
    END IF;
END
$$;

ALTER TABLE iam_accounts
    ADD COLUMN credential_version BIGINT NOT NULL DEFAULT 1 CHECK (credential_version > 0);

CREATE TABLE auth_sessions (
    session_id UUID PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES iam_accounts (user_id) ON DELETE CASCADE,
    credential_version BIGINT NOT NULL CHECK (credential_version > 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoke_reason TEXT,
    last_refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_sessions_user_status ON auth_sessions (user_id, status);
CREATE INDEX idx_auth_sessions_expires ON auth_sessions (expires_at) WHERE status = 'active';

CREATE TABLE auth_refresh_tokens (
    token_hash BYTEA PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES auth_sessions (session_id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'consumed')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    replaced_by_hash BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_auth_refresh_tokens_active_session
    ON auth_refresh_tokens (session_id) WHERE status = 'active';
CREATE INDEX idx_auth_refresh_tokens_session ON auth_refresh_tokens (session_id);

CREATE FUNCTION auth_revoke_sessions_on_account_change() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status <> OLD.status OR NEW.credential_version <> OLD.credential_version THEN
        UPDATE auth_sessions
        SET status = 'revoked',
            revoked_at = COALESCE(revoked_at, now()),
            revoke_reason = CASE
                WHEN NEW.status <> OLD.status THEN 'account_status_changed'
                ELSE 'credential_changed'
            END,
            updated_at = now()
        WHERE user_id = NEW.user_id AND status = 'active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_auth_revoke_sessions_on_account_change
    AFTER UPDATE OF status, credential_version ON iam_accounts
    FOR EACH ROW EXECUTE FUNCTION auth_revoke_sessions_on_account_change();

UPDATE dai_schema_metadata SET version = 11, updated_at = now() WHERE singleton = TRUE;

COMMIT;

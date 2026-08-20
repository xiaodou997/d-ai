-- Schema 11 -> 12: one-time account activation and password-reset credentials.
BEGIN;

DO $$
BEGIN
    IF (SELECT version FROM dai_schema_metadata WHERE singleton = TRUE FOR UPDATE) IS DISTINCT FROM 11 THEN
        RAISE EXCEPTION 'migration 0012 requires schema version 11';
    END IF;
END
$$;

ALTER TABLE iam_accounts
    ADD COLUMN credential_state TEXT NOT NULL DEFAULT 'active'
    CHECK (credential_state IN ('active', 'pending_activation'));

CREATE TABLE auth_activation_tokens (
    token_hash BYTEA PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES iam_accounts (user_id) ON DELETE CASCADE,
    purpose TEXT NOT NULL CHECK (purpose IN ('account_activation', 'password_reset')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_auth_activation_tokens_pending_user
    ON auth_activation_tokens (user_id) WHERE consumed_at IS NULL;
CREATE INDEX idx_auth_activation_tokens_expires
    ON auth_activation_tokens (expires_at) WHERE consumed_at IS NULL;

UPDATE dai_schema_metadata SET version = 12, updated_at = now() WHERE singleton = TRUE;

COMMIT;

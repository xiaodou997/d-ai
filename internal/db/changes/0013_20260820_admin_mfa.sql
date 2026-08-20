-- Schema 12 -> 13: encrypted TOTP enrollment for privileged accounts.
BEGIN;

DO $$
BEGIN
    IF (SELECT version FROM dai_schema_metadata WHERE singleton = TRUE FOR UPDATE) IS DISTINCT FROM 12 THEN
        RAISE EXCEPTION 'migration 0013 requires schema version 12';
    END IF;
END
$$;

ALTER TABLE iam_accounts
    ADD COLUMN mfa_secret_encrypted TEXT,
    ADD COLUMN mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN mfa_enrolled_at TIMESTAMPTZ;

UPDATE dai_schema_metadata SET version = 13, updated_at = now() WHERE singleton = TRUE;

COMMIT;

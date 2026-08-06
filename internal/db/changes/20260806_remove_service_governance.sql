-- Remove the obsolete multi-service registry and access policy model.
-- D-AI is one backend and one Portal; account type and domain permissions
-- determine access, not a second service-level allowlist.

BEGIN;

DROP TABLE IF EXISTS gov_subject_service_access;
DROP TABLE IF EXISTS gov_service_instances;
DROP TABLE IF EXISTS gov_service_sources;
DROP TABLE IF EXISTS gov_clients;
DROP TABLE IF EXISTS auth_oauth_codes;

ALTER TABLE auth_signing_keys
    DROP COLUMN IF EXISTS key_use;
CREATE INDEX IF NOT EXISTS idx_auth_signing_keys_status ON auth_signing_keys (status);

UPDATE dai_schema_metadata
SET version = 3
WHERE singleton = TRUE AND version = 2;

COMMIT;

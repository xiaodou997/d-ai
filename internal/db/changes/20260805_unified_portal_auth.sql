-- Unified Portal authentication context.
-- Apply manually to existing databases after reviewing the deployment window.

BEGIN;

ALTER TABLE auth_oauth_codes
    DROP CONSTRAINT IF EXISTS auth_oauth_codes_client_type_check;

-- Authorization codes are short-lived. Normalize any outstanding codes before
-- enforcing the one-client Portal contract.
UPDATE auth_oauth_codes
SET client_type = 'portal'
WHERE client_type <> 'portal';

ALTER TABLE auth_oauth_codes
    ADD CONSTRAINT auth_oauth_codes_client_type_check CHECK (client_type = 'portal');

UPDATE dai_schema_metadata
SET version = 2
WHERE singleton = TRUE AND version = 1;

COMMIT;

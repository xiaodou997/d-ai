-- User authentication is authorized by user type and permissions. Service
-- scopes belonged to the removed inter-service client model.

BEGIN;

ALTER TABLE auth_audit_logs DROP COLUMN IF EXISTS scopes;

UPDATE dai_schema_metadata
SET version = 6
WHERE singleton = TRUE AND version = 5;

COMMIT;

-- A single Portal authenticates users directly; a synthetic client identity adds no authorization value.

BEGIN;

DROP INDEX IF EXISTS idx_auth_audit_logs_client_time;
DROP INDEX IF EXISTS idx_auth_audit_logs_client_jti_time;
ALTER TABLE auth_audit_logs DROP COLUMN IF EXISTS client_id;

UPDATE dai_schema_metadata
SET version = 5
WHERE singleton = TRUE AND version = 4;

COMMIT;

-- Remove the last service-specific names from billing and audit records.

BEGIN;

ALTER TABLE auth_audit_logs
    DROP CONSTRAINT IF EXISTS auth_audit_logs_principal_type_check;
DELETE FROM auth_audit_logs WHERE principal_type = 'service';
ALTER TABLE auth_audit_logs
    ADD CONSTRAINT auth_audit_logs_principal_type_check CHECK (principal_type IN ('user', 'admin'));

ALTER TABLE ai_usage_logs RENAME COLUMN urm_transaction_id TO billing_event_id;
ALTER TABLE ai_async_tasks RENAME COLUMN urm_transaction_id TO billing_event_id;
ALTER TABLE ai_billing_settlement_batches RENAME COLUMN urm_event_id TO billing_event_id;
ALTER TABLE ai_sub_orders RENAME COLUMN urm_event_id TO billing_event_id;
ALTER INDEX IF EXISTS idx_ai_usage_logs_urm_transaction RENAME TO idx_ai_usage_logs_billing_event;

UPDATE dai_schema_metadata
SET version = 4
WHERE singleton = TRUE AND version = 3;

COMMIT;

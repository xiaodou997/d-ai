-- Application rollback companion for migration 0047. Restores schema 46 without
-- changing account IDs, endpoint IDs, credentials, bindings or request history.
BEGIN;
SELECT pg_advisory_xact_lock(82624001);
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM dai_schema_metadata WHERE singleton = TRUE AND version = 47) THEN
        RAISE EXCEPTION 'expected D-AI schema version 47';
    END IF;
END
$$;
ALTER TABLE ai_upstream_account_endpoints
    DROP CONSTRAINT ai_upstream_account_endpoints_account_id_key,
    ADD CONSTRAINT ai_upstream_account_endpoints_account_id_api_format_key UNIQUE (account_id, api_format);
UPDATE dai_schema_metadata SET version = 46, updated_at = now() WHERE singleton = TRUE AND version = 47;
COMMIT;

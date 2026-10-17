-- from_version: 46
-- to_version: 47
-- created_at: 2026-10-17
-- description: restrict each direct upstream account to one endpoint without deleting existing data

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 46
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 46';
    END IF;
END
$$;

-- Block concurrent account/endpoint writes until the preflight and constraint
-- change commit together. Legacy connections must be resolved explicitly.
LOCK TABLE ai_upstream_accounts, ai_upstream_account_endpoints IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT a.id
        FROM ai_upstream_accounts a
        LEFT JOIN ai_upstream_account_endpoints e ON e.account_id = a.id
        GROUP BY a.id
        HAVING count(e.id) <> 1
    ) THEN
        RAISE EXCEPTION 'every upstream account must have exactly one endpoint before migration 0047; no accounts or endpoints were changed';
    END IF;
END
$$;

ALTER TABLE ai_upstream_account_endpoints
    DROP CONSTRAINT ai_upstream_account_endpoints_account_id_api_format_key,
    ADD CONSTRAINT ai_upstream_account_endpoints_account_id_key UNIQUE (account_id);

UPDATE dai_schema_metadata
SET version = 47, updated_at = now()
WHERE singleton = TRUE AND version = 46;

COMMIT;

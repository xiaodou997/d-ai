-- from_version: 39
-- to_version: 40
-- created_at: 2026-09-11
-- description: expose direct upstream account descriptions in the tenant resource view

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 39
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 39';
    END IF;
END
$$;

-- PostgreSQL permits adding columns only at the end when replacing a view.
-- Keep the existing column order stable for consumers and append description.
CREATE OR REPLACE VIEW ai_upstream_resources AS
  SELECT
    id,
    'direct_upstream'::TEXT AS resource_kind,
    name,
    price_book_id,
    tenant_multiplier,
    status,
    created_at,
    updated_at,
    tenant_display_name,
    tenant_access_mode,
    description
  FROM ai_upstream_accounts
  UNION ALL
  SELECT
    id,
    'oauth_pool'::TEXT AS resource_kind,
    name,
    price_book_id,
    tenant_multiplier,
    status,
    created_at,
    updated_at,
    tenant_display_name,
    tenant_access_mode,
    NULL::TEXT AS description
  FROM ai_credential_pools;

UPDATE dai_schema_metadata
SET version = 40,
    updated_at = now()
WHERE singleton = TRUE AND version = 39;

COMMIT;

-- from_version: 31
-- to_version: 32
-- created_at: 2026-09-04
-- description: separate direct-account API endpoints from account-level models

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 31
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 31';
    END IF;
END
$$;

CREATE TABLE ai_upstream_account_endpoints (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID        NOT NULL REFERENCES ai_upstream_accounts(id) ON DELETE CASCADE,
	api_format      TEXT        NOT NULL,
    base_url        TEXT        NOT NULL CHECK (btrim(base_url) <> ''),
    path_override   TEXT        NOT NULL DEFAULT '',
    auth_scheme     TEXT        NOT NULL DEFAULT 'format_default' CHECK (auth_scheme IN (
      'format_default', 'bearer', 'anthropic_api_key', 'gemini_api_key', 'custom_header'
    )),
    auth_header     TEXT        NOT NULL DEFAULT '',
	extra_headers   JSONB       NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(extra_headers) = 'object'),
	status          TEXT        NOT NULL DEFAULT 'active',
    health_status   TEXT        NOT NULL DEFAULT 'unknown' CHECK (health_status IN ('unknown', 'healthy', 'unhealthy')),
    last_error      TEXT        NOT NULL DEFAULT '',
    last_checked_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (account_id, api_format),
	CHECK (auth_scheme <> 'custom_header' OR btrim(auth_header) <> '')
);

CREATE INDEX idx_ai_upstream_account_endpoints_account
    ON ai_upstream_account_endpoints (account_id, status, api_format);

-- Existing binding formats preserve every exact format already in use.
INSERT INTO ai_upstream_account_endpoints (
    account_id, api_format, base_url, extra_headers, status
)
SELECT DISTINCT
    a.id, um.api_format, a.base_url,
    CASE WHEN jsonb_typeof(a.extra_headers) = 'object' THEN a.extra_headers ELSE '{}'::jsonb END,
    'active'
FROM ai_upstream_accounts a
JOIN ai_upstream_models um
  ON um.upstream_kind = 'direct_upstream' AND um.upstream_id = a.id;

-- Accounts without model bindings still keep one endpoint. Their former coarse
-- provider family is mapped to the product's previous default format so every
-- migrated account satisfies the new one-or-more endpoint invariant.
INSERT INTO ai_upstream_account_endpoints (
    account_id, api_format, base_url, extra_headers, status
)
SELECT
    a.id,
    CASE a.default_protocol
      WHEN 'anthropic' THEN 'anthropic_messages'
      WHEN 'gemini' THEN 'gemini_generate'
      ELSE 'openai_responses'
    END,
    a.base_url,
    CASE WHEN jsonb_typeof(a.extra_headers) = 'object' THEN a.extra_headers ELSE '{}'::jsonb END,
    'active'
FROM ai_upstream_accounts a
WHERE NOT EXISTS (
    SELECT 1 FROM ai_upstream_account_endpoints ae WHERE ae.account_id = a.id
);

-- Split the historical account attribution out of endpoint_id, then resolve
-- the concrete endpoint whenever the recorded provider format makes it known.
ALTER TABLE ai_usage_logs ADD COLUMN upstream_account_id UUID;
UPDATE ai_usage_logs
SET upstream_account_id = endpoint_id
WHERE endpoint_id IS NOT NULL;
UPDATE ai_usage_logs SET endpoint_id = NULL WHERE upstream_account_id IS NOT NULL;
UPDATE ai_usage_logs l
SET endpoint_id = ae.id
FROM ai_upstream_account_endpoints ae
WHERE ae.account_id = l.upstream_account_id
  AND ae.api_format = l.provider_format;
CREATE INDEX idx_ai_usage_logs_upstream_account
    ON ai_usage_logs (upstream_account_id, created_at DESC)
    WHERE upstream_account_id IS NOT NULL;
CREATE INDEX idx_ai_usage_logs_endpoint
    ON ai_usage_logs (endpoint_id, created_at DESC)
    WHERE endpoint_id IS NOT NULL;

ALTER TABLE ai_upstream_models DROP COLUMN api_format;
DROP INDEX IF EXISTS idx_ai_upstream_models_model;
CREATE INDEX idx_ai_upstream_models_model
    ON ai_upstream_models (model_code, capability_type, status);

ALTER TABLE ai_upstream_accounts
    DROP COLUMN base_url,
    DROP COLUMN extra_headers,
    DROP COLUMN default_protocol;

UPDATE dai_schema_metadata
SET version = 32,
    updated_at = now()
WHERE singleton = TRUE AND version = 31;

COMMIT;

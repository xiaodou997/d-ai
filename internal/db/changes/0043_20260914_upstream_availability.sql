-- from_version: 42
-- to_version: 43
-- created_at: 2026-09-14
-- description: timed upstream recovery and per-provider-attempt observability
-- Stop new requests and drain the gateway before applying; retain request history.
BEGIN;
SELECT pg_advisory_xact_lock(82624001);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM dai_schema_metadata WHERE singleton AND version=42) THEN RAISE EXCEPTION 'expected schema version 42'; END IF;
 IF EXISTS(SELECT 1 FROM ai_request_keys WHERE sealed_at IS NULL AND lease_until>now()) THEN RAISE EXCEPTION 'drain active requests before availability cutover'; END IF;
END $$;
ALTER TABLE ai_request_attempts
 ADD COLUMN upstream_kind text,
 ADD COLUMN upstream_resource_id text,
 ADD COLUMN endpoint_id text,
 ADD COLUMN credential_id text,
 ADD COLUMN model_code text,
 ADD COLUMN upstream_model text,
 ADD COLUMN operation text,
 ADD COLUMN stream boolean,
 ADD COLUMN completed_at timestamptz,
 ADD COLUMN outcome text,
 ADD COLUMN cooldown_entered boolean NOT NULL DEFAULT false,
 ADD COLUMN latency_ms integer;
CREATE INDEX ai_request_attempts_upstream_window ON ai_request_attempts(upstream_kind,upstream_resource_id,completed_at DESC);
CREATE INDEX ai_request_attempts_completed_window ON ai_request_attempts(completed_at DESC) INCLUDE(upstream_kind,upstream_resource_id,outcome,cooldown_entered) WHERE completed_at IS NOT NULL;
CREATE INDEX ai_request_attempts_credential_window ON ai_request_attempts(credential_id,completed_at DESC) WHERE credential_id<>'';
CREATE INDEX ai_request_attempts_endpoint_model_window ON ai_request_attempts(endpoint_id,upstream_model,operation,completed_at DESC) WHERE endpoint_id<>'';
CREATE TABLE ai_upstream_runtime_metadata(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), started_at timestamptz NOT NULL DEFAULT now());
INSERT INTO ai_upstream_runtime_metadata(singleton) VALUES(true);
-- Preserve the rejection evidence for administrators while restoring eligibility.
UPDATE ai_upstream_accounts SET status='active' WHERE status='invalid';
UPDATE ai_provider_oauth_credentials SET status='active',cooldown_until=now()+interval '30 minutes' WHERE status='invalid';
UPDATE dai_schema_metadata SET version=43,updated_at=now() WHERE singleton AND version=42;
COMMIT;

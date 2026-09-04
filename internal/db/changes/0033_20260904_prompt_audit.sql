-- from_version: 32
-- to_version: 33
-- created_at: 2026-09-04
-- description: add privacy-safe Qwen3Guard prompt audit events

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 32
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 32';
    END IF;
END
$$;

ALTER TABLE ai_content_moderation_logs
    ADD COLUMN input_hash TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_ai_content_moderation_logs_input_hash
    ON ai_content_moderation_logs (input_hash)
    WHERE input_hash <> '';

CREATE TABLE ai_prompt_audit_events (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id          TEXT,
    tenant_id           TEXT,
    user_id             TEXT,
    api_key_id          UUID,
    model_code          TEXT,
    capability_type     TEXT,
    protocol            TEXT,
    prompt_hash         TEXT        NOT NULL,
    redacted_preview    TEXT        NOT NULL DEFAULT '',
    prompt_length       INTEGER     NOT NULL DEFAULT 0 CHECK (prompt_length >= 0),
    message_count       INTEGER     NOT NULL DEFAULT 0 CHECK (message_count >= 0),
    decision            TEXT        NOT NULL CHECK (decision IN ('pass', 'flag', 'critical', 'error')),
    risk_level          TEXT        NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'critical', 'unknown')),
    action              TEXT        NOT NULL CHECK (action IN ('Allow', 'Warn', 'Block', 'Error')),
    safety              TEXT,
    categories          JSONB       NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(categories) = 'array'),
    matched_scanners    JSONB       NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(matched_scanners) = 'array'),
    scanner_scores      JSONB       NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(scanner_scores) = 'object'),
    unknown_categories  JSONB       NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(unknown_categories) = 'array'),
    scanner_backend     TEXT        NOT NULL DEFAULT 'qwen3guard-openai',
    scanner_version     TEXT,
    guard_endpoint_id   TEXT,
    config_revision     BIGINT      NOT NULL DEFAULT 1 CHECK (config_revision >= 1),
    chunk_total         INTEGER     NOT NULL DEFAULT 0 CHECK (chunk_total >= 0),
    latency_ms          INTEGER     NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    error_code          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_prompt_audit_events_created
    ON ai_prompt_audit_events (created_at DESC, id DESC);
CREATE INDEX idx_ai_prompt_audit_events_tenant
    ON ai_prompt_audit_events (tenant_id, created_at DESC);
CREATE INDEX idx_ai_prompt_audit_events_user
    ON ai_prompt_audit_events (user_id, created_at DESC);
CREATE INDEX idx_ai_prompt_audit_events_decision
    ON ai_prompt_audit_events (decision, created_at DESC);
CREATE INDEX idx_ai_prompt_audit_events_hash
    ON ai_prompt_audit_events (prompt_hash);

INSERT INTO ai_settings (key, value) VALUES ('prompt_audit_config', '{
  "enabled": false,
  "mode": "off",
  "latest_turn_only": false,
  "store_pass_events": false,
  "worker_count": 4,
  "queue_capacity": 4096,
  "scanners": ["violent", "non_violent_illegal_acts", "sexual_content_or_sexual_acts", "pii", "suicide_and_self_harm", "unethical_acts", "politically_sensitive_topics", "copyright_violation", "jailbreak"],
  "tenant_ids": [],
  "endpoints": [],
  "config_revision": 1
}'::jsonb)
ON CONFLICT (key) DO NOTHING;

UPDATE dai_schema_metadata
SET version = 33,
    updated_at = now()
WHERE singleton = TRUE AND version = 32;

COMMIT;


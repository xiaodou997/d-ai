-- from_version: 9
-- to_version: 10
-- created_at: 2026-08-19
-- description: add data cleanup policy and run history

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 9
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 9';
    END IF;
END
$$;

CREATE TABLE sys_data_cleanup_runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trigger      TEXT NOT NULL CHECK (trigger IN ('automatic', 'manual')),
    status       TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    requested_by TEXT,
    targets      JSONB NOT NULL DEFAULT '[]'::jsonb,
    summary      JSONB NOT NULL DEFAULT '{}'::jsonb,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_sys_data_cleanup_active
    ON sys_data_cleanup_runs ((1))
    WHERE status IN ('queued', 'running');
CREATE INDEX idx_sys_data_cleanup_runs_created
    ON sys_data_cleanup_runs (created_at DESC);

CREATE INDEX idx_ai_audit_blobs_created_at
    ON ai_audit_blobs (created_at);

INSERT INTO sys_settings (key, value)
VALUES (
    'data_cleanup',
    '{
        "enabled": true,
        "requestBodyDays": 30,
        "requestPayloadDays": 180,
        "notificationDays": 90,
        "moderationDays": 90,
        "riskEventDays": 365,
        "adminAuditDays": 365,
        "auditBlobDays": 180,
        "usageRollupDays": 730,
        "batchSize": 1000
    }'::jsonb
)
ON CONFLICT (key) DO NOTHING;

UPDATE dai_schema_metadata
SET version = 10,
    updated_at = now()
WHERE singleton = TRUE AND version = 9;

COMMIT;

-- from_version: 8
-- to_version: 9
-- created_at: 2026-08-18
-- description: add module runtime tables for notifications and proxy egress

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 8
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 8';
    END IF;
END
$$;

CREATE TABLE ai_proxy_nodes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    proxy_type          TEXT NOT NULL CHECK (proxy_type IN ('http', 'socks5')),
    endpoint            TEXT NOT NULL CHECK (btrim(endpoint) <> ''),
    username            TEXT NOT NULL DEFAULT '',
    proxy_password_enc  TEXT NOT NULL DEFAULT '',
    weight              INTEGER NOT NULL DEFAULT 1 CHECK (weight > 0 AND weight <= 1000),
    status              TEXT NOT NULL DEFAULT 'disabled',
    health_status       TEXT NOT NULL DEFAULT 'unknown' CHECK (health_status IN ('unknown', 'healthy', 'unhealthy')),
    last_checked_at     TIMESTAMPTZ,
    last_error          TEXT,
    created_by          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_proxy_nodes_status ON ai_proxy_nodes (status, health_status, updated_at DESC);

CREATE TABLE sys_notification_deliveries (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_key           TEXT NOT NULL,
    channel             TEXT NOT NULL CHECK (channel IN ('in_app', 'webhook')),
    recipient_user_id   TEXT,
    recipient_user_type INTEGER,
    tenant_id           TEXT,
    title               TEXT NOT NULL,
    body                TEXT NOT NULL,
    payload             JSONB NOT NULL DEFAULT '{}'::jsonb,
    status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts            INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error          TEXT,
    idempotency_key     TEXT UNIQUE,
    sent_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sys_notification_user ON sys_notification_deliveries (recipient_user_id, created_at DESC);
CREATE INDEX idx_sys_notification_status ON sys_notification_deliveries (status, created_at ASC);

UPDATE dai_schema_metadata
SET version = 9, updated_at = now()
WHERE singleton = TRUE AND version = 8;

COMMIT;

-- from_version: 13
-- to_version: 14
-- created_at: 2026-08-20
-- description: durable PostgreSQL audit inbox with idempotent payload keys

BEGIN;

DO $$
BEGIN
    IF (SELECT version FROM dai_schema_metadata WHERE singleton = TRUE FOR UPDATE) IS DISTINCT FROM 13 THEN
        RAISE EXCEPTION 'migration 0014 requires schema version 13';
    END IF;
END
$$;

-- Older in-memory workers could write a duplicate payload after a retry. Keep
-- the newest copy before making request_id the idempotency key.
DELETE FROM ai_request_payloads older
USING ai_request_payloads newer
WHERE older.request_id = newer.request_id
  AND (older.created_at < newer.created_at
       OR (older.created_at = newer.created_at AND older.id < newer.id));

DROP INDEX idx_arp_request_id;
CREATE UNIQUE INDEX idx_arp_request_id ON ai_request_payloads (request_id);

CREATE TABLE ai_audit_inbox (
    id           BIGSERIAL PRIMARY KEY,
    request_id   TEXT NOT NULL UNIQUE,
    payload      JSONB NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending'
      CHECK (status IN ('pending', 'processing', 'dead')),
    attempts     INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at    TIMESTAMPTZ,
    locked_by    TEXT,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    dead_at      TIMESTAMPTZ
);

CREATE INDEX idx_ai_audit_inbox_ready
    ON ai_audit_inbox (available_at, created_at) WHERE status = 'pending';
CREATE INDEX idx_ai_audit_inbox_status
    ON ai_audit_inbox (status, created_at);

UPDATE dai_schema_metadata SET version = 14, updated_at = now() WHERE singleton = TRUE;

COMMIT;

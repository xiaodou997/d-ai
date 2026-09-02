-- from_version: 30
-- to_version: 31
-- created_at: 2026-09-02
-- description: allow announcement deletion audit events

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 30
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 30';
    END IF;
END
$$;

ALTER TABLE ann_audit_events
    DROP CONSTRAINT ann_audit_events_event_type_check,
    ADD CONSTRAINT ann_audit_events_event_type_check
        CHECK (event_type IN ('created', 'updated', 'published', 'archived', 'draft_deleted', 'deleted'));

UPDATE dai_schema_metadata
SET version = 31,
    updated_at = now()
WHERE singleton = TRUE AND version = 30;

COMMIT;

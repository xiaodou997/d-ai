-- from_version: 27
-- to_version: 28
-- created_at: 2026-09-01
-- description: allow destructive tenant deletion lifecycle states

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 27
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 27';
    END IF;
END
$$;

ALTER TABLE iam_tenants
    DROP CONSTRAINT IF EXISTS iam_tenants_status_check;

ALTER TABLE iam_tenants
    ADD CONSTRAINT iam_tenants_status_check
    CHECK (status IN ('active', 'disabled', 'suspended', 'deleting', 'purging'));

CREATE TABLE tenant_deletion_jobs (
    job_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'cancelled', 'completed', 'failed')),
    requested_by TEXT,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    execute_after TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    last_error TEXT,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0)
);
CREATE UNIQUE INDEX uq_tenant_deletion_jobs_active ON tenant_deletion_jobs (tenant_id)
    WHERE status IN ('pending', 'running');
CREATE INDEX idx_tenant_deletion_jobs_ready ON tenant_deletion_jobs (execute_after)
    WHERE status = 'pending';

UPDATE dai_schema_metadata
SET version = 28,
    updated_at = now()
WHERE singleton = TRUE AND version = 27;

COMMIT;

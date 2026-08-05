-- +goose Up

-- URM controls entry into portal business services. Business-domain
-- permissions remain owned by each service.
CREATE TABLE gov_subject_service_access (
    subject_type TEXT NOT NULL CHECK (subject_type IN ('admin', 'tenant')),
    subject_id TEXT NOT NULL,
    access_mode TEXT NOT NULL CHECK (access_mode IN ('all', 'selected')),
    service_ids TEXT[] NOT NULL DEFAULT '{}',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (subject_type, subject_id),
    CHECK (access_mode <> 'all' OR cardinality(service_ids) = 0)
);

CREATE INDEX idx_gov_subject_service_access_updated_at
    ON gov_subject_service_access (updated_at);

-- +goose Down
DROP TABLE IF EXISTS gov_subject_service_access;

-- +goose Up

CREATE TABLE iam_user_legal_acceptances (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    document_key TEXT NOT NULL CHECK (document_key IN ('terms', 'privacy')),
    document_version TEXT NOT NULL CHECK (length(trim(document_version)) > 0),
    source TEXT NOT NULL DEFAULT 'public_registration',
    accepted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, document_key, document_version)
);

CREATE INDEX idx_iam_user_legal_acceptances_subject
    ON iam_user_legal_acceptances (user_id, accepted_at DESC);

CREATE INDEX idx_iam_user_legal_acceptances_tenant
    ON iam_user_legal_acceptances (tenant_id, accepted_at DESC);

-- +goose Down

DROP TABLE IF EXISTS iam_user_legal_acceptances;

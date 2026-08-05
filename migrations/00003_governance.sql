-- +goose Up

CREATE TABLE gov_clients (
    id BIGSERIAL PRIMARY KEY,
    client_id TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    portal_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_gov_clients_client_id ON gov_clients (client_id);
CREATE INDEX idx_gov_clients_status ON gov_clients (status);

CREATE TABLE gov_service_sources (
    id BIGSERIAL PRIMARY KEY,
    service_id TEXT NOT NULL,
    source_cidr CIDR NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (service_id, source_cidr)
);

CREATE INDEX idx_gov_service_sources_cidr ON gov_service_sources USING gist (source_cidr inet_ops);

CREATE TABLE gov_service_instances (
    service_id TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    observed_ip INET NOT NULL,
    source_cidr CIDR NOT NULL,
    service_name TEXT,
    version TEXT,
    environment TEXT,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (service_id, instance_id)
);

CREATE INDEX idx_gov_service_instances_last_seen ON gov_service_instances (last_seen DESC);

-- +goose Down
DROP TABLE IF EXISTS gov_service_instances;
DROP TABLE IF EXISTS gov_service_sources;
DROP TABLE IF EXISTS gov_clients;

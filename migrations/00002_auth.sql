-- +goose Up

CREATE TABLE auth_signing_keys (
    id BIGSERIAL PRIMARY KEY,
    kid TEXT NOT NULL UNIQUE,
    private_key TEXT NOT NULL,
    public_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'grace', 'retired')),
    key_use TEXT NOT NULL DEFAULT 'shared' CHECK (key_use IN ('shared', 'user_access', 'user_refresh', 'service_access')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    grace_until TIMESTAMPTZ,
    retired_at TIMESTAMPTZ
);

CREATE INDEX idx_auth_signing_keys_status_use ON auth_signing_keys (status, key_use);

CREATE TABLE auth_oauth_codes (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    username TEXT NOT NULL,
    tenant_id TEXT,
    user_type INTEGER NOT NULL CHECK (user_type IN (1, 2, 3, 4)),
    client_type TEXT NOT NULL CHECK (client_type IN ('admin', 'tenant', 'customer')),
    redirect_uri TEXT NOT NULL,
    code_challenge TEXT NOT NULL DEFAULT '',
    code_challenge_method TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_oauth_codes_expires ON auth_oauth_codes (expires_at);

CREATE TABLE auth_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    principal_type TEXT NOT NULL CHECK (principal_type IN ('user', 'service', 'admin')),
    client_id TEXT,
    user_id TEXT,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    jti TEXT,
    request_id TEXT,
    decision TEXT NOT NULL CHECK (decision IN ('success', 'deny', 'error')),
    reason_code TEXT,
    reason_message TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_audit_logs_client_time ON auth_audit_logs (client_id, created_at DESC);
CREATE INDEX idx_auth_audit_logs_event_time ON auth_audit_logs (event_type, created_at DESC);
CREATE INDEX idx_auth_audit_logs_client_jti_time ON auth_audit_logs (client_id, jti, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS auth_audit_logs;
DROP TABLE IF EXISTS auth_oauth_codes;
DROP TABLE IF EXISTS auth_signing_keys;

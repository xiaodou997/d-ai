-- +goose Up

CREATE TABLE iam_admins (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email TEXT,
    user_type INTEGER NOT NULL CHECK (user_type IN (1, 2)),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_iam_admins_status ON iam_admins (status);

CREATE TABLE iam_tenants (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL UNIQUE,
    tenant_name TEXT NOT NULL,
    contact_person TEXT,
    contact_email TEXT,
    frozen_credits BIGINT NOT NULL DEFAULT 0 CHECK (frozen_credits >= 0),
    overdraft_limit BIGINT NOT NULL DEFAULT 0 CHECK (overdraft_limit >= 0),
    current_overdraft BIGINT NOT NULL DEFAULT 0 CHECK (current_overdraft >= 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_iam_tenants_status ON iam_tenants (status);
CREATE INDEX idx_iam_tenants_has_overdraft ON iam_tenants (tenant_id) WHERE current_overdraft > 0;

CREATE TABLE iam_tenant_users (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'inherited_disabled')),
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, username)
);

CREATE INDEX idx_iam_tenant_users_tenant ON iam_tenant_users (tenant_id);
CREATE INDEX idx_iam_tenant_users_status ON iam_tenant_users (status);

CREATE TABLE iam_users (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    nickname TEXT,
    avatar TEXT,
    frozen_credits BIGINT NOT NULL DEFAULT 0 CHECK (frozen_credits >= 0),
    overdraft_limit BIGINT NOT NULL DEFAULT 0 CHECK (overdraft_limit >= 0),
    current_overdraft BIGINT NOT NULL DEFAULT 0 CHECK (current_overdraft >= 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'locked', 'inherited_disabled')),
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, username)
);

CREATE INDEX idx_iam_users_tenant ON iam_users (tenant_id);
CREATE INDEX idx_iam_users_status ON iam_users (status);
CREATE INDEX idx_iam_users_has_overdraft ON iam_users (user_id) WHERE current_overdraft > 0;

CREATE TABLE iam_invitation_codes (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    created_by TEXT NOT NULL,
    description TEXT,
    max_uses INTEGER NOT NULL DEFAULT 0 CHECK (max_uses >= 0),
    used_count INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_iam_invitation_codes_tenant ON iam_invitation_codes (tenant_id);
CREATE INDEX idx_iam_invitation_codes_status_expires ON iam_invitation_codes (status, expires_at);

-- +goose Down
DROP TABLE IF EXISTS iam_invitation_codes;
DROP TABLE IF EXISTS iam_users;
DROP TABLE IF EXISTS iam_tenant_users;
DROP TABLE IF EXISTS iam_tenants;
DROP TABLE IF EXISTS iam_admins;

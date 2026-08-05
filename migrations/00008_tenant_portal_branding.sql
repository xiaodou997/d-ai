-- +goose Up

CREATE TABLE iam_tenant_portal_branding (
    tenant_id TEXT PRIMARY KEY REFERENCES iam_tenants(tenant_id) ON DELETE CASCADE,
    customer_site_name TEXT NOT NULL DEFAULT '',
    favicon_png BYTEA,
    favicon_updated_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (char_length(customer_site_name) <= 80)
);

-- +goose Down

DROP TABLE IF EXISTS iam_tenant_portal_branding;

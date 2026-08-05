-- +goose Up

CREATE UNIQUE INDEX ux_iam_tenants_tenant_name ON iam_tenants (tenant_name);

-- +goose Down

DROP INDEX IF EXISTS ux_iam_tenants_tenant_name;

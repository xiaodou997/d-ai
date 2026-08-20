-- D-AI local-development seed
-- Never apply this file in production.

INSERT INTO iam_tenants (tenant_id, tenant_name, contact_person, contact_email, status)
VALUES ('DAI_DEV_TENANT', 'D-AI Development Tenant', 'D-AI Developer', 'dev-tenant@localhost', 'active')
ON CONFLICT (tenant_id) DO UPDATE SET
    tenant_name = EXCLUDED.tenant_name,
    contact_person = EXCLUDED.contact_person,
    contact_email = EXCLUDED.contact_email,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO iam_accounts (user_id, username, password_hash, credential_state, email, user_type, status)
VALUES ('DAI_ADMIN', 'dai_admin', '$2a$10$VI.y0TjcNQQX/5X/ukr7xOMmmRPfAEnrFs9fnJhkEajX6JPl43JXS', 'active', NULL, 1, 'active')
ON CONFLICT (user_id) DO UPDATE SET
    username = EXCLUDED.username,
    password_hash = EXCLUDED.password_hash,
    credential_state = EXCLUDED.credential_state,
    user_type = EXCLUDED.user_type,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO iam_accounts (user_id, username, password_hash, credential_state, email, user_type, status)
VALUES ('DAI_PLATFORM_ADMIN', 'dai_platform_admin', '$2a$10$VI.y0TjcNQQX/5X/ukr7xOMmmRPfAEnrFs9fnJhkEajX6JPl43JXS', 'active', 'dev-platform-admin@localhost', 2, 'active')
ON CONFLICT (user_id) DO UPDATE SET
    username = EXCLUDED.username,
    password_hash = EXCLUDED.password_hash,
    credential_state = EXCLUDED.credential_state,
    email = EXCLUDED.email,
    user_type = EXCLUDED.user_type,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, credential_state, email, user_type, status)
VALUES ('DAI_TENANT_USER', 'DAI_DEV_TENANT', 'dai_tenant', '$2a$10$VI.y0TjcNQQX/5X/ukr7xOMmmRPfAEnrFs9fnJhkEajX6JPl43JXS', 'active', 'dev-tenant-user@localhost', 3, 'active')
ON CONFLICT (user_id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    username = EXCLUDED.username,
    password_hash = EXCLUDED.password_hash,
    credential_state = EXCLUDED.credential_state,
    email = EXCLUDED.email,
    user_type = EXCLUDED.user_type,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, credential_state, email, nickname, user_type, status)
VALUES ('DAI_END_USER', 'DAI_DEV_TENANT', 'u_dai_user', '$2a$10$VI.y0TjcNQQX/5X/ukr7xOMmmRPfAEnrFs9fnJhkEajX6JPl43JXS', 'active', 'dev-user@localhost', 'D-AI Development User', 4, 'active')
ON CONFLICT (user_id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    username = EXCLUDED.username,
    password_hash = EXCLUDED.password_hash,
    credential_state = EXCLUDED.credential_state,
    email = EXCLUDED.email,
    nickname = EXCLUDED.nickname,
    user_type = EXCLUDED.user_type,
    status = EXCLUDED.status,
    updated_at = now();

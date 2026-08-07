-- Merge every login identity into one globally unique account namespace.
-- End-user names receive one u_ prefix. A tenant user colliding with an
-- administrator receives t_; administrator names remain stable.

BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata WHERE singleton = TRUE AND version = 7
    ) THEN
        RAISE EXCEPTION 'cannot unify accounts: expected schema version 7';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM (
            SELECT username FROM iam_admins
            UNION ALL
            SELECT username FROM iam_tenant_users
            UNION ALL
            SELECT username FROM iam_users
        ) legacy_accounts
        WHERE btrim(username) = ''
    ) THEN
        RAISE EXCEPTION 'cannot unify accounts: empty legacy username';
    END IF;

    IF EXISTS (
        SELECT user_id
        FROM (
            SELECT user_id FROM iam_admins
            UNION ALL
            SELECT user_id FROM iam_tenant_users
            UNION ALL
            SELECT user_id FROM iam_users
        ) ids
        GROUP BY user_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot unify accounts: duplicate user_id across legacy account tables';
    END IF;

    IF EXISTS (
        SELECT normalized_username
        FROM (
            SELECT lower(btrim(username)) AS normalized_username FROM iam_admins
            UNION ALL
            SELECT lower(
                CASE
                    WHEN EXISTS (
                        SELECT 1 FROM iam_admins a
                        WHERE lower(btrim(a.username)) = lower(btrim(t.username))
                    ) THEN 't_' || btrim(t.username)
                    ELSE btrim(t.username)
                END
            )
            FROM iam_tenant_users t
            UNION ALL
            SELECT lower(
                CASE
                    WHEN btrim(username) LIKE 'u\_%' ESCAPE '\' THEN btrim(username)
                    ELSE 'u_' || btrim(username)
                END
            )
            FROM iam_users
        ) usernames
        GROUP BY normalized_username
        HAVING normalized_username = '' OR count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot unify accounts: duplicate or empty normalized username';
    END IF;
END
$$;

CREATE TABLE iam_accounts (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT REFERENCES iam_tenants (tenant_id),
    username TEXT NOT NULL CHECK (username <> '' AND username = btrim(username)),
    password_hash TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    user_type INTEGER NOT NULL CHECK (user_type IN (1, 2, 3, 4)),
    internal_note TEXT NOT NULL DEFAULT '',
    nickname TEXT,
    avatar TEXT,
    frozen_credits BIGINT NOT NULL DEFAULT 0 CHECK (frozen_credits >= 0),
    overdraft_limit BIGINT NOT NULL DEFAULT 0 CHECK (overdraft_limit >= 0),
    current_overdraft BIGINT NOT NULL DEFAULT 0 CHECK (current_overdraft >= 0),
    status TEXT NOT NULL DEFAULT 'active',
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT iam_accounts_tenant_pairing_check CHECK (
        (user_type IN (1, 2) AND tenant_id IS NULL)
        OR (user_type IN (3, 4) AND tenant_id IS NOT NULL)
    ),
    CONSTRAINT iam_accounts_username_namespace_check CHECK (
        (user_type = 4 AND char_length(username) > 2 AND username LIKE 'u\_%' ESCAPE '\')
        OR (user_type <> 4 AND username NOT LIKE 'u\_%' ESCAPE '\')
    ),
    CONSTRAINT iam_accounts_status_check CHECK (
        (user_type IN (1, 2) AND status IN ('active', 'disabled'))
        OR (user_type = 3 AND status IN ('active', 'disabled', 'inherited_disabled'))
        OR (user_type = 4 AND status IN ('active', 'disabled', 'locked', 'inherited_disabled', 'deleted'))
    )
);

INSERT INTO iam_accounts (
    user_id, tenant_id, username, password_hash, email, phone, user_type,
    internal_note, nickname, avatar, frozen_credits, overdraft_limit,
    current_overdraft, status, last_login_at, created_at, updated_at
)
SELECT user_id, NULL, btrim(username), password_hash, email, NULL, user_type,
       '', NULL, NULL, 0, 0, 0, status, last_login_at, created_at, updated_at
FROM iam_admins
UNION ALL
SELECT user_id, tenant_id,
       CASE
           WHEN EXISTS (
               SELECT 1 FROM iam_admins a
               WHERE lower(btrim(a.username)) = lower(btrim(t.username))
           ) THEN 't_' || btrim(t.username)
           ELSE btrim(t.username)
       END,
       password_hash, email, phone, 3,
       '', NULL, NULL, 0, 0, 0, status, last_login_at, created_at, updated_at
FROM iam_tenant_users t
UNION ALL
SELECT user_id, tenant_id,
       CASE
           WHEN btrim(username) LIKE 'u\_%' ESCAPE '\' THEN btrim(username)
           ELSE 'u_' || btrim(username)
       END,
       password_hash, email, phone, 4, internal_note, nickname, avatar,
       frozen_credits, overdraft_limit, current_overdraft, status,
       last_login_at, created_at, updated_at
FROM iam_users;

CREATE UNIQUE INDEX ux_iam_accounts_username_normalized ON iam_accounts (lower(username));
CREATE INDEX idx_iam_accounts_tenant_type ON iam_accounts (tenant_id, user_type);
CREATE INDEX idx_iam_accounts_type_status ON iam_accounts (user_type, status);
CREATE INDEX idx_iam_accounts_has_overdraft ON iam_accounts (user_id)
    WHERE user_type = 4 AND current_overdraft > 0;

DROP TABLE iam_admins;
DROP TABLE iam_tenant_users;
DROP TABLE iam_users;

UPDATE dai_schema_metadata
SET version = 8
WHERE singleton = TRUE AND version = 7;

COMMIT;

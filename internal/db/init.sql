-- D-AI canonical database schema
--
-- Apply only to an empty PostgreSQL schema. The application never executes this
-- file and never upgrades the schema automatically. Maintain this file as the full
-- desired state, and place post-release manual changes under internal/db/changes/.

BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class AS c
        JOIN pg_namespace AS n ON n.oid = c.relnamespace
        WHERE n.nspname = current_schema()
          AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
    ) THEN
        RAISE EXCEPTION 'D-AI canonical schema initialization requires an empty schema';
    END IF;
END
$$;

-- Identity and tenant management
CREATE TABLE iam_tenants (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL UNIQUE,
    tenant_name TEXT NOT NULL,
    contact_person TEXT,
    contact_email TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_iam_tenants_status ON iam_tenants (status);

CREATE TABLE iam_accounts (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT REFERENCES iam_tenants (tenant_id),
    username TEXT NOT NULL CHECK (username <> '' AND username = btrim(username)),
    password_hash TEXT NOT NULL,
    credential_version BIGINT NOT NULL DEFAULT 1 CHECK (credential_version > 0),
    credential_state TEXT NOT NULL DEFAULT 'active' CHECK (credential_state IN ('active', 'pending_activation')),
    mfa_secret_encrypted TEXT,
    mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_enrolled_at TIMESTAMPTZ,
    email TEXT,
    phone TEXT,
    user_type INTEGER NOT NULL CHECK (user_type IN (1, 2, 3, 4)),
    internal_note TEXT NOT NULL DEFAULT '',
    nickname TEXT,
    avatar TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT iam_accounts_tenant_pairing_check CHECK (
        (user_type IN (1, 2) AND tenant_id IS NULL)
        OR (user_type IN (3, 4) AND tenant_id IS NOT NULL)
    ),
    CONSTRAINT iam_accounts_status_check CHECK (
        (user_type IN (1, 2) AND status IN ('active', 'disabled'))
        OR (user_type = 3 AND status IN ('active', 'disabled', 'inherited_disabled'))
        OR (user_type = 4 AND status IN ('active', 'disabled', 'locked', 'inherited_disabled', 'deleted'))
    )
);

CREATE UNIQUE INDEX ux_iam_accounts_username_normalized ON iam_accounts (lower(username));
CREATE UNIQUE INDEX ux_iam_accounts_email_normalized ON iam_accounts (lower(email))
    WHERE email IS NOT NULL;
CREATE INDEX idx_iam_accounts_tenant_type ON iam_accounts (tenant_id, user_type);
CREATE INDEX idx_iam_accounts_type_status ON iam_accounts (user_type, status);

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

CREATE UNIQUE INDEX ux_iam_tenants_tenant_name ON iam_tenants (tenant_name);

-- Authentication
CREATE TABLE auth_signing_keys (
    id BIGSERIAL PRIMARY KEY,
    kid TEXT NOT NULL UNIQUE,
    private_key TEXT NOT NULL,
    public_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'grace', 'retired')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    grace_until TIMESTAMPTZ,
    retired_at TIMESTAMPTZ
);

CREATE INDEX idx_auth_signing_keys_status ON auth_signing_keys (status);

CREATE TABLE auth_sessions (
    session_id UUID PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES iam_accounts (user_id) ON DELETE CASCADE,
    credential_version BIGINT NOT NULL CHECK (credential_version > 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoke_reason TEXT,
    last_refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_sessions_user_status ON auth_sessions (user_id, status);
CREATE INDEX idx_auth_sessions_expires ON auth_sessions (expires_at) WHERE status = 'active';

CREATE TABLE auth_refresh_tokens (
    token_hash BYTEA PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES auth_sessions (session_id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'consumed')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    replaced_by_hash BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_auth_refresh_tokens_active_session
    ON auth_refresh_tokens (session_id) WHERE status = 'active';
CREATE INDEX idx_auth_refresh_tokens_session ON auth_refresh_tokens (session_id);

CREATE TABLE auth_activation_tokens (
    token_hash BYTEA PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES iam_accounts (user_id) ON DELETE CASCADE,
    purpose TEXT NOT NULL CHECK (purpose IN ('account_activation', 'password_reset')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_auth_activation_tokens_pending_user
    ON auth_activation_tokens (user_id) WHERE consumed_at IS NULL;
CREATE INDEX idx_auth_activation_tokens_expires
    ON auth_activation_tokens (expires_at) WHERE consumed_at IS NULL;

CREATE FUNCTION auth_revoke_sessions_on_account_change() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status <> OLD.status OR NEW.credential_version <> OLD.credential_version THEN
        UPDATE auth_sessions
        SET status = 'revoked',
            revoked_at = COALESCE(revoked_at, now()),
            revoke_reason = CASE
                WHEN NEW.status <> OLD.status THEN 'account_status_changed'
                ELSE 'credential_changed'
            END,
            updated_at = now()
        WHERE user_id = NEW.user_id AND status = 'active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_auth_revoke_sessions_on_account_change
    AFTER UPDATE OF status, credential_version ON iam_accounts
    FOR EACH ROW EXECUTE FUNCTION auth_revoke_sessions_on_account_change();

CREATE TABLE auth_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    principal_type TEXT NOT NULL CHECK (principal_type IN ('user', 'admin')),
    user_id TEXT,
    jti TEXT,
    request_id TEXT,
    decision TEXT NOT NULL CHECK (decision IN ('success', 'deny', 'error')),
    reason_code TEXT,
    reason_message TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_audit_logs_event_time ON auth_audit_logs (event_type, created_at DESC);

-- Billing and payment
-- All billing ledger amounts use micro-USD ($1 = 1,000,000 micro-USD).
--
-- bill_accounts.balance_micro is the single authority for "how much money does
-- this account have". It is deliberately SIGNED and deliberately has no CHECK:
-- a negative balance IS the debt. Nothing else in this schema may represent a
-- balance, and no reader may reassemble one from other tables.
--
--   admission  = balance_micro > 0
--   settlement = balance_micro -= cost   (may go negative, never blocked)
--   top-up     = balance_micro += amount (negative balance is absorbed)
CREATE TABLE bill_accounts (
    account_id TEXT PRIMARY KEY,
    account_kind SMALLINT NOT NULL CHECK (account_kind IN (1, 2)),
    tenant_id TEXT NOT NULL,
    balance_micro BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bill_accounts_tenant ON bill_accounts (tenant_id, account_kind);
CREATE INDEX idx_bill_accounts_negative ON bill_accounts (account_id) WHERE balance_micro <= 0;

-- Every tenant and every end user has exactly one account row, guaranteed here
-- rather than at each of the several places that create them. Admission reads
-- this table and fails closed on a missing row, so "someone forgot to
-- provision" would take a caller offline; making it structural removes the
-- possibility instead of documenting it.
CREATE FUNCTION bill_provision_account() RETURNS TRIGGER AS $$
BEGIN
    IF TG_TABLE_NAME = 'iam_tenants' THEN
        INSERT INTO bill_accounts (account_id, account_kind, tenant_id)
        VALUES (NEW.tenant_id, 1, NEW.tenant_id)
        ON CONFLICT (account_id) DO NOTHING;
    ELSIF NEW.user_type = 4 AND NEW.tenant_id IS NOT NULL THEN
        INSERT INTO bill_accounts (account_id, account_kind, tenant_id)
        VALUES (NEW.user_id, 2, NEW.tenant_id)
        ON CONFLICT (account_id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_bill_provision_tenant_account
    AFTER INSERT ON iam_tenants
    FOR EACH ROW EXECUTE FUNCTION bill_provision_account();

CREATE TRIGGER trg_bill_provision_user_account
    AFTER INSERT ON iam_accounts
    FOR EACH ROW EXECUTE FUNCTION bill_provision_account();

CREATE TABLE bill_recharge_orders (
    id BIGSERIAL PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE,
    order_type TEXT NOT NULL CONSTRAINT bill_recharge_orders_order_type_check
        CHECK (order_type IN ('platform_to_tenant', 'tenant_to_user',
                              'online_user_topup', 'online_tenant_topup', 'user_topup_income')),
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    credit_amount BIGINT NOT NULL CHECK (credit_amount > 0),
    paid_amount BIGINT NOT NULL DEFAULT 0 CHECK (paid_amount >= 0),
    payment_ref TEXT,
    payment_order_id TEXT,
    expires_at TIMESTAMPTZ,
    operator_id TEXT NOT NULL,
    note TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'reversed')),
    reversed_at TIMESTAMPTZ,
    reversed_by TEXT,
    reversal_reason TEXT,
    reversed_amount_micro BIGINT NOT NULL DEFAULT 0 CHECK (reversed_amount_micro >= 0),
    lost_amount_micro BIGINT NOT NULL DEFAULT 0 CHECK (lost_amount_micro >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bill_recharge_orders_user_id_pairing_check CHECK (
        (order_type IN ('platform_to_tenant', 'online_tenant_topup', 'user_topup_income') AND user_id IS NULL)
        OR (order_type IN ('tenant_to_user', 'online_user_topup') AND user_id IS NOT NULL)
    ),
    CONSTRAINT bill_recharge_orders_reversal_amount_check CHECK (
        reversed_amount_micro + lost_amount_micro <= credit_amount
    )
);

CREATE INDEX idx_bill_recharge_orders_tenant ON bill_recharge_orders (order_type, tenant_id, created_at DESC);
CREATE INDEX idx_bill_recharge_orders_user ON bill_recharge_orders (user_id, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_bill_recharge_orders_status ON bill_recharge_orders (status, created_at DESC);
CREATE UNIQUE INDEX uq_bill_recharge_orders_user_topup_income_payment_ref
    ON bill_recharge_orders (payment_ref)
    WHERE order_type = 'user_topup_income' AND payment_ref IS NOT NULL;

-- Credit lots record WHERE a grant came from and WHEN it expires. They are an
-- attribution detail, never the balance: bill_accounts.balance_micro is.
-- Lot state is derived, not stored: a lot is spent when
-- consumed_micro >= granted_micro, gone when expired_at/revoked_at is set.
CREATE TABLE bill_credit_lots (
    id BIGSERIAL PRIMARY KEY,
    lot_id TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES bill_accounts (account_id),
    granted_micro BIGINT NOT NULL CHECK (granted_micro > 0),
    consumed_micro BIGINT NOT NULL DEFAULT 0 CHECK (consumed_micro >= 0),
    expires_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    expired_unused_micro BIGINT CHECK (
        expired_unused_micro IS NULL
        OR (expired_unused_micro >= 0 AND expired_unused_micro <= granted_micro)
    ),
    revoked_at TIMESTAMPTZ,
    source TEXT NOT NULL CONSTRAINT bill_credit_lots_source_check
        CHECK (source IN ('ADMIN_RECHARGE', 'TENANT_RECHARGE', 'REFUND', 'ONLINE_TOPUP', 'USER_TOPUP_INCOME')),
    recharge_order_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bill_credit_lots_fifo
    ON bill_credit_lots (account_id, expires_at NULLS LAST, created_at)
    WHERE expired_at IS NULL AND revoked_at IS NULL;
CREATE INDEX idx_bill_credit_lots_due
    ON bill_credit_lots (expires_at)
    WHERE expired_at IS NULL AND revoked_at IS NULL AND expires_at IS NOT NULL;
CREATE INDEX idx_bill_credit_lots_order
    ON bill_credit_lots (recharge_order_id)
    WHERE recharge_order_id IS NOT NULL;

-- Settlement outbox. The AI runtime writes one row in the SAME transaction as
-- the ai_usage_logs insert, so a usage record and its charge can never diverge:
-- if the usage row exists, the charge is owed and will be applied. A background
-- consumer drains it with FOR UPDATE SKIP LOCKED, which keeps it safe across
-- instances and makes head-of-line blocking on a poison row impossible.
CREATE TABLE bill_charge_outbox (
    id BIGSERIAL PRIMARY KEY,
    request_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    tenant_micro BIGINT NOT NULL DEFAULT 0 CHECK (tenant_micro >= 0),
    user_micro BIGINT NOT NULL DEFAULT 0 CHECK (user_micro >= 0),
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at TIMESTAMPTZ
);

CREATE INDEX idx_bill_charge_outbox_pending ON bill_charge_outbox (id) WHERE status = 'pending';
CREATE INDEX idx_bill_charge_outbox_failed ON bill_charge_outbox (created_at DESC) WHERE status = 'failed';

CREATE TABLE pay_orders (
    id BIGSERIAL PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE,
    out_trade_no TEXT NOT NULL UNIQUE,
    scene TEXT NOT NULL CHECK (scene IN ('user_topup', 'tenant_topup')),
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    topup_mode TEXT NOT NULL DEFAULT 'custom' CONSTRAINT pay_orders_topup_mode_check
        CHECK (topup_mode IN ('custom', 'package')),
    package_id TEXT,
    package_name TEXT,
    package_badge TEXT,
    payment_currency TEXT NOT NULL DEFAULT 'USD' CHECK (payment_currency = 'USD'),
    payment_amount_minor BIGINT NOT NULL CHECK (payment_amount_minor > 0),
    ledger_currency TEXT NOT NULL DEFAULT 'USD' CHECK (ledger_currency = 'USD'),
    gross_amount_micro_usd BIGINT NOT NULL CHECK (gross_amount_micro_usd > 0),
    fee_amount_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK (fee_amount_micro_usd >= 0),
    gift_amount_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK (gift_amount_micro_usd >= 0),
    credited_amount_micro_usd BIGINT NOT NULL CHECK (credited_amount_micro_usd > 0),
    fee_rate_bp INTEGER NOT NULL DEFAULT 0,
    tenant_income_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK (tenant_income_micro_usd >= 0),
    balance_expires_at TIMESTAMPTZ,
    channel TEXT NOT NULL DEFAULT 'wechat_native',
    code_url TEXT,
    transaction_id TEXT,
    status TEXT NOT NULL DEFAULT 'created' CHECK (status IN ('created', 'paying', 'paid', 'closed', 'expired')),
    fulfillment_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (fulfillment_status IN ('pending', 'credited', 'partially_reversed', 'reversed')),
    refund_status TEXT NOT NULL DEFAULT 'none'
        CHECK (refund_status IN ('none', 'refunded')),
    paid_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    balance_order_id TEXT,
    fail_note TEXT,
    sweep_attempts INTEGER NOT NULL DEFAULT 0 CHECK (sweep_attempts >= 0),
    sweep_next_attempt_at TIMESTAMPTZ,
    sweep_last_attempt_at TIMESTAMPTZ,
    sweep_last_error TEXT,
    notify_raw JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (scene = 'user_topup' AND user_id IS NOT NULL)
        OR (scene = 'tenant_topup' AND user_id IS NULL)
    ),
    CONSTRAINT pay_orders_fulfillment_pairing_check CHECK (
        (status = 'paid' AND fulfillment_status IN ('credited', 'partially_reversed', 'reversed'))
        OR (status <> 'paid' AND fulfillment_status = 'pending')
    ),
    CONSTRAINT pay_orders_refund_pairing_check CHECK (
        refund_status = 'none'
        OR (refund_status = 'refunded' AND status = 'paid' AND fulfillment_status = 'reversed')
    )
);

ALTER TABLE bill_recharge_orders
    ADD CONSTRAINT bill_recharge_orders_payment_order_fk
    FOREIGN KEY (payment_order_id) REFERENCES pay_orders (order_id),
    ADD CONSTRAINT bill_recharge_orders_payment_link_check CHECK (
        (order_type IN ('online_user_topup', 'online_tenant_topup', 'user_topup_income') AND payment_order_id IS NOT NULL)
        OR (order_type IN ('platform_to_tenant', 'tenant_to_user') AND payment_order_id IS NULL)
    );

CREATE INDEX idx_bill_recharge_orders_payment_order ON bill_recharge_orders (payment_order_id)
    WHERE payment_order_id IS NOT NULL;

CREATE UNIQUE INDEX uq_pay_orders_txn ON pay_orders (transaction_id) WHERE transaction_id IS NOT NULL;
CREATE INDEX idx_pay_orders_sweep
    ON pay_orders (status, sweep_next_attempt_at, expires_at)
    WHERE status IN ('created', 'paying', 'expired');
CREATE INDEX idx_pay_orders_sweep_retry_health
    ON pay_orders (sweep_last_attempt_at)
    WHERE status IN ('created', 'paying', 'expired') AND sweep_attempts > 0;
CREATE INDEX idx_pay_orders_closed_cleanup ON pay_orders (updated_at)
    WHERE status = 'closed' AND fulfillment_status = 'pending';
CREATE INDEX idx_pay_orders_tenant ON pay_orders (tenant_id, created_at DESC);
CREATE INDEX idx_pay_orders_user ON pay_orders (user_id, created_at DESC) WHERE user_id IS NOT NULL;

CREATE TABLE pay_refunds (
    id BIGSERIAL PRIMARY KEY,
    refund_id TEXT NOT NULL UNIQUE,
    payment_order_id TEXT NOT NULL UNIQUE REFERENCES pay_orders (order_id),
    refund_method TEXT NOT NULL CHECK (refund_method IN ('wechat', 'offline')),
    refund_reference TEXT NOT NULL CHECK (btrim(refund_reference) <> ''),
    channel_refund_id TEXT,
    refund_amount_minor BIGINT NOT NULL CHECK (refund_amount_minor > 0),
    status TEXT NOT NULL DEFAULT 'completed' CHECK (status = 'completed'),
    refunded_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
    note TEXT,
    operator_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pay_refunds_channel_reference_check CHECK (
        refund_method = 'offline'
        OR (channel_refund_id IS NOT NULL AND btrim(channel_refund_id) <> '')
    ),
    UNIQUE (refund_method, refund_reference)
);

CREATE UNIQUE INDEX uq_pay_refunds_channel_refund_id ON pay_refunds (channel_refund_id)
    WHERE channel_refund_id IS NOT NULL;
CREATE INDEX idx_pay_refunds_refunded_at ON pay_refunds (refunded_at DESC);

CREATE TABLE bill_refund_reversal_effects (
    id BIGSERIAL PRIMARY KEY,
    reversal_id TEXT NOT NULL UNIQUE,
    refund_id TEXT NOT NULL REFERENCES pay_refunds (refund_id),
    recharge_order_id TEXT NOT NULL UNIQUE REFERENCES bill_recharge_orders (order_id),
    account_id TEXT NOT NULL REFERENCES bill_accounts (account_id),
    credit_amount_micro BIGINT NOT NULL CHECK (credit_amount_micro > 0),
    available_reclaimed_micro BIGINT NOT NULL CHECK (available_reclaimed_micro >= 0),
    non_available_debit_micro BIGINT NOT NULL CHECK (non_available_debit_micro >= 0),
    expired_amount_micro BIGINT NOT NULL CHECK (expired_amount_micro >= 0),
    account_debit_micro BIGINT NOT NULL CHECK (account_debit_micro >= 0),
    balance_after_micro BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bill_refund_reversal_effects_components_check CHECK (
        available_reclaimed_micro + non_available_debit_micro + expired_amount_micro = credit_amount_micro
        AND available_reclaimed_micro + non_available_debit_micro = account_debit_micro
    )
);

CREATE INDEX idx_bill_refund_reversal_effects_refund ON bill_refund_reversal_effects (refund_id);

CREATE TABLE pay_cash_ledger (
    id BIGSERIAL PRIMARY KEY,
    txn_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    txn_type TEXT NOT NULL CHECK (txn_type IN ('topup_income', 'refund_reversal', 'consumption', 'withdraw', 'adjust')),
    amount_micro_usd BIGINT NOT NULL,
    balance_after_micro_usd BIGINT NOT NULL,
    ref_type TEXT,
    ref_id TEXT,
    operator_id TEXT,
    note TEXT,
    idempotency_key TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pay_cash_ledger_tenant ON pay_cash_ledger (tenant_id, created_at DESC);

CREATE TABLE pay_withdrawals (
    id BIGSERIAL PRIMARY KEY,
    withdrawal_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    amount_micro_usd BIGINT NOT NULL CHECK (amount_micro_usd > 0),
    fee_amount_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK (fee_amount_micro_usd >= 0),
    payout_amount_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK (payout_amount_micro_usd >= 0),
    account_name TEXT NOT NULL,
    bank_name TEXT NOT NULL,
    account_no TEXT NOT NULL,
    apply_note TEXT,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected', 'paid', 'cancelled')),
    applied_by TEXT NOT NULL,
    reviewed_by TEXT,
    reviewed_at TIMESTAMPTZ,
    review_note TEXT,
    paid_by TEXT,
    paid_at TIMESTAMPTZ,
    payment_ref TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pay_withdrawals_tenant ON pay_withdrawals (tenant_id, created_at DESC);
CREATE INDEX idx_pay_withdrawals_status ON pay_withdrawals (status, created_at DESC);

CREATE TABLE sys_settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    updated_by TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 后台数据清理运行记录。清理按批次执行，避免单次长事务锁住大表。
CREATE TABLE sys_data_cleanup_runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trigger      TEXT NOT NULL CHECK (trigger IN ('automatic', 'manual')),
    status       TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    requested_by TEXT,
    targets      JSONB NOT NULL DEFAULT '[]'::jsonb,
    summary      JSONB NOT NULL DEFAULT '{}'::jsonb,
    error        TEXT,
    owner_id     TEXT,
    heartbeat_at TIMESTAMPTZ,
    lease_until  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

-- 整个实例只允许一个清理运行，避免手动清理和定时清理互相争抢。
CREATE UNIQUE INDEX uq_sys_data_cleanup_active
    ON sys_data_cleanup_runs ((1))
    WHERE status IN ('queued', 'running');
CREATE INDEX idx_sys_data_cleanup_runs_created
    ON sys_data_cleanup_runs (created_at DESC);
CREATE INDEX idx_sys_data_cleanup_runs_lease
    ON sys_data_cleanup_runs (lease_until)
    WHERE status IN ('queued', 'running');

CREATE TABLE pay_tenant_settings (
    tenant_id TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    updated_by TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE pay_wechat_config (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    mock BOOLEAN NOT NULL DEFAULT TRUE,
    verify_mode TEXT NOT NULL DEFAULT 'platform_cert'
        CHECK (verify_mode IN ('platform_cert', 'public_key')),
    app_id TEXT,
    mch_id TEXT,
    mch_cert_serial_no TEXT,
    mch_private_key_enc TEXT,
    apiv3_key_enc TEXT,
    wechat_public_key_id TEXT,
    wechat_public_key TEXT,
    notify_base_url TEXT,
    order_ttl_seconds INTEGER NOT NULL DEFAULT 7200
        CONSTRAINT pay_wechat_config_order_ttl_check CHECK (order_ttl_seconds BETWEEN 300 AND 86400),
    updated_by TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Tenant branding
CREATE TABLE iam_tenant_portal_branding (
    tenant_id TEXT PRIMARY KEY REFERENCES iam_tenants(tenant_id) ON DELETE CASCADE,
    customer_site_name TEXT NOT NULL DEFAULT '',
    favicon_png BYTEA,
    favicon_updated_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (char_length(customer_site_name) <= 80)
);

-- Legal acceptances
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

-- Announcements
CREATE TABLE ann_announcements (
    announcement_id TEXT PRIMARY KEY,
    publisher_type TEXT NOT NULL CHECK (publisher_type IN ('platform', 'tenant')),
    publisher_tenant_id TEXT,
    title VARCHAR(200) NOT NULL,
    content_markdown TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'general'
        CHECK (category IN ('general', 'maintenance', 'upgrade', 'pricing', 'security')),
    severity TEXT NOT NULL DEFAULT 'info'
        CHECK (severity IN ('info', 'important', 'critical')),
    display_mode TEXT NOT NULL DEFAULT 'inbox'
        CHECK (display_mode IN ('inbox', 'popup')),
    status TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'archived')),
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    audience_size_at_publish BIGINT CHECK (audience_size_at_publish IS NULL OR audience_size_at_publish >= 0),
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((publisher_type = 'platform' AND publisher_tenant_id IS NULL)
        OR (publisher_type = 'tenant' AND publisher_tenant_id IS NOT NULL)),
    CHECK (ends_at IS NULL OR starts_at IS NULL OR starts_at < ends_at),
    CHECK (char_length(content_markdown) BETWEEN 1 AND 50000)
);

CREATE INDEX idx_ann_visible_window
    ON ann_announcements (status, starts_at, ends_at, published_at DESC);
CREATE INDEX idx_ann_publisher
    ON ann_announcements (publisher_type, publisher_tenant_id, created_at DESC);

CREATE TABLE ann_audiences (
    id BIGSERIAL PRIMARY KEY,
    announcement_id TEXT NOT NULL REFERENCES ann_announcements(announcement_id) ON DELETE CASCADE,
    audience_kind TEXT NOT NULL CHECK (audience_kind IN ('admin', 'tenant_user', 'end_user')),
    scope_type TEXT NOT NULL CHECK (scope_type IN ('all', 'tenant')),
    tenant_id TEXT,
    CHECK ((scope_type = 'all' AND tenant_id IS NULL)
        OR (scope_type = 'tenant' AND tenant_id IS NOT NULL)),
    CHECK (audience_kind <> 'admin' OR scope_type = 'all')
);

CREATE UNIQUE INDEX uq_ann_audience_rule
    ON ann_audiences (announcement_id, audience_kind, scope_type, COALESCE(tenant_id, ''));
CREATE INDEX idx_ann_audience_match
    ON ann_audiences (audience_kind, scope_type, tenant_id, announcement_id);

CREATE TABLE ann_receipts (
    announcement_id TEXT NOT NULL REFERENCES ann_announcements(announcement_id) ON DELETE CASCADE,
    user_type INTEGER NOT NULL CHECK (user_type IN (1, 2, 3, 4)),
    user_id TEXT NOT NULL,
    tenant_id TEXT,
    read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (announcement_id, user_type, user_id)
);

CREATE INDEX idx_ann_receipts_user
    ON ann_receipts (user_type, user_id, read_at DESC);
CREATE INDEX idx_ann_receipts_announcement
    ON ann_receipts (announcement_id, read_at DESC);

CREATE TABLE ann_audit_events (
    id BIGSERIAL PRIMARY KEY,
    announcement_id TEXT NOT NULL,
    event_type TEXT NOT NULL
        CHECK (event_type IN ('created', 'updated', 'published', 'archived', 'draft_deleted')),
    actor_user_type INTEGER NOT NULL CHECK (actor_user_type IN (1, 2, 3)),
    actor_user_id TEXT NOT NULL,
    actor_tenant_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ann_audit_announcement
    ON ann_audit_events (announcement_id, created_at DESC);

-- Credit leases
-- Credit leases are mutable escrow records. The request/usage row is the
-- user-visible AI billing fact; leases only hold temporary settlement state.
CREATE TABLE ledger_credit_leases (
    id BIGSERIAL PRIMARY KEY,
    lease_id TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL,
    client_window_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    description TEXT,
    requested_tenant_micro BIGINT NOT NULL DEFAULT 0 CHECK (requested_tenant_micro >= 0),
    requested_user_micro BIGINT NOT NULL DEFAULT 0 CHECK (requested_user_micro >= 0),
    granted_tenant_micro BIGINT NOT NULL DEFAULT 0 CHECK (granted_tenant_micro >= 0),
    granted_user_micro BIGINT NOT NULL DEFAULT 0 CHECK (granted_user_micro >= 0),
    escrow_state TEXT NOT NULL DEFAULT 'active'
        CHECK (escrow_state IN ('active', 'grace', 'released')),
    settlement_state TEXT NOT NULL DEFAULT 'unsettled'
        CHECK (settlement_state IN ('unsettled', 'settled')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    grace_until TIMESTAMPTZ NOT NULL,
    settlement_id TEXT,
    actual_tenant_micro BIGINT CHECK (actual_tenant_micro IS NULL OR actual_tenant_micro >= 0),
    actual_user_micro BIGINT CHECK (actual_user_micro IS NULL OR actual_user_micro >= 0),
    tenant_deducted_micro BIGINT NOT NULL DEFAULT 0 CHECK (tenant_deducted_micro >= 0),
    user_deducted_micro BIGINT NOT NULL DEFAULT 0 CHECK (user_deducted_micro >= 0),
    tenant_debt_added_micro BIGINT NOT NULL DEFAULT 0 CHECK (tenant_debt_added_micro >= 0),
    user_debt_added_micro BIGINT NOT NULL DEFAULT 0 CHECK (user_debt_added_micro >= 0),
    account_state TEXT NOT NULL DEFAULT 'OK'
        CHECK (account_state IN ('OK', 'OVERDRAFT', 'EXHAUSTED')),
    allow_further_usage BOOLEAN NOT NULL DEFAULT true,
    settled_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (client_id, client_window_id),
    CHECK (requested_tenant_micro > 0 OR requested_user_micro > 0),
    CHECK (granted_tenant_micro > 0 OR granted_user_micro > 0),
    CHECK (granted_tenant_micro <= requested_tenant_micro),
    CHECK (granted_user_micro <= requested_user_micro),
    CHECK (requested_user_micro = 0 OR user_id IS NOT NULL),
    CHECK (granted_user_micro = 0 OR user_id IS NOT NULL),
    CHECK (grace_until > expires_at),
    CHECK (
        (escrow_state = 'released' AND released_at IS NOT NULL)
        OR
        (escrow_state <> 'released' AND released_at IS NULL)
    ),
    CHECK (
        (settlement_state = 'unsettled'
            AND settlement_id IS NULL
            AND actual_tenant_micro IS NULL
            AND actual_user_micro IS NULL
            AND tenant_deducted_micro = 0
            AND user_deducted_micro = 0
            AND tenant_debt_added_micro = 0
            AND user_debt_added_micro = 0
            AND settled_at IS NULL)
        OR
        (settlement_state = 'settled'
            AND settlement_id IS NOT NULL
            AND actual_tenant_micro IS NOT NULL
            AND actual_user_micro IS NOT NULL
            AND tenant_deducted_micro + tenant_debt_added_micro = actual_tenant_micro
            AND user_deducted_micro + user_debt_added_micro = actual_user_micro
            AND settled_at IS NOT NULL
            AND escrow_state = 'released')
    )
);

CREATE UNIQUE INDEX uq_ledger_credit_leases_settlement
    ON ledger_credit_leases (settlement_id)
    WHERE settlement_id IS NOT NULL;

CREATE INDEX idx_ledger_credit_leases_reaper
    ON ledger_credit_leases (escrow_state, expires_at, grace_until)
    WHERE escrow_state IN ('active', 'grace');

CREATE INDEX idx_ledger_credit_leases_account
    ON ledger_credit_leases (tenant_id, user_id, created_at DESC);

-- AI domain
-- ============================================================================
  -- D-AI AI domain canonical schema
  -- ----------------------------------------------------------------------------
  -- 说明：
  -- 1. 本段与上面的身份计费域共同组成 D-AI 新数据库的完整基线。
  -- 2. 项目不保留旧服务的历史迁移链，结构变化直接维护本文件。
  -- 3. 数据库只负责稳定的结构、数值和状态机不变量；易变的业务枚举由服务层校验。
  -- 4. 租户授权关联使用组合外键固定归属；多态路由等无法稳定表达的引用由业务模块事务负责。
  -- ----------------------------------------------------------------------------
  -- 租户自治商业控制面：
  --   1. 平台管理上游资源和公共价格表；租户管理私有价格表、分组、用户和订阅。
  --   2. 用户零售价 = 分组价格表 × 用户有效倍率；租户扣费 = 成功账号价格表 × 账号倍率。
  --      两条计费线独立计算，账号倍率与分组/用户倍率绝不串行相乘。
  --   3. 分组直属租户，绑定完整上游资源；启用模型自动进入分组，缺价模型运行时拒绝。
  --   4. 每个 API key 只绑定一个分组；故障转移在分组内的多上游目标之间完成。
  --   5. 本基线不包含旧平台分组授权和上游真实成本模型。
  -- ============================================================================

  -- Extensions are database-global while this file is applied per schema, so two
  -- sessions loading it at once can both pass IF NOT EXISTS and then collide on
  -- pg_extension_name_index. Swallowing that race keeps concurrent test-schema
  -- loads (and repeated deploys) from failing on a no-op.
  DO $$
  BEGIN
      CREATE EXTENSION IF NOT EXISTS pgcrypto;
  EXCEPTION
      WHEN duplicate_object OR unique_violation THEN NULL;
  END
  $$;

  -- ============================================================================
  -- 定价体系（Price Book 统一定价）
  -- 一套 USD 价格表，上游成本与分组对外售价共用同一目录；各处引用时设单倍率。
  -- 注意：定义在所有引用方（accounts / deployments / groups）之前，便于逻辑引用。
  -- ============================================================================

  -- 价格表：USD 目录（业界官方定价）。可手建，也可从 LiteLLM JSON 自动填充。
  CREATE TABLE IF NOT EXISTS ai_price_books (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type      TEXT        NOT NULL CHECK (owner_type IN ('platform', 'tenant')),
    owner_tenant_id TEXT        NOT NULL DEFAULT '',
    name            TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'active',
    revision        BIGINT      NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ai_price_books_owner_scope_ck CHECK (
      (owner_type = 'platform' AND owner_tenant_id = '') OR
      (owner_type = 'tenant' AND owner_tenant_id <> '')
    ),
    UNIQUE (owner_type, owner_tenant_id, name)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_price_books_visible
    ON ai_price_books (owner_type, owner_tenant_id, status, updated_at DESC);

  -- 价格表条目：每 (price_book, model_code) 一行。
  -- token_price_tiers 原子保存按输入上下文选档的 USD/token 数组。
  --   image_default_price/video_default_price 为默认 USD 单价（每图 / 每秒）。
  --   image_prices/video_prices JSON 内 price 为规格覆盖 USD（每图 / 每秒）。
  --   audio_tts_per_char 为 USD/字符，audio_stt_per_minute 为 USD/分钟。
  -- 缓存价格 0 表示真正免费；reasoning tokens 始终包含在 completion tokens 中按输出价计费。
  -- model_code 为自由字符串（无模型字典外键）：成本价格表可包含未对外转售的上游模型。
  -- manually_edited=true 时 LiteLLM 导入「仅填空」会跳过该条目，不覆盖手改价。
  CREATE TABLE IF NOT EXISTS ai_price_book_entries (
    id                    UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    price_book_id         UUID           NOT NULL,
    model_code            TEXT           NOT NULL,
    capability_type       TEXT           NOT NULL DEFAULT 'chat',
    token_price_tiers     JSONB          NOT NULL DEFAULT '[]',
    image_default_price   NUMERIC(20,12) NOT NULL DEFAULT 0,
    video_default_price   NUMERIC(20,12) NOT NULL DEFAULT 0,
    image_prices          JSONB          NOT NULL DEFAULT '[]',   -- [{"resolution":"1024x1024","price":0.04}]
    video_prices          JSONB          NOT NULL DEFAULT '[]',   -- [{"resolution":"720p","price":0.05}]
    audio_tts_per_char    NUMERIC(20,12) NOT NULL DEFAULT 0,
    audio_stt_per_minute  NUMERIC(20,12) NOT NULL DEFAULT 0,
    source                TEXT           NOT NULL DEFAULT 'manual',
    manually_edited       BOOLEAN        NOT NULL DEFAULT false,
    created_at            TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ    NOT NULL DEFAULT now(),
    UNIQUE (price_book_id, model_code, capability_type),
    CONSTRAINT ai_price_book_entries_nonnegative CHECK (
      image_default_price >= 0 AND video_default_price >= 0
      AND audio_tts_per_char >= 0 AND audio_stt_per_minute >= 0
    ),
    CONSTRAINT ai_price_book_entries_token_tiers_array CHECK (
      jsonb_typeof(token_price_tiers) = 'array'
      AND (capability_type NOT IN ('chat', 'embedding', 'rerank') OR jsonb_array_length(token_price_tiers) > 0)
    ),
    CONSTRAINT ai_price_book_entries_default_price_required CHECK (
      (capability_type <> 'image' OR image_default_price > 0)
      AND (capability_type <> 'video' OR video_default_price > 0)
    )
  );

  CREATE INDEX IF NOT EXISTS idx_ai_price_book_entries_book  ON ai_price_book_entries (price_book_id);
  CREATE INDEX IF NOT EXISTS idx_ai_price_book_entries_model ON ai_price_book_entries (price_book_id, model_code, capability_type);

  -- 全局 AI 设置：key-value。计费币种固定为 USD。
  CREATE TABLE IF NOT EXISTS ai_settings (
    key        TEXT        PRIMARY KEY,
    value      JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  -- AI 出口代理节点。密码使用应用层密钥加密后写入 proxy_password_enc。
  CREATE TABLE IF NOT EXISTS ai_proxy_nodes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    proxy_type          TEXT NOT NULL CHECK (proxy_type IN ('http', 'socks5')),
    endpoint            TEXT NOT NULL CHECK (btrim(endpoint) <> ''),
    username            TEXT NOT NULL DEFAULT '',
    proxy_password_enc  TEXT NOT NULL DEFAULT '',
    weight              INTEGER NOT NULL DEFAULT 1 CHECK (weight > 0 AND weight <= 1000),
    status              TEXT NOT NULL DEFAULT 'disabled',
    health_status       TEXT NOT NULL DEFAULT 'unknown' CHECK (health_status IN ('unknown', 'healthy', 'unhealthy')),
    last_checked_at     TIMESTAMPTZ,
    last_error          TEXT,
    created_by          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
  );
  CREATE INDEX IF NOT EXISTS idx_ai_proxy_nodes_status ON ai_proxy_nodes (status, health_status, updated_at DESC);

  -- 统一通知服务的站内/Webhook 投递记录。
  CREATE TABLE IF NOT EXISTS sys_notification_deliveries (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_key           TEXT NOT NULL,
    channel             TEXT NOT NULL CHECK (channel IN ('in_app', 'webhook')),
    recipient_user_id   TEXT,
    recipient_user_type INTEGER,
    tenant_id           TEXT,
    title               TEXT NOT NULL,
    body                TEXT NOT NULL,
    payload             JSONB NOT NULL DEFAULT '{}'::jsonb,
    status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts            INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error          TEXT,
    idempotency_key     TEXT UNIQUE,
    sent_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
  );
  CREATE INDEX IF NOT EXISTS idx_sys_notification_user ON sys_notification_deliveries (recipient_user_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_sys_notification_status ON sys_notification_deliveries (status, created_at ASC);

  CREATE TABLE IF NOT EXISTS ai_api_keys (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type     TEXT        NOT NULL CHECK (owner_type IN ('user', 'tenant')),
    tenant_id      TEXT        NOT NULL,
    user_id        TEXT,
    group_id       UUID        NOT NULL,
    key_hash       TEXT        NOT NULL,
    key_ciphertext TEXT        NOT NULL,
    last_four      CHAR(4),
    name           TEXT        NOT NULL,
    -- quota_* 列单位：micro-USD。NULL = 无上限。
    -- quota_used 由 ai_usage_logs.api_key_quota_cost 累加。
    quota_limit    BIGINT,
    quota_used     BIGINT      NOT NULL DEFAULT 0,
    status         TEXT        NOT NULL DEFAULT 'active',
    expires_at     TIMESTAMPTZ,
    last_used_at   TIMESTAMPTZ,
    created_by     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((owner_type = 'tenant' AND user_id IS NULL) OR (owner_type = 'user' AND user_id IS NOT NULL)),
    CONSTRAINT ai_api_keys_quota_nonnegative CHECK (
      quota_used >= 0 AND (quota_limit IS NULL OR quota_limit >= 0)
    ),
    UNIQUE (tenant_id, id)
  );

  CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_api_keys_key_hash     ON ai_api_keys (key_hash);
  CREATE INDEX IF NOT EXISTS idx_ai_api_keys_tenant              ON ai_api_keys (tenant_id);
  CREATE INDEX IF NOT EXISTS idx_ai_api_keys_user                ON ai_api_keys (user_id);
  CREATE INDEX IF NOT EXISTS idx_ai_api_keys_group               ON ai_api_keys (group_id);
  CREATE INDEX IF NOT EXISTS idx_ai_api_keys_tenant_status       ON ai_api_keys (tenant_id, status);

  -- ============================================================================
  -- AI Upstream Accounts (上游账号，API Key 型上游；原 ai_providers + ai_provider_endpoints 合并)
  -- 顶级实体，不再有「厂商」父层。OAuth 池型上游见 ai_credential_pools。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_upstream_accounts (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT        NOT NULL UNIQUE,
    tenant_display_name TEXT       NOT NULL,
    tenant_access_mode  TEXT       NOT NULL DEFAULT 'public',
    base_url           TEXT        NOT NULL,
    api_key_ciphertext TEXT        NOT NULL,
    extra_headers      JSONB       NOT NULL DEFAULT '{}',
    default_protocol   TEXT        NOT NULL DEFAULT 'openai_compatible',
    -- 该账号允许同时在飞的出站请求数上限；NULL = 不限制。
    -- 刻意用并发而非 RPM：LLM 请求时长跨数量级（短问答 800ms / 生图 30s / 长流式 3min），
    -- 每分钟请求数与上游真实资源占用不成正比，而并发数直接对应上游占用、长短请求自适应。
    concurrency_limit  INTEGER     CHECK (concurrency_limit IS NULL OR concurrency_limit > 0),
    -- 租户结算绑定：租户扣费 = entry(price_book, 对外模型)[USD] × tenant_multiplier。
    price_book_id      UUID,
    tenant_multiplier NUMERIC(10,4) CHECK (tenant_multiplier IS NULL OR tenant_multiplier >= 0),
    -- 平台向该上游实付的折扣倍率（如中转站「官方价 7 折」= 0.7）。目录价共用 price_book_id，
    -- 与 tenant_multiplier 同基数、分别相乘，二者从不相乘。
    -- 刻意不存平台采购成本：上游多为充值制，实付金额与单次请求的目录价没有函数关系，
    -- 用「目录价 × 折扣」推算只会得到一个精确但错误的数。平台成本由外部账本核算，
    -- 本系统只记录每个账号的产出（见 /api/v1/usage-upstream-summary）。
    -- 三态：active=参与路由；invalid=系统检测到凭据失效；disabled=管理员停用。
    status             TEXT        NOT NULL DEFAULT 'disabled',
    invalid_reason     TEXT        NOT NULL DEFAULT '',
    invalid_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_ai_upstream_accounts_status   ON ai_upstream_accounts (status);

  -- ============================================================================
  -- AI Credential Pools (OAuth 账号池，对应 Codex/Claude OAuth/Gemini CLI 等固定厂商)
  -- 一个 Pool 绑定一种 fixed_provider_type，池内有多个 OAuth Token 账号。
  -- 池是一等路由目标：经 ai_group_targets 被分组直连（与上游账号对等，多对多）。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_credential_pools (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT        NOT NULL,
    tenant_display_name TEXT        NOT NULL,
    tenant_access_mode  TEXT        NOT NULL DEFAULT 'public',
    fixed_provider_type TEXT        NOT NULL,
    oauth_strategy      TEXT        NOT NULL DEFAULT 'round_robin',
    sticky_granularity  TEXT        NOT NULL DEFAULT 'credential',
    -- 池级租户结算绑定，与普通上游账号保持相同语义。
    price_book_id       UUID,
    tenant_multiplier NUMERIC(10,4) CHECK (tenant_multiplier IS NULL OR tenant_multiplier >= 0),
    -- 池同样不存平台采购成本：订阅周期不固定（可能只买一天），按月摊销会严重失真。
    -- 与上游账号一致，成本归外部账本，系统只统计产出。
    notes               TEXT,
    status              TEXT        NOT NULL DEFAULT 'disabled',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_ai_credential_pools_type   ON ai_credential_pools (fixed_provider_type);
  CREATE INDEX IF NOT EXISTS idx_ai_credential_pools_status ON ai_credential_pools (status);

  -- 租户供应目录的统一安全读模型。普通账号与凭据池对租户都表现为上游资源；
  -- 该视图刻意不包含地址、密钥、请求头、OAuth 凭据或内部模型名。
  CREATE OR REPLACE VIEW ai_upstream_resources AS
    SELECT
      id,
      'direct_upstream'::TEXT AS resource_kind,
      name,
      price_book_id,
      tenant_multiplier,
      status,
      created_at,
      updated_at,
      tenant_display_name,
      tenant_access_mode
    FROM ai_upstream_accounts
    UNION ALL
    SELECT
      id,
      'oauth_pool'::TEXT AS resource_kind,
      name,
      price_book_id,
      tenant_multiplier,
      status,
      created_at,
      updated_at,
      tenant_display_name,
      tenant_access_mode
    FROM ai_credential_pools;

  -- 租户上游资源策略。资源生命周期与策略一致性由业务事务维护，不使用外键。
  -- access_granted 控制 restricted 资源是否可见；倍率覆盖为 NULL 时继承资源默认倍率。
  CREATE TABLE IF NOT EXISTS ai_upstream_resource_tenant_policies (
    resource_kind TEXT        NOT NULL CHECK (resource_kind IN ('direct_upstream', 'oauth_pool')),
    resource_id   UUID        NOT NULL,
    tenant_id     TEXT        NOT NULL,
    access_granted BOOLEAN    NOT NULL DEFAULT false,
    tenant_multiplier_override NUMERIC(10,4),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_kind, resource_id, tenant_id)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_upstream_resource_tenant_policies_tenant
    ON ai_upstream_resource_tenant_policies (tenant_id, resource_kind, resource_id);

  -- ============================================================================
  -- AI Upstream Models（显式上游模型绑定）
  -- 一行显式表达“哪个上游目标，可服务哪个对外模型，实际走哪个上游协议/模型名”。
  -- 说明：
  --   1. api_format 使用运行时协议名（openai_chat / gemini_generate / ...）。
  --   2. 不使用外键，由业务层保证关联关系。
  --   3. request/response 协议已合并为单一 api_format——实际使用中两者永远相同。
  --   4. weight 已移除——运行时路由不使用 binding 级 weight，故障转移由
  --      ai_group_targets.priority 控制。
  --   5. priority 已移除——每个 model_code + capability_type 在同一上游上只有
  --      一条绑定，binding 级 priority 作为次级排序键从未实际生效。
  --   6. 唯一约束已收紧为 (upstream_kind, upstream_id, model_code, capability_type)——
  --      从 DB 层面保证每个模型只有一条绑定，消除多协议选择逻辑。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_upstream_models (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    upstream_kind       TEXT        NOT NULL
      CHECK (upstream_kind IN ('direct_upstream', 'oauth_pool')),
    upstream_id         UUID        NOT NULL,
    model_code          TEXT        NOT NULL,
    capability_type     TEXT        NOT NULL,
    api_format          TEXT        NOT NULL,
    upstream_model_name TEXT        NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'active',
    config_json         JSONB       NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (upstream_kind, upstream_id, model_code, capability_type)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_upstream_models_lookup
    ON ai_upstream_models (upstream_kind, upstream_id, model_code, capability_type, status);
  CREATE INDEX IF NOT EXISTS idx_ai_upstream_models_model
    ON ai_upstream_models (model_code, capability_type, api_format, status);

  -- ============================================================================
  -- AI Provider OAuth Credentials (Pool 内的 OAuth Token 账号)
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_provider_oauth_credentials (
    id                       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    pool_id                  UUID        NOT NULL,
    name                     TEXT        NOT NULL,
    provider_type            TEXT        NOT NULL,
    email                    TEXT,
    access_token_ciphertext  TEXT        NOT NULL,
    refresh_token_ciphertext TEXT,
    token_type               TEXT        NOT NULL DEFAULT 'bearer',
    scope                    TEXT,
    expires_at               TIMESTAMPTZ,
    token_version            BIGINT      NOT NULL DEFAULT 1 CHECK (token_version >= 1),
    auth_metadata            JSONB       NOT NULL DEFAULT '{}',
    weight                   INTEGER     NOT NULL DEFAULT 100 CHECK (weight >= 0),
    status                   TEXT        NOT NULL DEFAULT 'active',
    invalid_reason           TEXT,
    cooldown_until           TIMESTAMPTZ,
    last_used_at             TIMESTAMPTZ,
    last_refreshed_at        TIMESTAMPTZ,
    last_failed_at           TIMESTAMPTZ,
    consecutive_fail_count   INTEGER     NOT NULL DEFAULT 0 CHECK (consecutive_fail_count >= 0),
    success_count            BIGINT      NOT NULL DEFAULT 0 CHECK (success_count >= 0),
    fail_count               BIGINT      NOT NULL DEFAULT 0 CHECK (fail_count >= 0),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_oauth_cred_pool_status
    ON ai_provider_oauth_credentials (pool_id, status);
  CREATE INDEX IF NOT EXISTS idx_oauth_cred_pool_cooldown
    ON ai_provider_oauth_credentials (pool_id, cooldown_until)
    WHERE status = 'active' AND cooldown_until IS NOT NULL;
  CREATE INDEX IF NOT EXISTS idx_oauth_cred_expires
    ON ai_provider_oauth_credentials (expires_at)
    WHERE status = 'active' AND refresh_token_ciphertext IS NOT NULL;

  -- ============================================================================
  -- AI Groups（租户直属零售单元）
  -- 分组绑定一张平台公共或本租户私有价格表，并绑定完整上游资源。
  -- 用户倍率覆盖分组默认倍率，不与之相乘。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_groups (
    id                    UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             TEXT          NOT NULL,
    name                  TEXT          NOT NULL,
    description           TEXT          NOT NULL DEFAULT '',
    retail_price_book_id  UUID          NOT NULL,
    default_user_multiplier NUMERIC(10,4) NOT NULL DEFAULT 1 CHECK (default_user_multiplier >= 0),
    user_default_visible  BOOLEAN       NOT NULL DEFAULT false,
    -- 协议转换网关开关（分组级 opt-in）：false（默认）= 仅同家族 passthrough（今天的行为）；
    -- true = 允许把本组候选作为跨格式协议转换目标（client↔provider 落差由 internal/formats 翻译）。
    allow_protocol_conversion BOOLEAN   NOT NULL DEFAULT false,
    sort_order            INTEGER       NOT NULL DEFAULT 0,
    status                TEXT          NOT NULL DEFAULT 'active',
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name),
    UNIQUE (tenant_id, id)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_groups_tenant_status
    ON ai_groups (tenant_id, status, sort_order, updated_at DESC);

  ALTER TABLE ai_api_keys
    ADD CONSTRAINT ai_api_keys_tenant_group_fk
    FOREIGN KEY (tenant_id, group_id) REFERENCES ai_groups (tenant_id, id);

  -- ============================================================================
  -- AI Group Targets (分组 → 上游目标 直连关联)
  -- 分组不再按「模型→部署」路由，而是直连一批上游目标（账号或凭证池），
  -- 候选 = 该组关联的 active 目标里存在对应 ai_upstream_models 绑定者，按 priority 分层；
  -- 同层内再交给运行时 scorer 做动态择优（无实时 stats 时随机/粘性/健康兜底）。
  -- 目标为多态：target_kind + target_id 恰好描述一个上游目标。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_group_targets (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id     UUID        NOT NULL,
    target_kind  TEXT        NOT NULL
      CHECK (target_kind IN ('direct_upstream', 'oauth_pool')),
    target_id    UUID        NOT NULL,
    priority     INTEGER     NOT NULL DEFAULT 100 CHECK (priority >= 0),
    status       TEXT        NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, target_kind, target_id)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_group_targets_group
    ON ai_group_targets (group_id, status, priority);
  CREATE INDEX IF NOT EXISTS idx_ai_group_targets_target
    ON ai_group_targets (target_kind, target_id);

  -- ============================================================================
  -- AI Group Model Dispatch Rules (分组级请求模型调度策略)
  -- 面向客户端请求：requested_model + client_surface 命中本组规则后，先解析成平台
  -- 内部 logical model_code，再进入授权/候选选择/计费。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_group_model_dispatch_rules (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id               UUID        NOT NULL,
    client_surface         TEXT        NOT NULL,
    match_type             TEXT        NOT NULL,
    match_value            TEXT        NOT NULL,
    target_model_code      TEXT        NOT NULL,
    priority               INTEGER     NOT NULL DEFAULT 100 CHECK (priority >= 0),
    status                 TEXT        NOT NULL DEFAULT 'active',
    notes                  TEXT        NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_ai_group_dispatch_rules_group
    ON ai_group_model_dispatch_rules (group_id, client_surface, status, priority, created_at);

  -- ============================================================================
  -- AI Group Client Surfaces (分组可接受的客户端表面)
  -- 一个分组可显式声明它接受哪些 client surface；若同一 surface 的 bridge_enabled = true，
  -- 则允许该 surface 进入跨 surface 协议转换链路。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_group_client_surfaces (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id       UUID        NOT NULL,
    surface        TEXT        NOT NULL,
    bridge_enabled BOOLEAN     NOT NULL DEFAULT false,
    status         TEXT        NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, surface)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_group_client_surfaces_group
    ON ai_group_client_surfaces (group_id, status);

  -- ============================================================================
  -- AI User Groups（租户→用户分组授权及倍率覆盖）
  -- 关系存在即授权；删除关系即撤销。用户有效倍率 = COALESCE(
  -- ai_user_groups.user_multiplier_override, ai_groups.default_user_multiplier)。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_user_groups (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           TEXT          NOT NULL,
    user_id             TEXT          NOT NULL,
    group_id            UUID          NOT NULL,
    user_multiplier_override NUMERIC(10,4) CHECK (user_multiplier_override IS NULL OR user_multiplier_override >= 0),
    created_by          TEXT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, group_id),
    CONSTRAINT ai_user_groups_tenant_group_fk
      FOREIGN KEY (tenant_id, group_id) REFERENCES ai_groups (tenant_id, id)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_user_groups_user  ON ai_user_groups (tenant_id, user_id);
  CREATE INDEX IF NOT EXISTS idx_ai_user_groups_group ON ai_user_groups (group_id);

  -- ============================================================================
  -- Workspace Threads / Messages（P5 主线程模型）
  -- 当前运行时会话正式写入这里。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_workspace_threads (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_scope        TEXT        NOT NULL CHECK (owner_scope IN ('tenant', 'user')),
    tenant_id          TEXT        NOT NULL,
    user_id            TEXT        NOT NULL DEFAULT '',
    target_model_code  TEXT        NOT NULL DEFAULT '',
    selected_group_id  UUID,
    selected_group_name_snapshot TEXT NOT NULL DEFAULT '',
    selected_effective_user_multiplier_snapshot NUMERIC(10,4),
    title              TEXT        NOT NULL DEFAULT '新对话',
    selected_surface   TEXT,
    selected_route_id  UUID,
    status             TEXT        NOT NULL DEFAULT 'active',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (target_model_code <> '')
  );

  CREATE INDEX IF NOT EXISTS idx_ai_workspace_threads_owner
    ON ai_workspace_threads (tenant_id, owner_scope, user_id, updated_at DESC)
    WHERE status <> 'deleted';

  CREATE TABLE IF NOT EXISTS ai_workspace_messages (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id         UUID        NOT NULL,
    role              TEXT        NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
    content_text      TEXT        NOT NULL DEFAULT '',
    content_json      JSONB       NOT NULL DEFAULT '{}',
    client_surface    TEXT,
    upstream_surface  TEXT,
    route_snapshot    JSONB       NOT NULL DEFAULT '{}',
    usage_json        JSONB       NOT NULL DEFAULT '{}',
    error_json        JSONB       NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_ai_workspace_messages_thread
    ON ai_workspace_messages (thread_id, created_at ASC);

  -- ============================================================================
  -- AI Usage Logs (请求明细日志)
  -- provider_format: 实际使用的上游协议（同 upstream_protocol）
  -- cache_write / cache_read / reasoning tokens 独立记录，计费按 input 价格
  -- 说明：group_target_id 记录命中的 ai_group_targets.id；endpoint_id 记录命中的上游账号 id；
  -- credential_pool_id / oauth_credential_id 记录池与实际凭证。group_id 记录候选所属分组。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_usage_logs (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id             TEXT        NOT NULL UNIQUE,
    trace_id               TEXT,
    api_key_id             UUID,
    key_owner_type         TEXT        NOT NULL,
    auth_method            TEXT        NOT NULL DEFAULT 'api_key',
    request_source         TEXT        NOT NULL DEFAULT 'api_key',
    tenant_id              TEXT        NOT NULL,
    user_id                TEXT,
    client_user_agent      TEXT        NOT NULL DEFAULT '',
    external_user_id       TEXT,
    group_id               UUID,
    group_name_snapshot    TEXT        NOT NULL DEFAULT '',
    group_default_user_multiplier_snapshot NUMERIC(10,4),
    user_multiplier_override_snapshot NUMERIC(10,4),
    effective_user_multiplier_snapshot NUMERIC(10,4),
    billing_group_label_snapshot TEXT NOT NULL DEFAULT '',
    model_code             TEXT        NOT NULL,
    requested_model        TEXT        NOT NULL DEFAULT '',
    matched_dispatch_rule_id UUID,
    matched_dispatch_rule_summary TEXT,
    resolved_logical_model TEXT,
    resolved_provider_family TEXT,
    capability_type        TEXT        NOT NULL DEFAULT 'chat',
    group_target_id         UUID,
    endpoint_id            UUID,
    provider_code          TEXT,
    upstream_model         TEXT,
    provider_format        TEXT,
    conversation_id        TEXT,
    stream                 BOOLEAN     NOT NULL DEFAULT false,
    prompt_tokens          INTEGER     NOT NULL DEFAULT 0,
    completion_tokens      INTEGER     NOT NULL DEFAULT 0,
    cache_write_tokens     INTEGER     NOT NULL DEFAULT 0,
    cache_read_tokens      INTEGER     NOT NULL DEFAULT 0,
    reasoning_tokens       INTEGER     NOT NULL DEFAULT 0,
    -- 客户端请求侧推理强度（归一化档位；budget→effort 反映射有档位损失）
    reasoning_effort       TEXT,
    total_tokens           INTEGER     NOT NULL DEFAULT 0,
    billable_unit_type     TEXT        NOT NULL DEFAULT 'token',
    billable_units         BIGINT      NOT NULL DEFAULT 0,
    -- 以下金额列单位均为 micro-USD；1 USD = 1,000,000 micro-USD。
    --
    -- 金额列一律以「付款方」为前缀，`_payable` = 该方应往上游一层支付。链条自下而上：
    --   用户 ──付──▶ 租户 ──付──▶ 平台
    -- 于是毛利就是相邻两项相减，不需要记忆任何组合规则：
    --   租户毛利 = user_payable − tenant_payable
    -- 链条止于平台：平台付给上游多少钱不进本系统（上游多为充值制，实付与单次请求的
    -- 目录价无函数关系），由外部账本核算。平台单位成本 = 账本充值额 ÷ 本系统统计的
    -- 该账号产出（见 /api/v1/usage-upstream-summary）。
    --
    -- 两个基准价（倍率 1，谁都不付，只作基数与参照）：
    --   catalog_base = 命中上游资源价格表算出的目录价；
    --   retail_base  = 分组零售价格表算出的零售原价。
    -- 两条各自只乘一个倍率，两个倍率从不相乘：
    --   tenant_payable = catalog_base × 资源.tenant_multiplier （租户应付平台）
    --   user_payable   = retail_base  × 有效用户倍率           （用户应付租户）
    --
    -- user_charged 是实际扣款，与 user_payable 的差额即减免；差额原因由 billing_source
    -- 单一判别（见下方 CHECK），不另存冗余列——可推导的金额再存一份就多一个能对不上的来源。
    catalog_base           BIGINT      NOT NULL DEFAULT 0,
    tenant_payable         BIGINT      NOT NULL DEFAULT 0,
    retail_base            BIGINT      NOT NULL DEFAULT 0,
    user_payable           BIGINT      NOT NULL DEFAULT 0,
    user_charged           BIGINT      NOT NULL DEFAULT 0,
    api_key_quota_cost     BIGINT      NOT NULL DEFAULT 0,
    service_tier           TEXT        NOT NULL DEFAULT 'standard',
    billing_breakdown      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    billing_window_id      TEXT,
    settlement_batch_id    UUID,
    settled_at             TIMESTAMPTZ,
    billing_status         TEXT        NOT NULL,
    settlement_error      TEXT,
    refund_status         TEXT        NOT NULL DEFAULT 'none',
    refund_reason         TEXT,
    refund_operator_id    TEXT,
    refunded_at           TIMESTAMPTZ,
    request_status         TEXT        NOT NULL,
    http_status            INTEGER,
    upstream_status        INTEGER,
    latency_ms             INTEGER,
    first_token_latency_ms INTEGER,
    request_total_ms       INTEGER,
    request_setup_ms       INTEGER,
    first_response_byte_ms INTEGER,
    response_tail_ms       INTEGER,
    final_attempt_header_ms INTEGER,
    final_attempt_total_ms  INTEGER,
    error_code             TEXT,
    error_message          TEXT,
    oauth_credential_id    UUID,
    credential_pool_id     UUID,
    attempts_count         INT         NOT NULL DEFAULT 1,
    final_route_id         UUID,
    client_protocol        TEXT        NOT NULL DEFAULT 'openai_chat',
    resolution             TEXT,
    protocol_conversion_enabled BOOLEAN NOT NULL DEFAULT false,
    upstream_model_mapping_applied BOOLEAN NOT NULL DEFAULT false,
    public_response_model  TEXT,
    usage_estimated        BOOLEAN     NOT NULL DEFAULT false,
    token_usage_source     TEXT        NOT NULL DEFAULT 'upstream',
    -- 计费来源：payg 按量 / subscription 订阅覆盖（gate 决策落快照，见 ai_sub_* 表）
    billing_source         TEXT        NOT NULL DEFAULT 'payg' CHECK (billing_source IN ('payg', 'subscription')),
    subscription_id        UUID,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (billable_unit_type IN ('token', 'input_token', 'output_token', 'image', 'second', 'request')),
    CONSTRAINT ai_usage_logs_nonnegative CHECK (
      prompt_tokens >= 0 AND completion_tokens >= 0
      AND cache_write_tokens >= 0 AND cache_read_tokens >= 0 AND reasoning_tokens >= 0
      AND total_tokens >= 0 AND billable_units >= 0
      AND catalog_base >= 0 AND tenant_payable >= 0 AND retail_base >= 0
      AND user_payable >= 0 AND user_charged >= 0 AND api_key_quota_cost >= 0
    ),
    -- 实扣与应付的关系由 billing_source 单一判别，且在库层强制，不靠调用方自觉：
    --   任何情况下实扣不得超过应付；
    --   按量计费必须全额扣（两者相等，租户自有 key 时同为 0）；
    --   订阅覆盖必须实扣为 0 且指向具体订阅。
    -- 将来出促销/免费额度，应新增 billing_source 取值并在此显式声明其规则，
    -- 而不是在既有取值下悄悄让金额对不上。
    CONSTRAINT ai_usage_logs_charge_semantics CHECK (
      user_charged <= user_payable
      AND (billing_source <> 'payg' OR user_charged = user_payable)
      AND (billing_source <> 'subscription' OR (user_charged = 0 AND subscription_id IS NOT NULL))
    ),
    CONSTRAINT ai_usage_logs_billing_status_check CHECK (billing_status IN ('free', 'pending', 'settled', 'failed')),
    CONSTRAINT ai_usage_logs_refund_status_check CHECK (
      (refund_status = 'none' AND refunded_at IS NULL)
      OR (refund_status = 'refunded' AND refunded_at IS NOT NULL)
    )
  );

  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_tenant_time       ON ai_usage_logs (tenant_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_user_time         ON ai_usage_logs (user_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_key_time          ON ai_usage_logs (api_key_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_group_time        ON ai_usage_logs (group_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_model_time        ON ai_usage_logs (model_code, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_source_time       ON ai_usage_logs (request_source, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_subscription      ON ai_usage_logs (subscription_id, created_at DESC) WHERE subscription_id IS NOT NULL;
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_billing_status ON ai_usage_logs (billing_status, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_route             ON ai_usage_logs (group_target_id);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_tenant_error_time ON ai_usage_logs (tenant_id, created_at DESC)
    WHERE request_status = 'failed';
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_oauth_cred ON ai_usage_logs (oauth_credential_id)
    WHERE oauth_credential_id IS NOT NULL;
  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_pool ON ai_usage_logs (credential_pool_id)
    WHERE credential_pool_id IS NOT NULL;

  -- ============================================================================
  -- AI Usage Hourly Rollups (小时级预聚合，含 cache token 字段)
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_usage_rollups_hourly (
    bucket_start           TIMESTAMPTZ NOT NULL,
    tenant_id              TEXT        NOT NULL,
    user_id                TEXT        NOT NULL DEFAULT '',
    api_key_id             UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    request_source         TEXT        NOT NULL DEFAULT 'api_key',
    capability_type        TEXT        NOT NULL DEFAULT 'chat',
    model_code             TEXT        NOT NULL,
    provider_code          TEXT        NOT NULL DEFAULT '',
    request_status         TEXT        NOT NULL,
    billable_unit_type     TEXT        NOT NULL,
    request_count          BIGINT      NOT NULL DEFAULT 0,
    success_count          BIGINT      NOT NULL DEFAULT 0,
    failed_count           BIGINT      NOT NULL DEFAULT 0,
    prompt_tokens          BIGINT      NOT NULL DEFAULT 0,
    completion_tokens      BIGINT      NOT NULL DEFAULT 0,
    cache_write_tokens     BIGINT      NOT NULL DEFAULT 0,
    cache_read_tokens      BIGINT      NOT NULL DEFAULT 0,
    reasoning_tokens       BIGINT      NOT NULL DEFAULT 0,
    total_tokens           BIGINT      NOT NULL DEFAULT 0,
    billable_units         BIGINT      NOT NULL DEFAULT 0,
    catalog_base      BIGINT      NOT NULL DEFAULT 0,
    tenant_payable          BIGINT      NOT NULL DEFAULT 0,
    retail_base            BIGINT      NOT NULL DEFAULT 0,
    user_payable            BIGINT      NOT NULL DEFAULT 0,
    user_charged             BIGINT      NOT NULL DEFAULT 0,
    api_key_quota_cost     BIGINT      NOT NULL DEFAULT 0,
    latency_success_sum_ms BIGINT      NOT NULL DEFAULT 0,
    latency_success_count  BIGINT      NOT NULL DEFAULT 0,
    request_total_success_sum_ms BIGINT NOT NULL DEFAULT 0,
    request_total_success_count  BIGINT NOT NULL DEFAULT 0,
    first_response_byte_success_sum_ms BIGINT NOT NULL DEFAULT 0,
    first_response_byte_success_count  BIGINT NOT NULL DEFAULT 0,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (
      bucket_start, tenant_id, user_id, api_key_id, request_source,
      model_code, provider_code, request_status, billable_unit_type
    ),
    CHECK (billable_unit_type IN ('token', 'input_token', 'output_token', 'image', 'second', 'request'))
  );

  CREATE INDEX IF NOT EXISTS idx_ai_usage_rollups_hourly_time
    ON ai_usage_rollups_hourly (bucket_start DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_rollups_hourly_tenant_time
    ON ai_usage_rollups_hourly (tenant_id, bucket_start DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_usage_rollups_hourly_tenant_user_time
    ON ai_usage_rollups_hourly (tenant_id, user_id, bucket_start DESC);

  -- ============================================================================
  -- AI Runtime Limit Policies (限流策略，按主体生效：tenant/user/api_key)
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_runtime_limit_policies (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_type        TEXT        NOT NULL CHECK (scope_type IN ('tenant', 'user', 'api_key')),
    scope_id          TEXT        NOT NULL,
    concurrency_limit INTEGER     CHECK (concurrency_limit IS NULL OR concurrency_limit > 0),
    status            TEXT        NOT NULL DEFAULT 'active',
    created_by        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_ai_runtime_limit_policies_lookup
    ON ai_runtime_limit_policies (scope_type, scope_id, status);

  -- ============================================================================
  -- AI Conversation Bindings (会话粘性路由)
  -- 保证同 conversation_id 的请求路由到同一 upstream deployment
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_conversation_bindings (
    conversation_id        TEXT        NOT NULL,
    tenant_id              TEXT        NOT NULL,
    identity_id            TEXT        NOT NULL,
    model_id               UUID        NOT NULL,
    upstream_deployment_id UUID        NOT NULL,
    account_id             UUID,       -- 命中的上游账号 id（pool 路由为 NULL）
    expires_at             TIMESTAMPTZ NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, tenant_id, identity_id, model_id)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_conversation_bindings_expires ON ai_conversation_bindings (expires_at);

  -- ============================================================================
  -- AI Async Tasks (通用异步任务队列：生图、视频生成、批量推理等)
  -- 领取靠 SKIP LOCKED + 租约；任何能力注册一个 handler 即可接入。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_async_tasks (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type          TEXT        NOT NULL,
    tenant_id          TEXT        NOT NULL,
    user_id            TEXT,
    api_key_id         UUID,
    model_code         TEXT        NOT NULL,
    -- 脱敏、自包含：worker 唯一能拿到的执行输入。任何能力想接异步，
    -- 都必须能只靠这一列重建整个执行。
    input_payload      JSONB       NOT NULL,
    status             TEXT        NOT NULL DEFAULT 'pending',
    result_payload     JSONB,
    error_code         TEXT,
    error_message      TEXT,
    -- 仅管理员详情接口可查：未脱敏/未截断的真实底层错误（Go 错误链 + 上游原始报文全文）
    internal_error_detail TEXT,
    failed_step        VARCHAR(64),
    -- 计费是纯后扣：提交时的准入闸门只拒绝，不预扣，因此没有 estimated_cost。
    caller_charge        BIGINT      NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at         TIMESTAMPTZ,
    completed_at       TIMESTAMPTZ,
    expires_at         TIMESTAMPTZ,
    -- 提交者的引用而非快照：每次尝试前重新解析，于是吊销的密钥、耗尽的配额、
    -- 改动的分组授权，对排队中的任务同样生效。
    -- auth_method 逐字存 identity.AuthMethod 枚举值，不另造同义词。
    auth_method        TEXT        NOT NULL DEFAULT 'jwt',
    -- 租约：持有者定期续期，过期才允许被别的实例接管。
    worker_id          TEXT,
    lease_expires_at   TIMESTAMPTZ,
    available_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempt_count      INT         NOT NULL DEFAULT 0,
    max_attempts       INT         NOT NULL DEFAULT 1,
    -- 本次尝试的 ai_usage_logs.request_id，在上游被调用之前就写好。
    -- 既是对账锚，也是 reaper 的双花守卫。
    request_id         TEXT,
    idempotency_key    TEXT,
    idempotency_scope  TEXT,
    idempotency_fingerprint BYTEA,
    -- 客户端自己的业务标注，原样回显。不是执行输入，不进 handler 视野。
    metadata           JSONB,
    webhook_url        TEXT,
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT ai_async_tasks_attempts_sane CHECK (attempt_count >= 0 AND max_attempts >= 1),
    CONSTRAINT ai_async_tasks_cost_nonnegative CHECK (caller_charge >= 0)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_async_tasks_tenant_status ON ai_async_tasks (tenant_id, status, created_at DESC);
  -- 服务领取查询的 PARTITION BY tenant_id ORDER BY created_at。
  CREATE INDEX IF NOT EXISTS idx_ai_async_tasks_pending
    ON ai_async_tasks (tenant_id, created_at)
    WHERE status = 'pending';
  CREATE INDEX IF NOT EXISTS idx_ai_async_tasks_lease
    ON ai_async_tasks (lease_expires_at)
    WHERE status = 'running';
  -- 无 task_type 谓词：新增能力不需要改索引，task_type 只是过滤条件。
  CREATE INDEX IF NOT EXISTS idx_ai_async_tasks_owner_created
    ON ai_async_tasks (tenant_id, COALESCE(user_id, ''), created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_async_tasks_expires_at
    ON ai_async_tasks (expires_at)
    WHERE expires_at IS NOT NULL;
  -- 幂等键按凭据隔离：同租户两把密钥属于两套集成，各自的 retry-1 不该撞车。
  CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_async_tasks_idempotency
    ON ai_async_tasks (idempotency_scope, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
  CREATE INDEX IF NOT EXISTS idx_ai_async_tasks_request_id
    ON ai_async_tasks (request_id)
    WHERE request_id IS NOT NULL;

  -- Webhook 投递自成一个排队作业（领取 + 租约 + 尝试 + 退避），且它那几列每次
  -- 重试都在写 —— 放在任务行上会拖累每次领取和心跳都要写的热行。
  CREATE TABLE IF NOT EXISTS ai_async_task_deliveries (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id          UUID        NOT NULL,
    url              TEXT        NOT NULL,
    payload          JSONB       NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'pending',
    attempt_count    INT         NOT NULL DEFAULT 0,
    max_attempts     INT         NOT NULL DEFAULT 6,
    available_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_expires_at TIMESTAMPTZ,
    worker_id        TEXT,
    last_status_code INT,
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at     TIMESTAMPTZ,
    CONSTRAINT ai_async_task_deliveries_status
      CHECK (status IN ('pending', 'running', 'delivered', 'failed')),
    CONSTRAINT ai_async_task_deliveries_attempts_sane
      CHECK (attempt_count >= 0 AND max_attempts >= 1)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_async_task_deliveries_pending
    ON ai_async_task_deliveries (available_at)
    WHERE status = 'pending';
  CREATE INDEX IF NOT EXISTS idx_ai_async_task_deliveries_lease
    ON ai_async_task_deliveries (lease_expires_at)
    WHERE status = 'running';
  CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_async_task_deliveries_task
    ON ai_async_task_deliveries (task_id);

  -- ==========================================================================
  -- FileStore (短期平台文件资产 + 不透明公开能力链接)
  -- 业务记录只保存 media://<id>；真实本地路径仅由 FileStore 使用。
  -- ==========================================================================
  CREATE TABLE IF NOT EXISTS file_assets (
    id           UUID        PRIMARY KEY,
    storage_key  TEXT        NOT NULL UNIQUE,
    content_type TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL CHECK (size_bytes > 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    cleanup_owner TEXT,
    cleanup_lease_until TIMESTAMPTZ
  );

  CREATE INDEX IF NOT EXISTS idx_file_assets_expires_at
    ON file_assets (expires_at);
  CREATE INDEX IF NOT EXISTS idx_file_assets_cleanup_lease
    ON file_assets (expires_at, cleanup_lease_until);

  CREATE TABLE IF NOT EXISTS file_access_links (
    token_hash BYTEA       PRIMARY KEY,
    asset_id   UUID        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
  );

  CREATE INDEX IF NOT EXISTS idx_file_access_links_expires_at
    ON file_access_links (expires_at);

  -- ============================================================================
  -- AI Admin Audit Logs (管理操作审计)
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_admin_audit_logs (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor           TEXT,
    actor_tenant_id TEXT,
    action          TEXT        NOT NULL,
    object_type     TEXT,
    object_id       TEXT,
    request_summary JSONB       NOT NULL DEFAULT '{}',
    result          TEXT        NOT NULL,
    http_status     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_ai_admin_audit_logs_time   ON ai_admin_audit_logs (created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_admin_audit_logs_object ON ai_admin_audit_logs (object_type, object_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_admin_audit_logs_actor  ON ai_admin_audit_logs (actor, created_at DESC);

  -- ============================================================================
  -- AI Risk Control (风控中心：内容安全审核 v2)
  -- 配置存于 ai_settings.risk_control_config（JSONB，见 internal/domain/risk_control.go）。
  -- v2: keyword.entries 分级词条 + 拼音独立词库 + config_revision 版本号。
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_content_moderation_logs (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id           TEXT,
    tenant_id            TEXT,
    user_id              TEXT,
    api_key_id           UUID,
    model_code           TEXT,
    capability_type      TEXT,
    mode                 TEXT        NOT NULL,
    action               TEXT        NOT NULL,
    flagged              BOOLEAN     NOT NULL DEFAULT false,
    matched_keyword      TEXT,
    highest_category     TEXT,
    highest_score        NUMERIC(8,6),
    category_scores      JSONB,
    threshold_snapshot   JSONB,
    input_excerpt        TEXT,
    upstream_latency_ms  INTEGER,
    error                TEXT,
    hit_layer            TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_ai_content_moderation_logs_time    ON ai_content_moderation_logs (created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_content_moderation_logs_flagged ON ai_content_moderation_logs (flagged, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_content_moderation_logs_tenant  ON ai_content_moderation_logs (tenant_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_content_moderation_logs_user    ON ai_content_moderation_logs (user_id, created_at DESC);

  -- 风险事件：内容违规累计达到阈值后生成，人工在环处置（不自动改用户状态）。
  -- event_type 预留给未来其他风控事件类型（如二期的异常流量检测）复用同一张表。
  CREATE TABLE IF NOT EXISTS ai_risk_events (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type       TEXT        NOT NULL DEFAULT 'content_violation',
    severity         TEXT        NOT NULL DEFAULT 'medium',
    tenant_id        TEXT,
    user_id          TEXT,
    source_log_id    UUID,
    summary          TEXT        NOT NULL,
    detail           JSONB       NOT NULL DEFAULT '{}',
    status           TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'resolved', 'dismissed')),
    resolved_by      TEXT,
    resolved_at      TIMESTAMPTZ,
    resolution_note  TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_ai_risk_events_status ON ai_risk_events (status, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_risk_events_user    ON ai_risk_events (user_id, created_at DESC);

  INSERT INTO ai_settings (key, value) VALUES ('risk_control_config', '{
    "enabled": false,
    "mode": "observe",
    "config_revision": 1,
    "keyword": {
      "enabled": true,
      "entries": [],
      "homoglyph_map_extra": {},
      "pinyin": {"enabled": false, "entries": [], "include_initials": false}
    },
    "provider": {"base_url": "https://api.openai.com", "model": "omni-moderation-latest", "api_key_ciphertext": "", "timeout_ms": 5000},
    "thresholds": {
      "harassment": 0.7, "harassment/threatening": 0.7, "hate": 0.7, "hate/threatening": 0.7,
      "illicit": 0.7, "illicit/violent": 0.7,
      "self-harm": 0.7, "self-harm/intent": 0.7, "self-harm/instructions": 0.7,
      "sexual": 0.8, "sexual/minors": 0.5,
      "violence": 0.8, "violence/graphic": 0.8
    },
    "scope_group_ids": [],
    "sample_rate": 1.0,
    "verdict_cache_ttl_seconds": 600,
    "violation_window_hours": 24,
    "risk_event_threshold": 3,
    "record_non_hits": false,
    "block_status_code": 451,
    "block_message": "请求内容未通过安全审核"
  }'::jsonb) ON CONFLICT (key) DO NOTHING;

  -- ============================================================================
  -- AI Route Score Weights (多维评分路由权重配置)
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_route_score_weights (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    scope      TEXT        NOT NULL,   -- 'global' | 'tenant:<id>' | 'account:<id>'
    weights    JSONB       NOT NULL,   -- {"cost":0.4,"latency":0.3,"load":0.2,"health":0.1}
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope)
  );

  INSERT INTO ai_route_score_weights (scope, weights)
    VALUES ('global', '{"cost":0.4,"latency":0.3,"load":0.2,"health":0.1}'::jsonb)
    ON CONFLICT (scope) DO NOTHING;

  -- ============================================================================
  -- AI Request Payloads v3 (普通表，业务无感知；归档由存储过程 + 定时任务处理)
  -- 只存 usage_log 没有的 wire 层信息 + 正文 JSONB + 早期失败自带的结果字段。
  -- 归档策略：每月将 6 个月前的数据搬到 ai_request_payloads_archive_YYYY_MM，
  --           然后从主表删除，保持主表始终只保留近期数据。
  -- ============================================================================

  CREATE TABLE IF NOT EXISTS ai_request_payloads (
    id               UUID        NOT NULL DEFAULT gen_random_uuid(),
    request_id       TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    client_protocol  TEXT        NOT NULL,
    client_ip        TEXT,
    user_agent       TEXT,
    request_path     TEXT        NOT NULL,
    auth_masked      TEXT,
    request_model    TEXT        NOT NULL,
    matched_dispatch_rule_id UUID,
    matched_dispatch_rule_summary TEXT,
    resolved_logical_model TEXT,
    resolved_provider_family TEXT,
    protocol_conversion_enabled BOOLEAN NOT NULL DEFAULT false,
    selected_upstream_protocol TEXT,
    selected_upstream_model TEXT,
    upstream_model_mapping_applied BOOLEAN NOT NULL DEFAULT false,
    public_response_model TEXT,
    request_messages JSONB,
    request_params   JSONB,
    response_message JSONB,
    media_refs       JSONB,
    request_status   TEXT        NOT NULL,
    http_status      INT,
    error_code       TEXT,
    -- 仅管理员详情接口可查：未脱敏/未截断的真实底层错误（Go 错误链 + 上游原始报文全文）
    -- ai_usage_logs.error_message 对租户/终端用户自助用量页也可见，不能塞入这类内容。
    internal_error_detail TEXT,
    failed_step      VARCHAR(64),
    -- 一次请求内每次候选路由（上游账号/凭据）重试的明细，仅管理员可查；
    -- 只按 request_id 定位查看，不需要按内容搜索，JSONB 足够。
    attempts_detail  JSONB,
    PRIMARY KEY (id)
  );

  CREATE UNIQUE INDEX IF NOT EXISTS idx_arp_request_id ON ai_request_payloads (request_id);
  CREATE INDEX IF NOT EXISTS idx_arp_created_at ON ai_request_payloads (created_at);

  -- Durable audit inbox. The request path only commits this compact JSONB
  -- envelope; a worker later materializes it into ai_request_payloads.
  -- Rows are leased rather than deleted on claim, so a crashed worker can be
  -- recovered and retried without an external queue.
  CREATE TABLE IF NOT EXISTS ai_audit_inbox (
    id           BIGSERIAL PRIMARY KEY,
    request_id   TEXT NOT NULL UNIQUE,
    payload      JSONB NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending'
      CHECK (status IN ('pending', 'processing', 'dead')),
    attempts     INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at    TIMESTAMPTZ,
    locked_by    TEXT,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    dead_at      TIMESTAMPTZ
  );

  CREATE INDEX IF NOT EXISTS idx_ai_audit_inbox_ready
    ON ai_audit_inbox (available_at, created_at) WHERE status = 'pending';
  CREATE INDEX IF NOT EXISTS idx_ai_audit_inbox_status
    ON ai_audit_inbox (status, created_at);

  -- ============================================================================
  -- 归档存储过程：将 N 个月前的数据搬到归档表，然后从主表删除。
  -- 归档表命名格式：ai_request_payloads_archive_YYYY_MM
  -- 用法：SELECT archive_request_payloads(6);  -- 保留最近 6 个月
  -- ============================================================================
  CREATE OR REPLACE FUNCTION archive_request_payloads(retain_months INT DEFAULT 6)
  RETURNS TABLE(archived_table TEXT, rows_moved BIGINT) AS $$
  DECLARE
    cutoff        TIMESTAMPTZ;
    archive_name  TEXT;
    moved_count   BIGINT;
  BEGIN
    -- 算出归档分界点：保留 retain_months 个月的数据
    cutoff := date_trunc('month', NOW() - (retain_months || ' months')::INTERVAL);

    -- 如果没有需要归档的数据，直接返回
    IF NOT EXISTS (SELECT 1 FROM ai_request_payloads WHERE created_at < cutoff) THEN
      archived_table := NULL;
      rows_moved := 0;
      RETURN NEXT;
      RETURN;
    END IF;

    -- 逐月归档：找出所有早于 cutoff 的月份
    FOR archive_name IN
      SELECT DISTINCT 'ai_request_payloads_archive_' || to_char(date_trunc('month', created_at), 'YYYY_MM')
      FROM ai_request_payloads
      WHERE created_at < cutoff
      ORDER BY 1
    LOOP
      -- 提取月份用于过滤
      DECLARE
        month_start  TIMESTAMPTZ;
        month_end    TIMESTAMPTZ;
        month_label  TEXT;
      BEGIN
        -- 从归档表名解析月份，如 'ai_request_payloads_archive_2026_01' → '2026_01'
        month_label := substring(archive_name from '(\d{4}_\d{2})$');
        month_start := to_timestamp(replace(month_label, '_', '-'), 'YYYY-MM')::TIMESTAMPTZ;
        month_end   := month_start + INTERVAL '1 month';

        -- 创建归档表（如果不存在），只继承默认值，不继承索引
        EXECUTE format(
          'CREATE TABLE IF NOT EXISTS %I (LIKE ai_request_payloads INCLUDING DEFAULTS)',
          archive_name
        );

        -- 搬数据
        EXECUTE format(
          'INSERT INTO %I SELECT * FROM ai_request_payloads WHERE created_at >= %L AND created_at < %L',
          archive_name, month_start, month_end
        );

        GET DIAGNOSTICS moved_count = ROW_COUNT;

        -- 从主表删除已归档的数据
        DELETE FROM ai_request_payloads WHERE created_at >= month_start AND created_at < month_end;

        -- 返回本次归档结果
        archived_table := archive_name;
        rows_moved := moved_count;
        RETURN NEXT;
      END;
    END LOOP;
  END;
  $$ LANGUAGE plpgsql;

  -- ============================================================================
  -- AI Audit Blobs (content-addressed media store; sha256 dedup via ON CONFLICT)
  -- ============================================================================
  CREATE TABLE IF NOT EXISTS ai_audit_blobs (
    sha256       TEXT        PRIMARY KEY,
    content      BYTEA       NOT NULL,
    content_type TEXT        NOT NULL,
    size_bytes   INT         NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  CREATE INDEX IF NOT EXISTS idx_ai_audit_blobs_created_at
    ON ai_audit_blobs (created_at);

  -- ============================================================================
  -- Billing ledger
  -- Credit lease、请求准入和结算投递分别建模。Redis 仅可作为唤醒/缓存，
  -- 不参与任何资金状态机判断。
  -- ============================================================================

  CREATE TABLE IF NOT EXISTS ai_billing_windows (
    window_id               TEXT        PRIMARY KEY,
    owner_type              TEXT        NOT NULL CHECK (owner_type IN ('user', 'tenant')),
    tenant_id               TEXT        NOT NULL,
    user_id                 TEXT        NOT NULL DEFAULT '',
    want_tenant             BOOLEAN     NOT NULL,
    want_user               BOOLEAN     NOT NULL,
    lease_id                TEXT        UNIQUE,
    lease_version           BIGINT      NOT NULL DEFAULT 0 CHECK (lease_version >= 0),
    requested_tenant_micro  BIGINT      NOT NULL DEFAULT 0 CHECK (requested_tenant_micro >= 0),
    requested_user_micro    BIGINT      NOT NULL DEFAULT 0 CHECK (requested_user_micro >= 0),
    granted_tenant_micro    BIGINT      NOT NULL DEFAULT 0 CHECK (granted_tenant_micro >= 0),
    granted_user_micro      BIGINT      NOT NULL DEFAULT 0 CHECK (granted_user_micro >= 0),
    accrued_tenant_micro    BIGINT      NOT NULL DEFAULT 0 CHECK (accrued_tenant_micro >= 0),
    accrued_user_micro      BIGINT      NOT NULL DEFAULT 0 CHECK (accrued_user_micro >= 0),
    state                   TEXT        NOT NULL
      CHECK (state IN ('opening', 'active', 'draining', 'settlement_pending', 'settled', 'reconciling')),
    expires_at              TIMESTAMPTZ,
    grace_until             TIMESTAMPTZ,
    max_age_at              TIMESTAMPTZ NOT NULL,
    last_admitted_at        TIMESTAMPTZ,
    last_error_code         TEXT,
    last_error_detail       TEXT,
    opened_at               TIMESTAMPTZ,
    settled_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (want_tenant OR want_user),
    CHECK (want_user = false OR user_id <> ''),
    CHECK (
      (state = 'opening' AND lease_id IS NULL)
      OR
      (state <> 'opening' AND lease_id IS NOT NULL)
    )
  );

  CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_billing_windows_active_owner
    ON ai_billing_windows (owner_type, tenant_id, user_id)
    WHERE state IN ('opening', 'active');

  CREATE INDEX IF NOT EXISTS idx_ai_billing_windows_worker
    ON ai_billing_windows (state, updated_at);

  CREATE TABLE IF NOT EXISTS ai_billing_request_admissions (
    request_id         TEXT        PRIMARY KEY,
    window_id          TEXT        NOT NULL REFERENCES ai_billing_windows(window_id) ON DELETE RESTRICT,
    lease_id           TEXT        NOT NULL,
    status             TEXT        NOT NULL CHECK (status IN ('active', 'reconciling', 'completed')),
    request_expires_at TIMESTAMPTZ NOT NULL,
    actual_tenant_micro BIGINT     CHECK (actual_tenant_micro IS NULL OR actual_tenant_micro >= 0),
    actual_user_micro   BIGINT     CHECK (actual_user_micro IS NULL OR actual_user_micro >= 0),
    completion_source   TEXT       CHECK (completion_source IN ('runtime', 'manual')),
    completion_note     TEXT,
    completed_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
      (status IN ('active', 'reconciling')
        AND actual_tenant_micro IS NULL
        AND actual_user_micro IS NULL
        AND completion_source IS NULL
        AND completed_at IS NULL)
      OR
      (status = 'completed'
        AND actual_tenant_micro IS NOT NULL
        AND actual_user_micro IS NOT NULL
        AND completion_source IS NOT NULL
        AND completed_at IS NOT NULL)
    )
  );

  CREATE INDEX IF NOT EXISTS idx_ai_billing_admissions_window_active
    ON ai_billing_request_admissions (window_id, request_expires_at)
    WHERE status IN ('active', 'reconciling');

  CREATE TABLE IF NOT EXISTS ai_billing_settlement_batches (
    batch_id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    window_id                 TEXT        NOT NULL UNIQUE REFERENCES ai_billing_windows(window_id) ON DELETE RESTRICT,
    lease_id                  TEXT        NOT NULL,
    settlement_id             TEXT        NOT NULL UNIQUE,
    actual_tenant_micro       BIGINT      NOT NULL CHECK (actual_tenant_micro >= 0),
    actual_user_micro         BIGINT      NOT NULL CHECK (actual_user_micro >= 0),
    status                    TEXT        NOT NULL CHECK (status IN ('pending', 'delivered', 'reconciling')),
    tenant_deducted_micro     BIGINT      NOT NULL DEFAULT 0 CHECK (tenant_deducted_micro >= 0),
    user_deducted_micro       BIGINT      NOT NULL DEFAULT 0 CHECK (user_deducted_micro >= 0),
    tenant_debt_added_micro   BIGINT      NOT NULL DEFAULT 0 CHECK (tenant_debt_added_micro >= 0),
    user_debt_added_micro     BIGINT      NOT NULL DEFAULT 0 CHECK (user_debt_added_micro >= 0),
    attempt_count             INTEGER     NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_error_code           TEXT,
    last_error_detail         TEXT,
    delivered_at              TIMESTAMPTZ,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
      (status IN ('pending', 'reconciling') AND delivered_at IS NULL)
      OR
      (status = 'delivered'
        AND delivered_at IS NOT NULL
        AND tenant_deducted_micro + tenant_debt_added_micro = actual_tenant_micro
        AND user_deducted_micro + user_debt_added_micro = actual_user_micro)
    )
  );

  CREATE INDEX IF NOT EXISTS idx_ai_billing_batches_status
    ON ai_billing_settlement_batches (status, updated_at);

  CREATE TABLE IF NOT EXISTS ai_billing_settlement_outbox (
    outbox_id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id           UUID        NOT NULL UNIQUE REFERENCES ai_billing_settlement_batches(batch_id) ON DELETE RESTRICT,
    lease_id           TEXT        NOT NULL,
    settlement_id      TEXT        NOT NULL UNIQUE,
    payload            JSONB       NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    status             TEXT        NOT NULL CHECK (status IN ('pending', 'processing', 'delivered')),
    attempt_count      INTEGER     NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    available_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until       TIMESTAMPTZ,
    last_error_code    TEXT,
    last_error_detail  TEXT,
    delivered_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
      (status = 'pending' AND locked_until IS NULL AND delivered_at IS NULL)
      OR
      (status = 'processing' AND locked_until IS NOT NULL AND delivered_at IS NULL)
      OR
      (status = 'delivered' AND locked_until IS NULL AND delivered_at IS NOT NULL)
    )
  );

  CREATE INDEX IF NOT EXISTS idx_ai_billing_outbox_dispatch
    ON ai_billing_settlement_outbox (available_at, created_at)
    WHERE status IN ('pending', 'processing');

  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_billing_window
    ON ai_usage_logs (billing_window_id, billing_status)
    WHERE billing_window_id IS NOT NULL;

  CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_settlement_batch
    ON ai_usage_logs (settlement_batch_id)
    WHERE settlement_batch_id IS NOT NULL;

  -- ============================================================================
  -- AI 订阅制套餐（Subscription Plans）—— docs/ai-subscription-design.md
  --   二期重构：docs/ai-subscription-group-refactor.md
  -- 终端用户用 USD 余额购买「固定时长 + 三层加权额度」套餐；订阅期内 AI 用量优先从
  -- 套餐额度扣减，任一额度触顶回落按量，窗口重置后自动回订阅。
  -- 价格和额度单位统一为 micro-USD（零售基准价 × 命中分组套餐扣额倍率，用户倍率不参与）。
  -- 套餐必须绑定 ≥1 个分组（ai_sub_plan_groups），订阅覆盖期硬限制路由到套餐分组交集。
  -- ============================================================================

  -- 套餐表：租户上架给自己终端用户的订阅套餐
  CREATE TABLE IF NOT EXISTS ai_sub_plans (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             TEXT        NOT NULL,
    name                  TEXT        NOT NULL,
    description           TEXT        NOT NULL DEFAULT '',
    price_micro_usd       BIGINT      NOT NULL CHECK (price_micro_usd > 0),
    duration_days         INTEGER     NOT NULL CHECK (duration_days > 0),
    total_limit_micro     BIGINT      NOT NULL CHECK (total_limit_micro > 0),
    window_5h_limit_micro BIGINT      CHECK (window_5h_limit_micro IS NULL OR window_5h_limit_micro > 0),
    window_7d_limit_micro BIGINT      CHECK (window_7d_limit_micro IS NULL OR window_7d_limit_micro > 0),
    status                TEXT        NOT NULL DEFAULT 'draft',
    sort_order            INTEGER     NOT NULL DEFAULT 0,
    sale_limit            INTEGER     CHECK (sale_limit IS NULL OR sale_limit > 0),
    sold_count            INTEGER     NOT NULL DEFAULT 0 CHECK (sold_count >= 0),
    reserved_count        INTEGER     NOT NULL DEFAULT 0 CHECK (reserved_count >= 0),
    created_by            TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 7 天窗口限额只对 ≥7 天套餐有意义
    CHECK (duration_days >= 7 OR window_7d_limit_micro IS NULL),
    CONSTRAINT ai_sub_plans_window_5h_within_total_check
      CHECK (window_5h_limit_micro IS NULL OR window_5h_limit_micro <= total_limit_micro),
    CONSTRAINT ai_sub_plans_window_7d_within_total_check
      CHECK (window_7d_limit_micro IS NULL OR window_7d_limit_micro <= total_limit_micro),
    CONSTRAINT ai_sub_plans_inventory_within_limit_check
      CHECK (sale_limit IS NULL OR sold_count + reserved_count <= sale_limit),
    CONSTRAINT ai_sub_plans_name_unique UNIQUE (tenant_id, name)
  );

  CREATE INDEX IF NOT EXISTS idx_ai_sub_plans_tenant ON ai_sub_plans (tenant_id, status, sort_order);

  -- 套餐购买政策：累计限购 + 单一周期限购 + 提前续购规则，全部按用户/套餐维度生效。
  CREATE TABLE IF NOT EXISTS ai_sub_plan_purchase_policies (
    plan_id                    UUID        PRIMARY KEY,
    lifetime_max_purchases     INTEGER     CHECK (lifetime_max_purchases IS NULL OR lifetime_max_purchases > 0),
    period_type                TEXT        NOT NULL DEFAULT 'none',
    period_max_purchases       INTEGER     CHECK (period_max_purchases IS NULL OR period_max_purchases > 0),
    rolling_window_hours       INTEGER     CHECK (rolling_window_hours IS NULL OR rolling_window_hours BETWEEN 1 AND 876000),
    calendar_unit              TEXT,
    calendar_timezone          TEXT,
    allow_advance_purchase     BOOLEAN     NOT NULL DEFAULT true,
    version                    BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  -- 政策修订历史用于运营审计；当前规则仍以 policies 表为准。
  CREATE TABLE IF NOT EXISTS ai_sub_plan_purchase_policy_revisions (
    plan_id          UUID        NOT NULL,
    version          BIGINT      NOT NULL CHECK (version > 0),
    policy_snapshot  JSONB       NOT NULL CHECK (jsonb_typeof(policy_snapshot) = 'object'),
    changed_by       TEXT,
    changed_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (plan_id, version)
  );

  -- 购买订单表（一致性锚点）：created → deducting → paid | failed
  CREATE TABLE IF NOT EXISTS ai_sub_orders (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    order_no           TEXT        NOT NULL UNIQUE,      -- 'SUB' + 无横线短 uuid，计费幂等键 = 'ai-sub-' || order_no
    tenant_id          TEXT        NOT NULL,
    user_id            TEXT        NOT NULL,
    plan_id            UUID        NOT NULL,
    plan_name_snapshot TEXT        NOT NULL,
    price_micro_usd    BIGINT      NOT NULL CHECK (price_micro_usd > 0),  -- 快照：下单价
    -- 完整权益快照：下单成功后套餐编辑/下架不改变待补偿订单最终开通的权益
    duration_days_snapshot          INTEGER     NOT NULL CHECK (duration_days_snapshot > 0),
    total_limit_micro_snapshot      BIGINT      NOT NULL CHECK (total_limit_micro_snapshot > 0),
    window_5h_limit_micro_snapshot  BIGINT      CHECK (window_5h_limit_micro_snapshot IS NULL OR window_5h_limit_micro_snapshot > 0),
    window_7d_limit_micro_snapshot  BIGINT      CHECK (window_7d_limit_micro_snapshot IS NULL OR window_7d_limit_micro_snapshot > 0),
    group_quota_debit_multipliers_snapshot JSONB NOT NULL,
    purchase_policy_version         BIGINT      NOT NULL CHECK (purchase_policy_version > 0),
    purchase_policy_snapshot        JSONB       NOT NULL CHECK (jsonb_typeof(purchase_policy_snapshot) = 'object'),
    inventory_reserved BOOLEAN     NOT NULL DEFAULT false,
    status             TEXT        NOT NULL DEFAULT 'created'
                           CHECK (status IN ('created', 'deducting', 'paid', 'failed')),
    debit_reference       TEXT,
    debited_at            TIMESTAMPTZ,
    subscription_id    UUID,                             -- paid 后回填
    fail_reason        TEXT,
    paid_at            TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ai_sub_orders_duration_window_7d_check
      CHECK (duration_days_snapshot >= 7 OR window_7d_limit_micro_snapshot IS NULL),
    CONSTRAINT ai_sub_orders_window_5h_within_total_check
      CHECK (window_5h_limit_micro_snapshot IS NULL OR window_5h_limit_micro_snapshot <= total_limit_micro_snapshot),
    CONSTRAINT ai_sub_orders_window_7d_within_total_check
      CHECK (window_7d_limit_micro_snapshot IS NULL OR window_7d_limit_micro_snapshot <= total_limit_micro_snapshot),
    CONSTRAINT ai_sub_orders_group_snapshot_check
      CHECK (jsonb_typeof(group_quota_debit_multipliers_snapshot) = 'object'
        AND group_quota_debit_multipliers_snapshot <> '{}'::jsonb),
    CONSTRAINT ai_sub_orders_paid_at_check
      CHECK ((status = 'paid' AND paid_at IS NOT NULL) OR (status <> 'paid' AND paid_at IS NULL))
  );

  CREATE INDEX IF NOT EXISTS idx_ai_sub_orders_user      ON ai_sub_orders (tenant_id, user_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_ai_sub_orders_reconcile ON ai_sub_orders (updated_at)
    WHERE status IN ('created', 'deducting');
  CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_sub_orders_debit_reference
    ON ai_sub_orders (debit_reference)
    WHERE debit_reference IS NOT NULL;
  CREATE INDEX IF NOT EXISTS idx_ai_sub_orders_purchase_eligibility
    ON ai_sub_orders (tenant_id, user_id, plan_id, created_at DESC)
    WHERE status IN ('created', 'deducting', 'paid');

  -- 订阅实例表：三窗口均为「首次使用触发的固定窗口」，win*_start 为 NULL 表示尚未开窗
  CREATE TABLE IF NOT EXISTS ai_sub_subscriptions (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             TEXT        NOT NULL,
    user_id               TEXT        NOT NULL,
    plan_id               UUID        NOT NULL,
    order_id              UUID        NOT NULL UNIQUE,   -- ai_sub_orders.id，1 单 1 订阅
    -- 套餐快照：售出后改套餐不影响已售订阅
    plan_name_snapshot    TEXT        NOT NULL,
    duration_days         INTEGER     NOT NULL,
    total_limit_micro     BIGINT      NOT NULL,
    window_5h_limit_micro BIGINT,
    window_7d_limit_micro BIGINT,
    status                TEXT        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'active', 'expired', 'cancelled')),
    activated_at          TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ,
    total_used_micro      BIGINT      NOT NULL DEFAULT 0 CHECK (total_used_micro >= 0),
    win5h_start           TIMESTAMPTZ,
    win5h_used_micro      BIGINT      NOT NULL DEFAULT 0 CHECK (win5h_used_micro >= 0),
    win7d_start           TIMESTAMPTZ,
    win7d_used_micro      BIGINT      NOT NULL DEFAULT 0 CHECK (win7d_used_micro >= 0),
    -- 套餐分组扣额倍率快照 {group_id: quota_debit_multiplier}：售出后改套餐不影响已售订阅
    group_quota_debit_multipliers JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status IN ('active', 'expired') AND activated_at IS NOT NULL AND expires_at IS NOT NULL)
        OR (status IN ('pending', 'cancelled')))
  );

  -- 热路径点查：每个请求 gate 读一次
  CREATE INDEX IF NOT EXISTS idx_ai_sub_subscriptions_user_live
    ON ai_sub_subscriptions (tenant_id, user_id, created_at)
    WHERE status IN ('pending', 'active');
  -- janitor 扫描到期
  CREATE INDEX IF NOT EXISTS idx_ai_sub_subscriptions_expiry
    ON ai_sub_subscriptions (expires_at)
    WHERE status = 'active';
  CREATE INDEX IF NOT EXISTS idx_ai_sub_subscriptions_tenant
    ON ai_sub_subscriptions (tenant_id, created_at DESC);

  -- 套餐-分组绑定：套餐必须绑 ≥1 分组；quota_debit_multiplier 为套餐扣额倍率
  --（额度消耗 = 基准价 × 套餐扣额倍率），与按量售价倍率无关。
  CREATE TABLE IF NOT EXISTS ai_sub_plan_groups (
    plan_id      UUID    NOT NULL,
    group_id     UUID    NOT NULL,
    quota_debit_multiplier NUMERIC(10,4) NOT NULL CHECK (quota_debit_multiplier > 0),
    sort_order   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (plan_id, group_id)
  );

  -- 商城可售视图：套餐仅可引用同租户 active 分组；利润评估只作租户定价提示。
  CREATE OR REPLACE VIEW ai_sub_available_on_sale_plans AS
  SELECT p.*
  FROM ai_sub_plans p
  WHERE p.status = 'on_sale'
    AND EXISTS (SELECT 1 FROM ai_sub_plan_groups pg WHERE pg.plan_id = p.id)
    AND NOT EXISTS (
      SELECT 1
      FROM ai_sub_plan_groups pg
      LEFT JOIN ai_groups g ON g.id = pg.group_id
      WHERE pg.plan_id = p.id
        AND (g.id IS NULL OR g.status <> 'active' OR g.tenant_id <> p.tenant_id)
    );

-- Cross-domain read models. These views are the stable boundary for billing
-- and payment projections that need mutable identity names. Runtime and
-- billing roles receive SELECT on the views; the billing role does not need
-- broad read access to every control-plane table just to render a report.
CREATE VIEW billing_recharge_order_projection AS
SELECT r.order_id,
       r.order_type,
       r.paid_amount,
       r.credit_amount,
       r.status,
       COALESCE(r.note, '') AS note,
       COALESCE(r.user_id, '') AS user_id,
       COALESCE(eu.username, '') AS username,
       r.tenant_id,
       COALESCE(t.tenant_name, '') AS tenant_name,
       r.created_at
FROM bill_recharge_orders r
LEFT JOIN iam_accounts eu ON eu.user_id = r.user_id AND eu.user_type = 4
LEFT JOIN iam_tenants t ON t.tenant_id = r.tenant_id;

CREATE VIEW payment_order_party_projection AS
SELECT o.order_id,
       COALESCE(t.tenant_name, '') AS tenant_name,
       COALESCE(u.username, '') AS username
FROM pay_orders o
LEFT JOIN iam_tenants t ON t.tenant_id = o.tenant_id
LEFT JOIN iam_accounts u ON u.user_id = o.user_id AND u.user_type = 4;

CREATE VIEW payment_admin_recharge_order_projection AS
SELECT p.order_id,
       COALESCE(p.balance_order_id, '') AS balance_order_id,
       CASE p.scene WHEN 'user_topup' THEN 'online_user_topup' ELSE 'online_tenant_topup' END AS order_type,
       'online'::text AS method,
       CASE p.scene WHEN 'user_topup' THEN 'user' ELSE 'tenant' END AS target_type,
       p.tenant_id,
       COALESCE(t.tenant_name, '') AS tenant_name,
       COALESCE(p.user_id, '') AS user_id,
       COALESCE(u.username, '') AS username,
       p.payment_amount_minor,
       p.gross_amount_micro_usd,
       p.fee_amount_micro_usd,
       p.gift_amount_micro_usd,
       p.credited_amount_micro_usd,
       p.tenant_income_micro_usd,
       p.status AS payment_status,
       p.fulfillment_status,
       p.refund_status,
       p.out_trade_no,
       COALESCE(p.transaction_id, '') AS transaction_id,
       p.topup_mode,
       COALESCE(p.package_name, '') AS package_name,
       p.channel,
       COALESCE(r.note, '') AS note,
       COALESCE(p.fail_note, '') AS fail_note,
       p.created_at,
       p.paid_at,
       p.expires_at AS payment_expires_at,
       p.balance_expires_at,
       r.reversed_at,
       COALESCE(r.reversed_by, '') AS reversed_by,
       COALESCE(r.reversal_reason, '') AS reversal_reason
FROM pay_orders p
LEFT JOIN bill_recharge_orders r ON r.order_id = p.balance_order_id
LEFT JOIN iam_tenants t ON t.tenant_id = p.tenant_id
LEFT JOIN iam_accounts u ON u.user_id = p.user_id AND u.user_type = 4

UNION ALL

SELECT r.order_id,
       r.order_id AS balance_order_id,
       r.order_type,
       'manual'::text AS method,
       CASE WHEN r.user_id IS NULL THEN 'tenant' ELSE 'user' END AS target_type,
       r.tenant_id,
       COALESCE(t.tenant_name, '') AS tenant_name,
       COALESCE(r.user_id, '') AS user_id,
       COALESCE(u.username, '') AS username,
       r.paid_amount AS payment_amount_minor,
       r.credit_amount AS gross_amount_micro_usd,
       0::bigint AS fee_amount_micro_usd,
       0::bigint AS gift_amount_micro_usd,
       r.credit_amount AS credited_amount_micro_usd,
       0::bigint AS tenant_income_micro_usd,
       'not_required'::text AS payment_status,
       CASE
         WHEN r.status = 'active' THEN 'credited'
         WHEN r.lost_amount_micro > 0 THEN 'partially_reversed'
         ELSE 'reversed'
       END AS fulfillment_status,
       'not_applicable'::text AS refund_status,
       ''::text AS out_trade_no,
       ''::text AS transaction_id,
       ''::text AS topup_mode,
       ''::text AS package_name,
       'manual'::text AS channel,
       COALESCE(r.note, '') AS note,
       ''::text AS fail_note,
       r.created_at,
       NULL::timestamptz AS paid_at,
       NULL::timestamptz AS payment_expires_at,
       r.expires_at AS balance_expires_at,
       r.reversed_at,
       COALESCE(r.reversed_by, '') AS reversed_by,
       COALESCE(r.reversal_reason, '') AS reversal_reason
FROM bill_recharge_orders r
LEFT JOIN iam_tenants t ON t.tenant_id = r.tenant_id
LEFT JOIN iam_accounts u ON u.user_id = r.user_id AND u.user_type = 4
WHERE r.order_type IN ('platform_to_tenant', 'tenant_to_user');

-- Required system defaults
INSERT INTO sys_settings (key, value)
VALUES (
    'payment',
    '{
        "tenantCustomTopupFeeBp": 160,
        "tenantWithdrawFeeBp": 160,
        "tenantTopupPackages": [
            {"id": "p10", "name": "$10 体验包", "paymentAmountMicroUsd": 10000000, "giftAmountMicroUsd": 0, "enabled": true, "sortOrder": 10},
            {"id": "p20", "name": "$20 基础包", "paymentAmountMicroUsd": 20000000, "giftAmountMicroUsd": 0, "enabled": true, "sortOrder": 20},
            {"id": "p50", "name": "$50 常用包", "paymentAmountMicroUsd": 50000000, "giftAmountMicroUsd": 0, "enabled": true, "sortOrder": 30},
            {"id": "p100", "name": "$100 进阶包", "paymentAmountMicroUsd": 100000000, "giftAmountMicroUsd": 0, "enabled": true, "sortOrder": 40}
        ]
    }'::jsonb
)
ON CONFLICT (key) DO NOTHING;

INSERT INTO sys_settings (key, value)
VALUES (
    'data_cleanup',
    '{
        "enabled": true,
        "requestBodyDays": 30,
        "requestPayloadDays": 180,
        "notificationDays": 90,
        "moderationDays": 90,
        "riskEventDays": 365,
        "adminAuditDays": 365,
        "auditBlobDays": 180,
        "usageRollupDays": 730,
        "batchSize": 1000
    }'::jsonb
)
ON CONFLICT (key) DO NOTHING;

INSERT INTO pay_wechat_config (id) VALUES (1) ON CONFLICT DO NOTHING;

-- Schema contract checked by the application at startup. Insert this last so a
-- partially applied schema can never be reported as ready.
CREATE TABLE dai_schema_metadata (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    version INTEGER NOT NULL CHECK (version > 0),
    initialized_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO dai_schema_metadata (singleton, version) VALUES (TRUE, 20);

COMMIT;

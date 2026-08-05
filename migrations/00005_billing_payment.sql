-- +goose Up

-- Ledger amounts use microcredits (1 credit = 10,000 microcredits). Legacy
-- column names are retained because the application contract uses them.
CREATE TABLE bill_recharge_orders (
    id BIGSERIAL PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE,
    order_type TEXT NOT NULL CONSTRAINT bill_recharge_orders_order_type_check
        CHECK (order_type IN ('platform_to_tenant', 'tenant_to_user',
                              'online_user_topup', 'online_tenant_topup', 'cash_purchase')),
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    credit_amount BIGINT NOT NULL CHECK (credit_amount > 0),
    paid_amount BIGINT NOT NULL DEFAULT 0 CHECK (paid_amount >= 0),
    payment_ref TEXT,
    expires_at TIMESTAMPTZ,
    operator_id TEXT NOT NULL,
    note TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'reversed')),
    reversed_at TIMESTAMPTZ,
    reversed_by TEXT,
    reversal_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bill_recharge_orders_user_id_pairing_check CHECK (
        (order_type IN ('platform_to_tenant', 'online_tenant_topup', 'cash_purchase') AND user_id IS NULL)
        OR (order_type IN ('tenant_to_user', 'online_user_topup') AND user_id IS NOT NULL)
    )
);

CREATE INDEX idx_bill_recharge_orders_tenant ON bill_recharge_orders (order_type, tenant_id, created_at DESC);
CREATE INDEX idx_bill_recharge_orders_user ON bill_recharge_orders (user_id, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_bill_recharge_orders_status ON bill_recharge_orders (status, created_at DESC);

CREATE TABLE bill_credit_packages (
    id BIGSERIAL PRIMARY KEY,
    package_id TEXT NOT NULL UNIQUE,
    package_type TEXT NOT NULL CHECK (package_type IN ('tenant', 'user')),
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    total_credits BIGINT NOT NULL CHECK (total_credits > 0),
    remaining_credits BIGINT NOT NULL CHECK (remaining_credits >= 0 AND remaining_credits <= total_credits),
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'available' CHECK (status IN ('available', 'expired', 'depleted', 'revoked')),
    source TEXT NOT NULL CONSTRAINT bill_credit_packages_source_check
        CHECK (source IN ('ADMIN_RECHARGE', 'TENANT_RECHARGE', 'REFUND', 'ONLINE_TOPUP', 'CASH_PURCHASE')),
    recharge_order_id TEXT,
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (package_type = 'tenant' AND user_id IS NULL)
        OR (package_type = 'user' AND user_id IS NOT NULL)
    )
);

CREATE INDEX idx_bill_credit_packages_tenant_fifo
    ON bill_credit_packages (package_type, tenant_id, status, expires_at, created_at);
CREATE INDEX idx_bill_credit_packages_user_fifo
    ON bill_credit_packages (package_type, user_id, status, expires_at, created_at)
    WHERE user_id IS NOT NULL;
CREATE INDEX idx_bill_credit_packages_order
    ON bill_credit_packages (recharge_order_id)
    WHERE recharge_order_id IS NOT NULL;

CREATE TABLE bill_events (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    idempotency_key TEXT UNIQUE,
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    client_id TEXT,
    description TEXT,
    event_type TEXT NOT NULL DEFAULT 'charge' CHECK (event_type IN ('charge', 'refund')),
    refund_of TEXT,
    tenant_credits BIGINT CHECK (tenant_credits IS NULL OR tenant_credits >= 0),
    user_credits BIGINT CHECK (user_credits IS NULL OR user_credits >= 0),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'cancelled', 'released', 'refunded')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    terminal_note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_bill_events_idempotency ON bill_events (idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_bill_events_tenant_time ON bill_events (tenant_id, created_at DESC);
CREATE INDEX idx_bill_events_user_time ON bill_events (user_id, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_bill_events_pending ON bill_events (status, created_at) WHERE status = 'pending';
CREATE INDEX idx_bill_events_type_tenant ON bill_events (event_type, tenant_id, created_at DESC);
CREATE INDEX idx_bill_events_refund_of ON bill_events (refund_of) WHERE refund_of IS NOT NULL;

CREATE TABLE bill_overdraft_adjustments (
    id BIGSERIAL PRIMARY KEY,
    account_type INTEGER NOT NULL CHECK (account_type IN (1, 2)),
    account_id TEXT NOT NULL,
    from_limit BIGINT NOT NULL CHECK (from_limit >= 0),
    to_limit BIGINT NOT NULL CHECK (to_limit >= 0),
    operator_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bill_overdraft_adjustments_account
    ON bill_overdraft_adjustments (account_type, account_id, created_at DESC);

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
    amount BIGINT NOT NULL CHECK (amount > 0),
    exchange_rate BIGINT NOT NULL CHECK (exchange_rate > 0),
    gross_credit_amount BIGINT NOT NULL DEFAULT 0 CHECK (gross_credit_amount >= 0),
    fee_credit_amount BIGINT NOT NULL DEFAULT 0 CHECK (fee_credit_amount >= 0),
    credit_amount BIGINT NOT NULL CHECK (credit_amount > 0),
    fee_rate_bp INTEGER NOT NULL DEFAULT 0,
    fee_amount BIGINT NOT NULL DEFAULT 0,
    net_amount BIGINT NOT NULL DEFAULT 0,
    channel TEXT NOT NULL DEFAULT 'wechat_native',
    code_url TEXT,
    transaction_id TEXT,
    status TEXT NOT NULL DEFAULT 'created' CHECK (status IN ('created', 'paying', 'paid', 'closed', 'expired')),
    paid_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    credit_order_id TEXT,
    fail_note TEXT,
    notify_raw JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (scene = 'user_topup' AND user_id IS NOT NULL)
        OR (scene = 'tenant_topup' AND user_id IS NULL)
    )
);

CREATE UNIQUE INDEX uq_pay_orders_txn ON pay_orders (transaction_id) WHERE transaction_id IS NOT NULL;
CREATE INDEX idx_pay_orders_sweep ON pay_orders (status, expires_at) WHERE status IN ('created', 'paying');
CREATE INDEX idx_pay_orders_tenant ON pay_orders (tenant_id, created_at DESC);
CREATE INDEX idx_pay_orders_user ON pay_orders (user_id, created_at DESC) WHERE user_id IS NOT NULL;

CREATE TABLE pay_cash_accounts (
    tenant_id TEXT PRIMARY KEY,
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    frozen BIGINT NOT NULL DEFAULT 0 CHECK (frozen >= 0 AND frozen <= balance),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE pay_cash_ledger (
    id BIGSERIAL PRIMARY KEY,
    txn_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    txn_type TEXT NOT NULL CHECK (txn_type IN ('topup_income', 'buy_credits', 'withdraw', 'adjust')),
    amount BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
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
    amount BIGINT NOT NULL CHECK (amount > 0),
    fee_amount BIGINT NOT NULL DEFAULT 0 CHECK (fee_amount >= 0),
    payout_amount BIGINT NOT NULL DEFAULT 0 CHECK (payout_amount >= 0),
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

-- +goose Down
DROP TABLE IF EXISTS pay_wechat_config;
DROP TABLE IF EXISTS pay_tenant_settings;
DROP TABLE IF EXISTS sys_settings;
DROP TABLE IF EXISTS pay_withdrawals;
DROP TABLE IF EXISTS pay_cash_ledger;
DROP TABLE IF EXISTS pay_cash_accounts;
DROP TABLE IF EXISTS pay_orders;
DROP TABLE IF EXISTS bill_overdraft_adjustments;
DROP TABLE IF EXISTS bill_events;
DROP TABLE IF EXISTS bill_credit_packages;
DROP TABLE IF EXISTS bill_recharge_orders;

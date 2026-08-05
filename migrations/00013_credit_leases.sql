-- +goose Up

-- Credit leases are mutable escrow records. They are deliberately separate
-- from bill_events, which remains the immutable financial-event journal.
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
    settled_event_id TEXT,
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
            AND settled_event_id IS NULL
            AND settled_at IS NULL)
        OR
        (settlement_state = 'settled'
            AND settlement_id IS NOT NULL
            AND actual_tenant_micro IS NOT NULL
            AND actual_user_micro IS NOT NULL
            AND tenant_deducted_micro + tenant_debt_added_micro = actual_tenant_micro
            AND user_deducted_micro + user_debt_added_micro = actual_user_micro
            AND (
                (actual_tenant_micro > 0 OR actual_user_micro > 0)
                = (settled_event_id IS NOT NULL)
            )
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

-- +goose Down

DROP TABLE IF EXISTS ledger_credit_leases;

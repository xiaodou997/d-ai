-- Forward update for Billing Ledger V3. Canonical definitions live in init.sql.
-- Stop AI Service instances before applying this script.

ALTER TABLE ai_usage_logs
  ADD COLUMN IF NOT EXISTS billing_window_id TEXT,
  ADD COLUMN IF NOT EXISTS settlement_batch_id UUID;

CREATE TABLE IF NOT EXISTS ai_billing_windows (
  window_id TEXT PRIMARY KEY,
  owner_type TEXT NOT NULL CHECK (owner_type IN ('user', 'tenant')),
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  want_tenant BOOLEAN NOT NULL,
  want_user BOOLEAN NOT NULL,
  lease_id TEXT UNIQUE,
  lease_version BIGINT NOT NULL DEFAULT 0 CHECK (lease_version >= 0),
  requested_tenant_micro BIGINT NOT NULL DEFAULT 0 CHECK (requested_tenant_micro >= 0),
  requested_user_micro BIGINT NOT NULL DEFAULT 0 CHECK (requested_user_micro >= 0),
  granted_tenant_micro BIGINT NOT NULL DEFAULT 0 CHECK (granted_tenant_micro >= 0),
  granted_user_micro BIGINT NOT NULL DEFAULT 0 CHECK (granted_user_micro >= 0),
  accrued_tenant_micro BIGINT NOT NULL DEFAULT 0 CHECK (accrued_tenant_micro >= 0),
  accrued_user_micro BIGINT NOT NULL DEFAULT 0 CHECK (accrued_user_micro >= 0),
  state TEXT NOT NULL CHECK (state IN ('opening', 'active', 'draining', 'settlement_pending', 'settled', 'reconciling')),
  expires_at TIMESTAMPTZ,
  grace_until TIMESTAMPTZ,
  max_age_at TIMESTAMPTZ NOT NULL,
  last_admitted_at TIMESTAMPTZ,
  last_error_code TEXT,
  last_error_detail TEXT,
  opened_at TIMESTAMPTZ,
  settled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
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
  request_id TEXT PRIMARY KEY,
  window_id TEXT NOT NULL REFERENCES ai_billing_windows(window_id) ON DELETE RESTRICT,
  lease_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'reconciling', 'completed')),
  request_expires_at TIMESTAMPTZ NOT NULL,
  actual_tenant_micro BIGINT CHECK (actual_tenant_micro IS NULL OR actual_tenant_micro >= 0),
  actual_user_micro BIGINT CHECK (actual_user_micro IS NULL OR actual_user_micro >= 0),
  completion_source TEXT CHECK (completion_source IN ('runtime', 'manual')),
  completion_note TEXT,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (status IN ('active','reconciling')
      AND actual_tenant_micro IS NULL
      AND actual_user_micro IS NULL
      AND completion_source IS NULL
      AND completed_at IS NULL)
    OR
    (status='completed'
      AND actual_tenant_micro IS NOT NULL
      AND actual_user_micro IS NOT NULL
      AND completion_source IS NOT NULL
      AND completed_at IS NOT NULL)
  )
);
CREATE INDEX IF NOT EXISTS idx_ai_billing_admissions_window_active
  ON ai_billing_request_admissions (window_id, request_expires_at)
  WHERE status IN ('active','reconciling');

CREATE TABLE IF NOT EXISTS ai_billing_settlement_batches (
  batch_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  window_id TEXT NOT NULL UNIQUE REFERENCES ai_billing_windows(window_id) ON DELETE RESTRICT,
  lease_id TEXT NOT NULL,
  settlement_id TEXT NOT NULL UNIQUE,
  actual_tenant_micro BIGINT NOT NULL CHECK (actual_tenant_micro >= 0),
  actual_user_micro BIGINT NOT NULL CHECK (actual_user_micro >= 0),
  status TEXT NOT NULL CHECK (status IN ('pending', 'delivered', 'reconciling')),
  urm_event_id TEXT,
  tenant_deducted_micro BIGINT NOT NULL DEFAULT 0 CHECK (tenant_deducted_micro >= 0),
  user_deducted_micro BIGINT NOT NULL DEFAULT 0 CHECK (user_deducted_micro >= 0),
  tenant_debt_added_micro BIGINT NOT NULL DEFAULT 0 CHECK (tenant_debt_added_micro >= 0),
  user_debt_added_micro BIGINT NOT NULL DEFAULT 0 CHECK (user_debt_added_micro >= 0),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  last_error_code TEXT,
  last_error_detail TEXT,
  delivered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (status IN ('pending', 'reconciling') AND delivered_at IS NULL)
    OR
    (status='delivered'
      AND delivered_at IS NOT NULL
      AND tenant_deducted_micro + tenant_debt_added_micro = actual_tenant_micro
      AND user_deducted_micro + user_debt_added_micro = actual_user_micro)
  )
);
CREATE INDEX IF NOT EXISTS idx_ai_billing_batches_status
  ON ai_billing_settlement_batches (status, updated_at);

CREATE TABLE IF NOT EXISTS ai_billing_settlement_outbox (
  outbox_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  batch_id UUID NOT NULL UNIQUE REFERENCES ai_billing_settlement_batches(batch_id) ON DELETE RESTRICT,
  lease_id TEXT NOT NULL,
  settlement_id TEXT NOT NULL UNIQUE,
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload)='object'),
  status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'delivered')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  last_error_code TEXT,
  last_error_detail TEXT,
  delivered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (status='pending' AND locked_until IS NULL AND delivered_at IS NULL)
    OR
    (status='processing' AND locked_until IS NOT NULL AND delivered_at IS NULL)
    OR
    (status='delivered' AND locked_until IS NULL AND delivered_at IS NOT NULL)
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

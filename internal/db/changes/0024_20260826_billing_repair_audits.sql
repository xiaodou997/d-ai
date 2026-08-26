-- from_version: 23
-- to_version: 24
-- created_at: 2026-08-26
-- description: add immutable billing repair audits and parked outbox requeue

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 23
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 23';
    END IF;
END
$$;

CREATE TABLE bill_repair_audits (
    id BIGSERIAL PRIMARY KEY,
    repair_id TEXT NOT NULL UNIQUE CHECK (btrim(repair_id) <> ''),
    action TEXT NOT NULL CHECK (action IN (
        'usage_refund', 'recharge_reversal', 'payment_refund', 'outbox_requeue'
    )),
    idempotency_key TEXT NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> ''),
    target_type TEXT NOT NULL CHECK (btrim(target_type) <> ''),
    target_id TEXT NOT NULL CHECK (btrim(target_id) <> ''),
    operator_id TEXT NOT NULL CHECK (btrim(operator_id) <> ''),
    reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
    before_state JSONB NOT NULL CHECK (jsonb_typeof(before_state) = 'object'),
    after_state JSONB NOT NULL CHECK (jsonb_typeof(after_state) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bill_repair_audits_target
    ON bill_repair_audits (target_type, target_id, created_at DESC);

CREATE FUNCTION bill_repair_audits_immutable() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'bill_repair_audits is immutable' USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_bill_repair_audits_immutable
    BEFORE UPDATE OR DELETE ON bill_repair_audits
    FOR EACH ROW EXECUTE FUNCTION bill_repair_audits_immutable();

CREATE FUNCTION bill_requeue_parked_outbox(
    p_request_id TEXT,
    p_repair_id TEXT,
    p_idempotency_key TEXT,
    p_operator_id TEXT,
    p_reason TEXT
) RETURNS TEXT AS $$
  DECLARE
    existing_repair_id TEXT;
    existing_action TEXT;
    existing_target_id TEXT;
    outbox_status TEXT;
    outbox_attempts INTEGER;
    outbox_tenant_id TEXT;
    outbox_user_id TEXT;
    outbox_tenant_micro BIGINT;
    outbox_user_micro BIGINT;
    outbox_last_error TEXT;
    outbox_created_at TIMESTAMPTZ;
    usage_status TEXT;
    usage_tenant_payable BIGINT;
    usage_user_charged BIGINT;
    usage_settlement_error TEXT;
    before_state JSONB;
    after_state JSONB;
    updated_rows INTEGER;
  BEGIN
    IF btrim(COALESCE(p_request_id, '')) = ''
       OR btrim(COALESCE(p_repair_id, '')) = ''
       OR btrim(COALESCE(p_idempotency_key, '')) = ''
       OR btrim(COALESCE(p_operator_id, '')) = ''
       OR btrim(COALESCE(p_reason, '')) = '' THEN
      RAISE EXCEPTION 'parked outbox requeue requires request, repair, idempotency, operator and reason';
    END IF;

    SELECT repair_id, action, target_id
    INTO existing_repair_id, existing_action, existing_target_id
    FROM bill_repair_audits
    WHERE idempotency_key = p_idempotency_key;
    IF FOUND THEN
      IF existing_action <> 'outbox_requeue' OR existing_target_id <> p_request_id THEN
        RAISE EXCEPTION 'idempotency key % belongs to another repair', p_idempotency_key;
      END IF;
      RETURN existing_repair_id;
    END IF;

    SELECT o.status, o.attempts, o.tenant_id, COALESCE(o.user_id, ''),
           o.tenant_micro, o.user_micro, o.last_error, o.created_at,
           u.billing_status, u.tenant_payable, u.user_charged, u.settlement_error
    INTO outbox_status, outbox_attempts, outbox_tenant_id, outbox_user_id,
         outbox_tenant_micro, outbox_user_micro, outbox_last_error, outbox_created_at,
         usage_status, usage_tenant_payable, usage_user_charged, usage_settlement_error
    FROM bill_charge_outbox o
    JOIN ai_usage_logs u ON u.request_id = o.request_id
    WHERE o.request_id = p_request_id
    FOR UPDATE OF o, u;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'parked outbox request % not found', p_request_id;
    END IF;

    -- A concurrent first attempt may have committed the audit while this
    -- transaction waited for the outbox row lock; make the retry idempotent.
    SELECT repair_id, action, target_id
    INTO existing_repair_id, existing_action, existing_target_id
    FROM bill_repair_audits
    WHERE idempotency_key = p_idempotency_key;
    IF FOUND THEN
      IF existing_action <> 'outbox_requeue' OR existing_target_id <> p_request_id THEN
        RAISE EXCEPTION 'idempotency key % belongs to another repair', p_idempotency_key;
      END IF;
      RETURN existing_repair_id;
    END IF;

    IF outbox_status <> 'failed' OR outbox_attempts < 10 OR usage_status <> 'failed' THEN
      RAISE EXCEPTION 'outbox request % is not a parked failed pair', p_request_id;
    END IF;
    IF outbox_tenant_micro < 0 OR outbox_user_micro < 0
       OR (outbox_tenant_micro = 0 AND outbox_user_micro = 0)
       OR (outbox_user_micro > 0 AND outbox_user_id = '')
       OR outbox_tenant_micro <> COALESCE(usage_tenant_payable, 0)
       OR outbox_user_micro <> COALESCE(usage_user_charged, 0) THEN
      RAISE EXCEPTION 'outbox request % failed linkage or amount validation', p_request_id;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM bill_accounts WHERE account_id = outbox_tenant_id)
       OR (outbox_user_micro > 0 AND NOT EXISTS (
           SELECT 1 FROM bill_accounts WHERE account_id = outbox_user_id
       )) THEN
      RAISE EXCEPTION 'outbox request % still references a missing billing account', p_request_id;
    END IF;

    before_state := jsonb_build_object(
      'outbox', jsonb_build_object(
        'status', outbox_status, 'attempts', outbox_attempts,
        'tenant_id', outbox_tenant_id, 'user_id', NULLIF(outbox_user_id, ''),
        'tenant_micro', outbox_tenant_micro, 'user_micro', outbox_user_micro,
        'last_error', outbox_last_error, 'created_at', outbox_created_at
      ),
      'usage', jsonb_build_object(
        'billing_status', usage_status, 'tenant_payable', usage_tenant_payable,
        'user_charged', usage_user_charged, 'settlement_error', usage_settlement_error
      )
    );

    UPDATE bill_charge_outbox
    SET status = 'pending', attempts = 0,
        last_error = LEFT('manual requeue: ' || p_reason, 1000),
        settled_at = NULL
    WHERE request_id = p_request_id;
    GET DIAGNOSTICS updated_rows = ROW_COUNT;
    IF updated_rows <> 1 THEN
      RAISE EXCEPTION 'parked outbox request % changed during requeue', p_request_id;
    END IF;

    UPDATE ai_usage_logs
    SET billing_status = 'pending', settlement_error = NULL, settled_at = NULL
    WHERE request_id = p_request_id AND billing_status = 'failed';
    GET DIAGNOSTICS updated_rows = ROW_COUNT;
    IF updated_rows <> 1 THEN
      RAISE EXCEPTION 'usage row for parked outbox request % changed during requeue', p_request_id;
    END IF;

    after_state := jsonb_build_object(
      'outbox', jsonb_build_object('status', 'pending', 'attempts', 0),
      'usage', jsonb_build_object('billing_status', 'pending', 'settlement_error', NULL)
    );
    INSERT INTO bill_repair_audits
      (repair_id, action, idempotency_key, target_type, target_id,
       operator_id, reason, before_state, after_state)
    VALUES
      (p_repair_id, 'outbox_requeue', p_idempotency_key, 'bill_charge_outbox', p_request_id,
       p_operator_id, p_reason, before_state, after_state);
    RETURN p_repair_id;
  END;
  $$ LANGUAGE plpgsql;

UPDATE dai_schema_metadata
SET version = 24,
    updated_at = now()
WHERE singleton = TRUE AND version = 23;

COMMIT;

-- from_version: 19
-- to_version: 20
-- created_at: 2026-08-26
-- description: add billing and payment cross-domain read models

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 19
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 19';
    END IF;
END
$$;

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

UPDATE dai_schema_metadata
SET version = 20,
    updated_at = now()
WHERE singleton = TRUE AND version = 19;

COMMIT;

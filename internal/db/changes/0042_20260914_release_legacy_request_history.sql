-- from_version: 41
-- to_version: 42
-- created_at: 2026-09-14
-- description: offline, destructive retirement of superseded request storage
-- Run only after record-cutover backup/verify; application writers must be stopped.
BEGIN;
SELECT pg_advisory_xact_lock(82624001);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM dai_schema_metadata WHERE singleton AND version=41) THEN RAISE EXCEPTION 'expected schema version 41'; END IF;
 IF EXISTS(SELECT 1 FROM bill_record_control WHERE accepting) OR EXISTS(SELECT 1 FROM bill_settlements) OR EXISTS(SELECT 1 FROM bill_journal) THEN RAISE EXCEPTION 'offline cutover must precede new financial activity'; END IF;
 IF EXISTS(SELECT 1 FROM bill_charge_outbox WHERE status<>'done')
 OR EXISTS(SELECT 1 FROM ai_billing_settlement_outbox WHERE status<>'delivered')
 OR EXISTS(SELECT 1 FROM ai_billing_settlement_batches WHERE status<>'delivered')
 OR EXISTS(SELECT 1 FROM ai_billing_request_admissions WHERE status<>'completed')
 OR EXISTS(SELECT 1 FROM ai_billing_windows WHERE state<>'settled')
 OR EXISTS(SELECT 1 FROM ledger_credit_leases WHERE escrow_state<>'released' OR settlement_state<>'settled')
 THEN RAISE EXCEPTION 'legacy settlements remain'; END IF;
 IF EXISTS(SELECT 1 FROM ai_request_keys WHERE sealed_at IS NULL)
 OR EXISTS(SELECT 1 FROM ai_async_tasks WHERE status IN ('pending','running'))
 OR EXISTS(SELECT 1 FROM pay_orders WHERE status IN ('created','paying') OR status='paid' AND fulfillment_status='pending')
 OR EXISTS(SELECT 1 FROM ai_sub_orders WHERE status IN ('created','deducting'))
 OR EXISTS(SELECT 1 FROM pay_refunds WHERE status IN ('created','processing'))
 THEN RAISE EXCEPTION 'active request or payment work remains'; END IF;
END $$;
DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename ~ '^ai_request_payloads_archive_[0-9]{4}_[0-9]{2}$' LOOP
  EXECUTE format('DROP TABLE %I RESTRICT',r.tablename);
 END LOOP;
END $$;

DROP FUNCTION archive_request_payloads(integer);
DROP TABLE ai_billing_settlement_outbox RESTRICT;
DROP TABLE ai_billing_settlement_batches RESTRICT;
DROP TABLE ai_billing_request_admissions RESTRICT;
DROP TABLE ai_billing_windows RESTRICT;
DROP TABLE bill_charge_outbox RESTRICT;
DROP TABLE ledger_credit_leases RESTRICT;
DROP TABLE ai_audit_inbox RESTRICT;
DROP TABLE ai_request_payloads RESTRICT;
DROP TABLE ai_audit_blobs RESTRICT;
DROP TABLE ai_usage_rollups_hourly RESTRICT;
DROP TABLE ai_usage_logs RESTRICT;

UPDATE dai_schema_metadata SET version=42,updated_at=now() WHERE singleton AND version=41;
COMMIT;

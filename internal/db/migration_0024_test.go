package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0024AddsImmutableRepairAuditAndParkedRequeue(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 23 WHERE singleton = TRUE;
		DROP TRIGGER trg_bill_repair_audits_immutable ON bill_repair_audits;
		DROP FUNCTION bill_requeue_parked_outbox(text, text, text, text, text);
		DROP FUNCTION bill_repair_audits_immutable();
		DROP TABLE bill_repair_audits;
	`); err != nil {
		t.Fatalf("prepare schema 23 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0024_20260826_billing_repair_audits.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0024: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 24 {
		t.Fatalf("migration version = %d, want 24", version)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('repair-audit-tenant', 'Repair Audit Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('repair-audit-user', 'repair-audit-tenant', 'repair-audit-user', 'hash', 4, 'active');
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
			 model_code, billable_unit_type, tenant_payable, user_payable, user_charged,
			 billing_status, request_status, client_protocol, billing_source, settlement_error)
		VALUES ('REPAIR_OUTBOX_USAGE', 'user', 'jwt', 'repair-test', 'repair-audit-tenant', 'repair-audit-user',
			 'repair-model', 'token', 50, 25, 25, 'failed', 'success', 'openai_chat', 'payg', 'missing account');
		INSERT INTO bill_charge_outbox
			(request_id, tenant_id, user_id, tenant_micro, user_micro, status, attempts, last_error)
		VALUES ('REPAIR_OUTBOX_USAGE', 'repair-audit-tenant', 'repair-audit-user', 50, 25, 'failed', 10, 'missing account');
	`); err != nil {
		t.Fatalf("seed repair audit fixture: %v", err)
	}

	var repairID string
	if err := pool.QueryRow(ctx, `
		SELECT bill_requeue_parked_outbox(
			'REPAIR_OUTBOX_USAGE', 'REPAIR_TEST_001', 'outbox-requeue:REPAIR_OUTBOX_USAGE:INC-1',
			'operator-1', 'restored billing account')
	`).Scan(&repairID); err != nil {
		t.Fatalf("requeue parked outbox: %v", err)
	}
	if repairID != "REPAIR_TEST_001" {
		t.Fatalf("requeue repair id = %q", repairID)
	}

	var outboxStatus, usageStatus string
	var attempts int
	if err := pool.QueryRow(ctx, `
		SELECT o.status, o.attempts, u.billing_status
		FROM bill_charge_outbox o JOIN ai_usage_logs u ON u.request_id = o.request_id
		WHERE o.request_id = 'REPAIR_OUTBOX_USAGE'
	`).Scan(&outboxStatus, &attempts, &usageStatus); err != nil {
		t.Fatal(err)
	}
	if outboxStatus != "pending" || attempts != 0 || usageStatus != "pending" {
		t.Fatalf("requeued state = outbox:%s attempts:%d usage:%s", outboxStatus, attempts, usageStatus)
	}

	var replayID string
	if err := pool.QueryRow(ctx, `
		SELECT bill_requeue_parked_outbox(
			'REPAIR_OUTBOX_USAGE', 'DIFFERENT_REPAIR_ID', 'outbox-requeue:REPAIR_OUTBOX_USAGE:INC-1',
			'operator-1', 'retry same incident')
	`).Scan(&replayID); err != nil {
		t.Fatalf("idempotent requeue replay: %v", err)
	}
	if replayID != repairID {
		t.Fatalf("idempotent replay id = %q, want %q", replayID, repairID)
	}

	var action, key, target string
	if err := pool.QueryRow(ctx, `
		SELECT action, idempotency_key, target_id
		FROM bill_repair_audits WHERE repair_id = 'REPAIR_TEST_001'
	`).Scan(&action, &key, &target); err != nil {
		t.Fatal(err)
	}
	if action != "outbox_requeue" || key != "outbox-requeue:REPAIR_OUTBOX_USAGE:INC-1" || target != "REPAIR_OUTBOX_USAGE" {
		t.Fatalf("repair audit = action:%s key:%s target:%s", action, key, target)
	}

	if _, err := pool.Exec(ctx, `UPDATE bill_repair_audits SET reason = 'tampered' WHERE repair_id = 'REPAIR_TEST_001'`); err == nil {
		t.Fatal("immutable repair audit accepted UPDATE")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM bill_repair_audits WHERE repair_id = 'REPAIR_TEST_001'`); err == nil {
		t.Fatal("immutable repair audit accepted DELETE")
	}

	if _, err := pool.Exec(ctx, `
		UPDATE bill_charge_outbox
		SET status = 'failed', attempts = 10, tenant_id = 'missing-repair-tenant'
		WHERE request_id = 'REPAIR_OUTBOX_USAGE';
		UPDATE ai_usage_logs SET billing_status = 'failed' WHERE request_id = 'REPAIR_OUTBOX_USAGE';
	`); err != nil {
		t.Fatalf("prepare second requeue: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		SELECT bill_requeue_parked_outbox(
			'REPAIR_OUTBOX_USAGE', 'REPAIR_TEST_002', 'outbox-requeue:REPAIR_OUTBOX_USAGE:INC-2',
			'operator-1', 'account still invalid')
	`); err == nil {
		t.Fatal("requeue accepted missing account state")
	}
}

package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0008FoldsBillEventsIntoUsageAndDropsJournal(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_usage_logs
			DROP CONSTRAINT ai_usage_logs_billing_status_check,
			DROP CONSTRAINT ai_usage_logs_refund_status_check,
			DROP COLUMN settlement_error,
			DROP COLUMN refund_status,
			DROP COLUMN refund_reason,
			DROP COLUMN refund_operator_id,
			DROP COLUMN refunded_at;
		ALTER TABLE ai_usage_logs
			ADD COLUMN billing_event_id TEXT,
			ADD COLUMN settled_event_id TEXT;
		CREATE INDEX idx_ai_usage_logs_billing_event ON ai_usage_logs (billing_event_id);

		ALTER TABLE ai_sub_orders
			DROP COLUMN debit_reference,
			DROP COLUMN debited_at,
			ADD COLUMN billing_event_id TEXT;
		DROP INDEX IF EXISTS uq_ai_sub_orders_debit_reference;

		ALTER TABLE ledger_credit_leases ADD COLUMN settled_event_id TEXT;
		ALTER TABLE ai_async_tasks ADD COLUMN billing_event_id TEXT;
		ALTER TABLE ai_billing_settlement_batches ADD COLUMN billing_event_id TEXT;

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
		UPDATE dai_schema_metadata SET version = 7 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 7 fixture: %v", err)
	}

	finished := time.Date(2026, time.August, 18, 8, 0, 0, 0, time.UTC)
	seedUsageFor0008(t, ctx, pool, "req-0008-settled", "evt-0008-settled", "pending_settle")
	seedUsageFor0008(t, ctx, pool, "req-0008-refunded", "evt-0008-refunded", "pending_settle")
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_events
			(event_id, tenant_id, user_id, event_type, tenant_credits, user_credits, status, created_at, finished_at)
		VALUES
			('evt-0008-settled', 'tenant-0008', 'user-0008', 'charge', 11, 20, 'succeeded', $1, $1),
			('evt-0008-refunded', 'tenant-0008', 'user-0008', 'charge', 12, 20, 'refunded', $1, $1),
			('evt-0008-orphan', 'tenant-0008', 'user-0008', 'charge', 99, 99, 'succeeded', $1, $1)
	`, finished); err != nil {
		t.Fatalf("seed bill events: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bill_events
		SET terminal_note = 'refund requested by operator'
		WHERE event_id = 'evt-0008-refunded'
	`); err != nil {
		t.Fatalf("seed refund note: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_sub_orders (
			order_no, tenant_id, user_id, plan_id, plan_name_snapshot, price_micro_usd,
			duration_days_snapshot, total_limit_micro_snapshot,
			group_quota_debit_multipliers_snapshot, purchase_policy_version,
			purchase_policy_snapshot, status, billing_event_id, paid_at
		) VALUES (
			'ORD-0008', 'tenant-0008', 'user-0008', gen_random_uuid(), 'legacy plan', 100,
			30, 1000, '{"default": 1}'::jsonb, 1, '{}'::jsonb,
			'paid', 'evt-sub-0008', $1
		)
	`, finished); err != nil {
		t.Fatalf("seed legacy subscription order: %v", err)
	}

	migration, err := os.ReadFile("changes/0008_20260818_remove_bill_events.sql")
	if err != nil {
		t.Fatalf("read migration 0008: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0008: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 8 {
		t.Fatalf("schema version = %d, want 8", version)
	}

	var settledTenant, settledUser int64
	var settledStatus string
	var settledAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT tenant_payable, user_charged, billing_status, settled_at
		FROM ai_usage_logs WHERE request_id = 'req-0008-settled'
	`).Scan(&settledTenant, &settledUser, &settledStatus, &settledAt); err != nil {
		t.Fatalf("read settled usage: %v", err)
	}
	if settledTenant != 10 || settledUser != 20 || settledStatus != "settled" || settledAt == nil {
		t.Fatalf("settled usage = %d/%d/%s/%v, want authoritative 10/20/settled/non-null", settledTenant, settledUser, settledStatus, settledAt)
	}

	var refundStatus, refundReason string
	var refundedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT refund_status, refund_reason, refunded_at
		FROM ai_usage_logs WHERE request_id = 'req-0008-refunded'
	`).Scan(&refundStatus, &refundReason, &refundedAt); err != nil {
		t.Fatalf("read refunded usage: %v", err)
	}
	if refundStatus != "refunded" || refundReason != "refund requested by operator" || refundedAt == nil {
		t.Fatalf("refunded usage = %s/%s/%v, want refunded/reason/non-null", refundStatus, refundReason, refundedAt)
	}

	var debitReference string
	if err := pool.QueryRow(ctx, `SELECT debit_reference FROM ai_sub_orders WHERE order_no = 'ORD-0008'`).Scan(&debitReference); err != nil {
		t.Fatalf("read migrated subscription order: %v", err)
	}
	if debitReference != "subscription:ORD-0008" {
		t.Fatalf("debit reference = %q, want subscription:ORD-0008", debitReference)
	}

	var billEventsExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('bill_events') IS NOT NULL`).Scan(&billEventsExists); err != nil {
		t.Fatalf("check bill_events removal: %v", err)
	}
	if billEventsExists {
		t.Fatal("bill_events still exists after migration")
	}
}

func seedUsageFor0008(t *testing.T, ctx context.Context, pool *pgxpool.Pool, requestID, eventID, billingStatus string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_logs (
			request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
			model_code, requested_model, capability_type, billable_unit_type,
			tenant_payable, user_payable, user_charged, billing_status, request_status,
			client_protocol, billing_source, billing_event_id, settled_event_id
		) VALUES ($1, 'user', 'jwt', 'web_chat', 'tenant-0008', 'user-0008',
			'model', 'model', 'chat', 'token', 10, 20, 20, $3, 'success',
			'openai_chat', 'payg', $2, $2)
	`, requestID, eventID, billingStatus); err != nil {
		t.Fatalf("seed usage %s: %v", requestID, err)
	}
}

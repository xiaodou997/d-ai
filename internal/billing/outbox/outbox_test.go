package outbox_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/billing/ledger"
	"xiaodou/dai/internal/billing/outbox"
	"xiaodou/dai/internal/dbtest"
)

func openOutboxPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 8})
	if err != nil {
		t.Skipf("outbox test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	return pool, ctx
}

func seedTenantAndUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (tenantID, userID string) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID = "t_" + suffix
	userID = "u_" + suffix
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status) VALUES ($1, $1, 'active')
	`, tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, $1, 'x', 4, 'active')
	`, userID, tenantID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return tenantID, userID
}

func grant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ref ledger.Ref, micro int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := ledger.Grant(ctx, tx, ref, micro, nil, "ADMIN_RECHARGE", ""); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit grant: %v", err)
	}
}

func enqueue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, c outbox.Charge) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	// Mirrors production: the usage row and the charge share one commit.
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_usage_logs (request_id, key_owner_type, auth_method, request_source,
		  tenant_id, user_id, model_code, requested_model, capability_type, billable_unit_type,
		  billing_status, request_status, client_protocol, billing_source)
		VALUES ($1, 'user', 'jwt', 'web_chat', $2, $3, 'm', 'm', 'chat', 'token',
		        'pending', 'success', 'openai_chat', 'payg')
	`, c.RequestID, c.TenantID, c.UserID); err != nil {
		t.Fatalf("seed usage log: %v", err)
	}
	if err := outbox.Enqueue(ctx, tx, c); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit enqueue: %v", err)
	}
}

func balance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) int64 {
	t.Helper()
	var b int64
	if err := pool.QueryRow(ctx, `SELECT balance_micro FROM bill_accounts WHERE account_id = $1`, id).Scan(&b); err != nil {
		t.Fatalf("read balance: %v", err)
	}
	return b
}

func drain(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	total := 0
	for range 10 {
		n, err := outbox.NewConsumer(pool, nil).DrainOnce(ctx)
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		total += n
		if n == 0 {
			break
		}
	}
	return total
}

func TestDrainAppliesChargeAndLinksUsage(t *testing.T) {
	pool, ctx := openOutboxPool(t)
	tenantID, userID := seedTenantAndUser(t, ctx, pool)
	grant(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, 1_000_000)
	grant(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 1_000_000)

	enqueue(t, ctx, pool, outbox.Charge{
		RequestID: "req-1", TenantID: tenantID, UserID: userID,
		TenantMicro: 700, UserMicro: 900, Description: "AI",
	})
	drain(t, ctx, pool)

	if got := balance(t, ctx, pool, tenantID); got != 1_000_000-700 {
		t.Fatalf("tenant balance = %d", got)
	}
	if got := balance(t, ctx, pool, userID); got != 1_000_000-900 {
		t.Fatalf("user balance = %d", got)
	}

	var status string
	var settledAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT billing_status, settled_at FROM ai_usage_logs WHERE request_id = 'req-1'
	`).Scan(&status, &settledAt); err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if status != "settled" || settledAt == nil {
		t.Fatalf("usage after settlement = status:%s settled_at:%v", status, settledAt)
	}
}

// request_id is unique, so a completion replayed after an unknown commit result
// enqueues nothing the second time and the account is charged once.
func TestDuplicateEnqueueChargesOnce(t *testing.T) {
	pool, ctx := openOutboxPool(t)
	tenantID, userID := seedTenantAndUser(t, ctx, pool)
	grant(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 1_000_000)

	charge := outbox.Charge{RequestID: "req-dup", TenantID: tenantID, UserID: userID, UserMicro: 250}
	enqueue(t, ctx, pool, charge)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := outbox.Enqueue(ctx, tx, charge); err != nil {
		t.Fatalf("re-enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	drain(t, ctx, pool)
	if got := balance(t, ctx, pool, userID); got != 1_000_000-250 {
		t.Fatalf("balance = %d, want one 250 charge only", got)
	}
}

// Settlement records a cost that was already incurred, so a suspended or
// deleted account still owes it. The old code refused here, which rolled back
// the usage row along with the charge and lost both.
func TestChargeSettlesForDisabledAccounts(t *testing.T) {
	pool, ctx := openOutboxPool(t)
	tenantID, userID := seedTenantAndUser(t, ctx, pool)
	grant(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 100)

	enqueue(t, ctx, pool, outbox.Charge{
		RequestID: "req-disabled", TenantID: tenantID, UserID: userID, UserMicro: 500,
	})
	if _, err := pool.Exec(ctx, `UPDATE iam_accounts SET status = 'disabled' WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE iam_tenants SET status = 'disabled' WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}

	drain(t, ctx, pool)

	if got := balance(t, ctx, pool, userID); got != -400 {
		t.Fatalf("balance = %d, want -400: a disabled account still owes what it spent", got)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM bill_charge_outbox WHERE request_id = 'req-disabled'`).Scan(&status); err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if status != "done" {
		t.Fatalf("outbox status = %s, want done", status)
	}
}

// A charge that can never be applied must not hold up the ones behind it. The
// Redis stream this replaced re-read from the head every pass, so one poison
// entry starved everything after it forever.
func TestPoisonRowDoesNotBlockTheQueue(t *testing.T) {
	pool, ctx := openOutboxPool(t)
	tenantID, userID := seedTenantAndUser(t, ctx, pool)
	grant(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 1_000_000)

	// References an account that does not exist, so applying it always fails.
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_charge_outbox (request_id, tenant_id, user_id, user_micro)
		VALUES ('req-poison', $1, 'ghost-user', 100)
	`, tenantID); err != nil {
		t.Fatalf("seed poison row: %v", err)
	}
	enqueue(t, ctx, pool, outbox.Charge{
		RequestID: "req-behind", TenantID: tenantID, UserID: userID, UserMicro: 400,
	})

	drain(t, ctx, pool)

	if got := balance(t, ctx, pool, userID); got != 1_000_000-400 {
		t.Fatalf("balance = %d: the row behind the poison entry was not settled", got)
	}
	var attempts int
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT attempts, status FROM bill_charge_outbox WHERE request_id = 'req-poison'
	`).Scan(&attempts, &status); err != nil {
		t.Fatalf("read poison row: %v", err)
	}
	if attempts == 0 {
		t.Fatal("poison row was never attempted")
	}
	if status == "done" {
		t.Fatal("poison row was marked settled")
	}
}

// A charge that keeps failing eventually parks in 'failed' so it leaves the
// pending index and becomes visible to an operator instead of spinning.
func TestRepeatedlyFailingChargeIsParked(t *testing.T) {
	pool, ctx := openOutboxPool(t)
	tenantID, _ := seedTenantAndUser(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_charge_outbox (request_id, tenant_id, user_id, user_micro)
		VALUES ('req-doomed', $1, 'ghost-user', 100)
	`, tenantID); err != nil {
		t.Fatalf("seed: %v", err)
	}

	consumer := outbox.NewConsumer(pool, nil)
	for range 12 {
		if _, err := consumer.DrainOnce(ctx); err != nil {
			t.Fatalf("drain: %v", err)
		}
	}

	var status string
	var lastError *string
	if err := pool.QueryRow(ctx, `
		SELECT status, last_error FROM bill_charge_outbox WHERE request_id = 'req-doomed'
	`).Scan(&status, &lastError); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %s, want failed after exhausting attempts", status)
	}
	if lastError == nil || *lastError == "" {
		t.Fatal("failed row carries no diagnosis")
	}

	stats, err := outbox.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Failed != 1 || stats.Pending != 0 {
		t.Fatalf("stats = %+v, want 1 failed / 0 pending", stats)
	}
}

// Tenant-owned traffic charges the tenant and leaves no user row behind.
func TestTenantOnlyChargeNeedsNoUser(t *testing.T) {
	pool, ctx := openOutboxPool(t)
	tenantID, _ := seedTenantAndUser(t, ctx, pool)
	grant(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, 5_000)

	enqueue(t, ctx, pool, outbox.Charge{
		RequestID: "req-tenant", TenantID: tenantID, TenantMicro: 1_200,
	})

	drain(t, ctx, pool)
	if got := balance(t, ctx, pool, tenantID); got != 5_000-1_200 {
		t.Fatalf("tenant balance = %d", got)
	}
}

// Settlement health has to be observable: usage keeps being recorded and
// requests keep being admitted while a stalled consumer leaves balances frozen,
// so a stuck queue is invisible without this.
func TestStatsReportQueueHealth(t *testing.T) {
	pool, ctx := openOutboxPool(t)
	tenantID, userID := seedTenantAndUser(t, ctx, pool)
	grant(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 1_000_000)

	enqueue(t, ctx, pool, outbox.Charge{
		RequestID: "req-health", TenantID: tenantID, UserID: userID, UserMicro: 100,
	})

	before, err := outbox.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if before.Pending != 1 || before.Failed != 0 {
		t.Fatalf("stats before drain = %+v, want 1 pending / 0 failed", before)
	}
	if before.OldestS < 0 {
		t.Fatalf("oldest pending age = %v, want >= 0", before.OldestS)
	}

	drain(t, ctx, pool)

	after, err := outbox.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if after.Pending != 0 || after.Failed != 0 || after.OldestS != 0 {
		t.Fatalf("stats after drain = %+v, want empty queue", after)
	}
}

func TestEnqueueIgnoresZeroAmounts(t *testing.T) {
	pool, ctx := openOutboxPool(t)
	tenantID, userID := seedTenantAndUser(t, ctx, pool)

	if err := func() error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := outbox.Enqueue(ctx, tx, outbox.Charge{
			RequestID: "req-free", TenantID: tenantID, UserID: userID,
		}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}(); err != nil {
		t.Fatalf("enqueue zero charge: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_charge_outbox`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("outbox rows = %d, want 0 for a free request", count)
	}
}

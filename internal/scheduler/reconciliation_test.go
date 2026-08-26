package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"

	"xiaodou/dai/internal/dbtest"
)

func TestBillingReconciliationPublishesHealthyAndDiffMetrics(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("scheduler reconciliation database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('scheduler-recon-tenant', 'Scheduler Recon Tenant', 'active')
	`); err != nil {
		t.Fatalf("seed reconciliation tenant: %v", err)
	}

	s := NewScheduler(pool, nil, nil, zap.NewNop())
	if err := s.reconcileBilling(); err != nil {
		t.Fatalf("healthy billing reconciliation: %v", err)
	}
	if got := testutil.ToFloat64(billingReconciliationViolations); got != 0 {
		t.Fatalf("healthy violation metric = %v, want 0", got)
	}
	if got := testutil.ToFloat64(billingReconciliationLastHealthy); got <= 0 {
		t.Fatalf("healthy timestamp = %v, want positive", got)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE bill_accounts SET balance_micro = balance_micro + 1
		WHERE account_id = 'scheduler-recon-tenant'
	`); err != nil {
		t.Fatalf("corrupt reconciliation balance: %v", err)
	}
	if err := s.reconcileBilling(); err == nil {
		t.Fatal("unhealthy billing reconciliation returned nil")
	}
	if got := testutil.ToFloat64(billingReconciliationViolations); got != 1 {
		t.Fatalf("unhealthy violation metric = %v, want 1", got)
	}
	if got := testutil.ToFloat64(billingReconciliationViolationKinds.WithLabelValues("account_lot_conservation")); got != 1 {
		t.Fatalf("account violation metric = %v, want 1", got)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE bill_accounts SET balance_micro = balance_micro - 1
		WHERE account_id = 'scheduler-recon-tenant'
	`); err != nil {
		t.Fatalf("restore reconciliation balance: %v", err)
	}
	if err := s.reconcileBilling(); err != nil {
		t.Fatalf("reconciliation after repair: %v", err)
	}
	if got := testutil.ToFloat64(billingReconciliationViolations); got != 0 {
		t.Fatalf("repaired violation metric = %v, want 0", got)
	}
	if got := testutil.ToFloat64(billingReconciliationViolationKinds.WithLabelValues("account_lot_conservation")); got != 0 {
		t.Fatalf("repaired account violation metric = %v, want 0", got)
	}
}

func TestBillingReconciliationSkipsHeldTransactionLock(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("scheduler reconciliation database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatalf("begin lock holder: %v", err)
	}
	defer tx.Rollback(ctx)
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, "dai_scheduler_billing_reconciliation").Scan(&locked); err != nil {
		t.Fatalf("hold reconciliation lock: %v", err)
	}
	if !locked {
		t.Fatal("test could not acquire reconciliation lock")
	}

	s := NewScheduler(pool, nil, nil, zap.NewNop())
	err = s.reconcileBilling()
	var skipped *taskSkippedError
	if !errors.As(err, &skipped) || skipped.reason != "billing_reconciliation_lock_held" {
		t.Fatalf("held-lock reconciliation error = %v, want lock-held skip", err)
	}
}

func TestSchedulerHealthIncludesBillingReconciliationTask(t *testing.T) {
	s := NewScheduler(nil, nil, nil, zap.NewNop())
	health := s.Health()
	if _, ok := health.Tasks[TaskBillingReconciliation]; !ok {
		t.Fatalf("scheduler health has no %q task: %+v", TaskBillingReconciliation, health.Tasks)
	}

	// Keep this assertion close to the task registration so changing the
	// cadence cannot accidentally remove the periodic worker from Start.
	if billingReconciliationInterval < time.Minute {
		t.Fatalf("billing reconciliation interval = %s, unexpectedly aggressive", billingReconciliationInterval)
	}
}

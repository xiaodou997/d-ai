package pg_test

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/dbtest"
	paymentpg "xiaodou/dai/internal/payment/pg"
)

func TestDeleteStaleClosedOrdersOnlyRemovesUnpaidShells(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("payment test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-2 * 24 * time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_cleanup', 'Cleanup Tenant', 'active')
	`); err != nil {
		t.Fatalf("seed cleanup tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO pay_orders (
			order_id, out_trade_no, scene, tenant_id, payment_amount_minor,
			gross_amount_micro_usd, credited_amount_micro_usd, status,
			fulfillment_status, expires_at, created_at, updated_at
		) VALUES
			('PAY_CLEANUP_OLD', 'MCH_CLEANUP_OLD', 'tenant_topup', 'tenant_cleanup', 100,
			 1000000, 1000000, 'closed', 'pending', $1, $1, $1),
			('PAY_CLEANUP_RECENT', 'MCH_CLEANUP_RECENT', 'tenant_topup', 'tenant_cleanup', 100,
			 1000000, 1000000, 'closed', 'pending', $2, $2, $2),
			('PAY_CLEANUP_PAID', 'MCH_CLEANUP_PAID', 'tenant_topup', 'tenant_cleanup', 100,
			 1000000, 1000000, 'paid', 'credited', $1, $1, $1),
			('PAY_CLEANUP_LINKED', 'MCH_CLEANUP_LINKED', 'tenant_topup', 'tenant_cleanup', 100,
			 1000000, 1000000, 'closed', 'pending', $1, $1, $1);
	`, old, recent); err != nil {
		t.Fatalf("seed cleanup orders: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, credit_amount, paid_amount,
			payment_order_id, operator_id
		) VALUES ('ORD_CLEANUP_LINKED', 'online_tenant_topup', 'tenant_cleanup',
			1000000, 100, 'PAY_CLEANUP_LINKED', 'system:test');
	`); err != nil {
		t.Fatalf("seed linked cleanup order: %v", err)
	}

	deleted, err := paymentpg.DeleteStaleClosedOrders(ctx, pool, time.Now().UTC().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatalf("delete stale closed orders: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM pay_orders WHERE order_id IN ('PAY_CLEANUP_OLD', 'PAY_CLEANUP_RECENT', 'PAY_CLEANUP_PAID', 'PAY_CLEANUP_LINKED')`).Scan(&remaining); err != nil {
		t.Fatalf("count cleanup orders: %v", err)
	}
	if remaining != 3 {
		t.Fatalf("remaining = %d, want 3", remaining)
	}
}

package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/dbtest"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/payment"
)

func TestPaymentServiceOwnsUnifiedRechargeQueries(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_payment_query', 'Payment Query Tenant', 'active');
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, credit_amount, paid_amount, operator_id, note
		) VALUES (
			'ORD_PAYMENT_QUERY', 'platform_to_tenant', 'tenant_payment_query', 1000, 0, 'admin-query', 'manual query fixture'
		);
	`); err != nil {
		t.Fatalf("seed unified recharge query fixture: %v", err)
	}

	svc := &PaymentService{pool: pool, logger: zap.NewNop()}
	items, total, err := svc.ListAdminRechargeOrders(ctx, payment.ListAdminRechargeOrdersParams{
		Method: "manual", Page: 0, Size: 0,
	})
	if err != nil {
		t.Fatalf("ListAdminRechargeOrders: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].OrderID != "ORD_PAYMENT_QUERY" || items[0].Method != "manual" || items[0].OrderType != billing.OrderTypePlatformToTenant {
		t.Fatalf("unified recharge list = total:%d items:%+v", total, items)
	}

	detail, err := svc.GetAdminRechargeOrder(ctx, "ORD_PAYMENT_QUERY")
	if err != nil {
		t.Fatalf("GetAdminRechargeOrder: %v", err)
	}
	if detail.OrderID != "ORD_PAYMENT_QUERY" || detail.TenantName != "Payment Query Tenant" || detail.Note != "manual query fixture" {
		t.Fatalf("unified recharge detail = %+v", detail)
	}
	if _, err := svc.GetAdminRechargeOrder(ctx, "missing-payment-query"); !errors.Is(err, domain.ErrPaymentOrderNotFound) {
		t.Fatalf("missing unified recharge error = %v, want payment not found", err)
	}
}

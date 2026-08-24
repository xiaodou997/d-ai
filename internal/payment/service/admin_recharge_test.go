package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
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

func TestPaymentServiceOwnsAdminRechargeStateActions(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant_payment_action', 'Payment Action Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user_payment_action', 'tenant_payment_action', 'payment-action-user', 'x', 4, 'active');
		INSERT INTO bill_recharge_orders (
			order_id, order_type, tenant_id, user_id, credit_amount, paid_amount, operator_id
		) VALUES (
			'ORD_PAYMENT_ACTION', 'tenant_to_user', 'tenant_payment_action', 'user_payment_action', 1000, 0, 'tenant-operator'
		);
		INSERT INTO bill_credit_lots (
			lot_id, account_id, granted_micro, consumed_micro, source, recharge_order_id
		) VALUES ('LOT_PAYMENT_ACTION', 'user_payment_action', 1000, 0, 'TENANT_RECHARGE', 'ORD_PAYMENT_ACTION');
		UPDATE bill_accounts SET balance_micro = 1000 WHERE account_id = 'user_payment_action';
	`); err != nil {
		t.Fatalf("seed admin recharge action fixture: %v", err)
	}

	svc := &PaymentService{
		pool:      pool,
		logger:    zap.NewNop(),
		deduction: billingsvc.NewDeductionService(pool, zap.NewNop()),
	}
	if _, err := svc.SyncAdminRechargeOrder(ctx, "ORD_PAYMENT_ACTION"); !errors.Is(err, domain.ErrBadRequest) {
		t.Fatalf("manual sync error = %v, want bad request", err)
	}
	if _, err := svc.RecordAdminRechargeRefund(ctx, "ORD_PAYMENT_ACTION", RecordCompletedRefundParams{Method: payment.RefundMethodOffline, RefundReference: "offline-ref", Reason: "manual order"}); !errors.Is(err, domain.ErrBadRequest) {
		t.Fatalf("manual refund error = %v, want bad request", err)
	}

	item, err := svc.ReverseManualRechargeCredit(ctx, "ORD_PAYMENT_ACTION", "operator correction", "admin-action")
	if err != nil {
		t.Fatalf("manual recharge reverse: %v", err)
	}
	if item.OrderID != "ORD_PAYMENT_ACTION" || item.FulfillmentStatus != payment.FulfillmentStatusReversed {
		t.Fatalf("reversed admin recharge = %+v", item)
	}
	if _, err := svc.ReverseManualRechargeCredit(ctx, "ORD_PAYMENT_ACTION", "duplicate", "admin-action"); !errors.Is(err, domain.ErrRechargeNotReversible) {
		t.Fatalf("duplicate manual reverse error = %v, want not reversible", err)
	}
}

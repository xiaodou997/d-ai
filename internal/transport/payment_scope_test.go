package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/payment"
	paymentsvc "xiaodou/dai/internal/payment/service"
)

type topupLookupStub struct {
	scene    string
	tenantID string
	userID   string
}

func (s *topupLookupStub) GetTopupConfigView(context.Context, string, string) (*paymentsvc.TopupConfigView, error) {
	return nil, nil
}

func (s *topupLookupStub) CreateTopupOrder(context.Context, paymentsvc.CreateTopupOrderParams) (*payment.Order, error) {
	return nil, nil
}

func (s *topupLookupStub) GetOrderForScope(_ context.Context, _, scene, tenantID, userID string) (*payment.Order, error) {
	s.scene = scene
	s.tenantID = tenantID
	s.userID = userID
	return &payment.Order{OrderID: "order-1", TenantID: "tenant-1", Status: payment.OrderStatusPaid}, nil
}

func (s *topupLookupStub) ListOrders(context.Context, payment.ListOrdersParams) ([]*payment.Order, int64, error) {
	return nil, 0, nil
}

func TestGetTopupOrderUsesTenantScopeForTenantOrders(t *testing.T) {
	stub := &topupLookupStub{}
	h := &paymentHandlers{svc: stub}
	ctx := context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{
		UserID: "tenant-operator", TenantID: "tenant-1", UserType: int(auth.UserTypeTenant),
	})

	if _, err := h.getOrder(ctx, &getTopupOrderInput{OrderID: "order-1"}); err != nil {
		t.Fatalf("getOrder returned error: %v", err)
	}
	if stub.scene != payment.SceneTenantTopup {
		t.Fatalf("tenant order lookup scene = %q, want %q", stub.scene, payment.SceneTenantTopup)
	}
	if stub.tenantID != "tenant-1" {
		t.Fatalf("tenant order lookup tenant id = %q, want tenant-1", stub.tenantID)
	}
	if stub.userID != "" {
		t.Fatalf("tenant order lookup user id = %q, want empty", stub.userID)
	}
}

func TestGetTopupOrderUsesUserScopeForCustomerOrders(t *testing.T) {
	stub := &topupLookupStub{}
	h := &paymentHandlers{svc: stub}
	ctx := context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{
		UserID: "user-1", TenantID: "tenant-1", UserType: int(auth.UserTypeCustomer),
	})

	if _, err := h.getOrder(ctx, &getTopupOrderInput{OrderID: "order-1"}); err != nil {
		t.Fatalf("getOrder returned error: %v", err)
	}
	if stub.scene != payment.SceneUserTopup {
		t.Fatalf("customer order lookup scene = %q, want %q", stub.scene, payment.SceneUserTopup)
	}
	if stub.tenantID != "tenant-1" {
		t.Fatalf("customer order lookup tenant id = %q, want tenant-1", stub.tenantID)
	}
	if stub.userID != "user-1" {
		t.Fatalf("customer order lookup user id = %q, want user-1", stub.userID)
	}
}

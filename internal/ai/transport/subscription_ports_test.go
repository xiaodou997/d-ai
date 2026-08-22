package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

var (
	_ SubscriptionPlanCatalog       = (*subscription.Service)(nil)
	_ SubscriptionPlanManager       = (*subscription.Service)(nil)
	_ SubscriptionPurchaser         = (*subscription.Service)(nil)
	_ SubscriptionReader            = (*subscription.Service)(nil)
	_ SubscriptionOrderReader       = (*subscription.Service)(nil)
	_ SubscriptionGroupNameResolver = (*subscription.Service)(nil)
)

type subscriptionPlanCatalogStub struct {
	plans      []subscription.Plan
	plan       subscription.Plan
	filter     subscription.PlanFilter
	userID     string
	getID      string
	revisionID string
}

func (s *subscriptionPlanCatalogStub) ListPlans(_ context.Context, filter subscription.PlanFilter) ([]subscription.Plan, int64, error) {
	s.filter = filter
	return s.plans, int64(len(s.plans)), nil
}

func (s *subscriptionPlanCatalogStub) ListPlansForUser(_ context.Context, filter subscription.PlanFilter, userID string) ([]subscription.Plan, int64, error) {
	s.filter, s.userID = filter, userID
	return s.plans, int64(len(s.plans)), nil
}

func (s *subscriptionPlanCatalogStub) GetPlan(_ context.Context, id string) (*subscription.Plan, error) {
	s.getID = id
	plan := s.plan
	return &plan, nil
}

func (s *subscriptionPlanCatalogStub) ListPurchasePolicyRevisions(_ context.Context, planID string) ([]subscription.PurchasePolicyRevision, error) {
	s.revisionID = planID
	return nil, nil
}

type subscriptionPlanManagerStub struct {
	create          subscription.CreatePlanParams
	update          subscription.UpdatePlanParams
	reorderTenantID string
	reorderIDs      []string
	statusID        string
	statusTenantID  string
	status          string
	plan            subscription.Plan
}

func (s *subscriptionPlanManagerStub) CreatePlan(_ context.Context, params subscription.CreatePlanParams) (*subscription.Plan, error) {
	s.create = params
	plan := s.plan
	return &plan, nil
}

func (s *subscriptionPlanManagerStub) UpdatePlan(_ context.Context, params subscription.UpdatePlanParams) (bool, error) {
	s.update = params
	return true, nil
}

func (s *subscriptionPlanManagerStub) ReorderPlans(_ context.Context, tenantID string, planIDs []string) error {
	s.reorderTenantID = tenantID
	s.reorderIDs = append([]string(nil), planIDs...)
	return nil
}

func (s *subscriptionPlanManagerStub) SetPlanStatus(_ context.Context, id, tenantID, status string) (bool, error) {
	s.statusID, s.statusTenantID, s.status = id, tenantID, status
	return true, nil
}

type subscriptionPurchaserStub struct {
	params subscription.PurchaseParams
	order  subscription.Order
	sub    subscription.Subscription
}

func (s *subscriptionPurchaserStub) Purchase(_ context.Context, params subscription.PurchaseParams) (*subscription.Order, *subscription.Subscription, error) {
	s.params = params
	order, sub := s.order, s.sub
	return &order, &sub, nil
}

type subscriptionReaderStub struct {
	filter   subscription.SubFilter
	tenantID string
	userID   string
	items    []subscription.Subscription
	current  subscription.Subscription
}

func (s *subscriptionReaderStub) ListSubscriptions(_ context.Context, filter subscription.SubFilter) ([]subscription.Subscription, int64, error) {
	s.filter = filter
	return s.items, int64(len(s.items)), nil
}

func (s *subscriptionReaderStub) CurrentSubscription(_ context.Context, tenantID, userID string) (*subscription.Subscription, error) {
	s.tenantID, s.userID = tenantID, userID
	current := s.current
	return &current, nil
}

type subscriptionOrderReaderStub struct {
	filter subscription.OrderFilter
	getID  string
	items  []subscription.Order
	order  subscription.Order
}

func (s *subscriptionOrderReaderStub) ListOrders(_ context.Context, filter subscription.OrderFilter) ([]subscription.Order, int64, error) {
	s.filter = filter
	return s.items, int64(len(s.items)), nil
}

func (s *subscriptionOrderReaderStub) GetOrder(_ context.Context, id string) (*subscription.Order, error) {
	s.getID = id
	order := s.order
	return &order, nil
}

type subscriptionGroupNameResolverStub struct {
	ids []string
}

func (s *subscriptionGroupNameResolverStub) GroupNames(_ context.Context, ids []string) (map[string]string, error) {
	s.ids = append([]string(nil), ids...)
	return map[string]string{"group-1": "Primary"}, nil
}

func TestSubscriptionUserRoutesUseSeparatedPorts(t *testing.T) {
	plan := subscription.Plan{ID: "plan-1", TenantID: "tenant-1", Name: "Starter", Status: subscription.PlanOnSale}
	order := subscription.Order{ID: "order-1", TenantID: "tenant-1", UserID: "user-1", PlanID: "plan-1", Status: subscription.OrderPaid}
	sub := subscription.Subscription{
		ID: "sub-1", TenantID: "tenant-1", UserID: "user-1", PlanID: "plan-1", OrderID: "order-1",
		Status: subscription.SubActive, TotalLimitMicro: 1_000_000, GroupQuotaDebitMultipliers: map[string]float64{"group-1": 1},
	}
	plans := &subscriptionPlanCatalogStub{plans: []subscription.Plan{plan}, plan: plan}
	purchases := &subscriptionPurchaserStub{order: order, sub: sub}
	subscriptions := &subscriptionReaderStub{items: []subscription.Subscription{sub}, current: sub}
	orders := &subscriptionOrderReaderStub{items: []subscription.Order{order}, order: order}
	groupNames := &subscriptionGroupNameResolverStub{}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerUserSelfSubscriptions(api, SubscriptionHTTPDeps{
		SubscriptionPlans:      plans,
		SubscriptionPurchases:  purchases,
		Subscriptions:          subscriptions,
		SubscriptionOrders:     orders,
		SubscriptionGroupNames: groupNames,
	})
	handler := withCommercialClaims(router, &auth.Claims{TenantID: "tenant-1", UserID: "user-1"})

	requireCommercialStatus(t, performSubscriptionRequest(handler, http.MethodGet, "/api/v1/users/me/subscription-plans", "", ""), http.StatusOK)
	requireCommercialStatus(t, performSubscriptionRequest(handler, http.MethodPost, "/api/v1/users/me/subscription-orders", `{"plan_id":"plan-1"}`, "idem-1"), http.StatusCreated)
	requireCommercialStatus(t, performSubscriptionRequest(handler, http.MethodGet, "/api/v1/users/me/subscription-orders", "", ""), http.StatusOK)
	requireCommercialStatus(t, performSubscriptionRequest(handler, http.MethodGet, "/api/v1/users/me/subscription-orders/order-1", "", ""), http.StatusOK)
	requireCommercialStatus(t, performSubscriptionRequest(handler, http.MethodGet, "/api/v1/users/me/subscriptions", "", ""), http.StatusOK)
	requireCommercialStatus(t, performSubscriptionRequest(handler, http.MethodGet, "/api/v1/users/me/subscriptions/current", "", ""), http.StatusOK)

	if plans.filter.TenantID != "tenant-1" || plans.userID != "user-1" {
		t.Fatalf("plan query = filter %#v user %q", plans.filter, plans.userID)
	}
	if purchases.params.TenantID != "tenant-1" || purchases.params.UserID != "user-1" || purchases.params.PlanID != "plan-1" || purchases.params.IdempotencyKey != "idem-1" {
		t.Fatalf("purchase params = %#v", purchases.params)
	}
	if orders.filter.TenantID != "tenant-1" || orders.filter.UserID != "user-1" || orders.getID != "order-1" {
		t.Fatalf("order queries = filter %#v id %q", orders.filter, orders.getID)
	}
	if subscriptions.filter.TenantID != "tenant-1" || subscriptions.filter.UserID != "user-1" || subscriptions.tenantID != "tenant-1" || subscriptions.userID != "user-1" {
		t.Fatalf("subscription queries = filter %#v current %q/%q", subscriptions.filter, subscriptions.tenantID, subscriptions.userID)
	}
	if len(groupNames.ids) != 1 || groupNames.ids[0] != "group-1" {
		t.Fatalf("group name ids = %#v", groupNames.ids)
	}
}

func TestSubscriptionPlanReadAndWritePortsAreIndependent(t *testing.T) {
	plan := subscription.Plan{ID: "plan-1", TenantID: "tenant-1", Name: "Starter", Status: subscription.PlanDraft}
	plans := &subscriptionPlanCatalogStub{plans: []subscription.Plan{plan}, plan: plan}
	readRouter, readAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerUserSelfSubscriptions(readAPI, SubscriptionHTTPDeps{SubscriptionPlans: plans})
	readHandler := withCommercialClaims(readRouter, &auth.Claims{TenantID: "tenant-1", UserID: "user-1"})
	requireCommercialStatus(t, performSubscriptionRequest(readHandler, http.MethodGet, "/api/v1/users/me/subscription-plans", "", ""), http.StatusOK)
	requireCommercialStatus(t, performSubscriptionRequest(readHandler, http.MethodPost, "/api/v1/users/me/subscription-orders", `{"plan_id":"plan-1"}`, "idem-1"), http.StatusServiceUnavailable)

	manager := &subscriptionPlanManagerStub{plan: plan}
	writeRouter, writeAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerTenantSelfSubscriptions(writeAPI, SubscriptionHTTPDeps{SubscriptionPlanWriter: manager})
	writeHandler := withCommercialClaims(writeRouter, &auth.Claims{TenantID: "tenant-1", UserID: "tenant-admin"})
	body := `{"name":"Starter","price_micro_usd":1000000,"duration_days":7,"total_limit_micro_usd":5000000,"groups":[{"group_id":"group-1","quota_debit_multiplier":1}]}`
	requireCommercialStatus(t, performSubscriptionRequest(writeHandler, http.MethodPost, "/api/v1/tenants/me/subscription-plans", body, ""), http.StatusCreated)
	requireCommercialStatus(t, performSubscriptionRequest(writeHandler, http.MethodGet, "/api/v1/tenants/me/subscription-plans/plan-1", "", ""), http.StatusServiceUnavailable)
	requireCommercialStatus(t, performSubscriptionRequest(writeHandler, http.MethodPut, "/api/v1/tenants/me/subscription-plans/plan-1", body, ""), http.StatusServiceUnavailable)
	if manager.create.TenantID != "tenant-1" || manager.create.CreatedBy != "tenant-admin" || len(manager.create.Groups) != 1 {
		t.Fatalf("create params = %#v", manager.create)
	}

	combinedRouter, combinedAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerTenantSelfSubscriptions(combinedAPI, SubscriptionHTTPDeps{
		SubscriptionPlans:      plans,
		SubscriptionPlanWriter: manager,
	})
	combinedHandler := withCommercialClaims(combinedRouter, &auth.Claims{TenantID: "tenant-1", UserID: "tenant-admin"})
	requireCommercialStatus(t, performSubscriptionRequest(combinedHandler, http.MethodPut, "/api/v1/tenants/me/subscription-plans/plan-1", body, ""), http.StatusOK)
	requireCommercialStatus(t, performSubscriptionRequest(combinedHandler, http.MethodPut, "/api/v1/tenants/me/subscription-plans/plan-1/status", `{"status":"on_sale"}`, ""), http.StatusOK)
	if manager.update.ID != "plan-1" || plans.getID != "plan-1" || manager.statusID != "plan-1" || manager.statusTenantID != "tenant-1" || manager.status != subscription.PlanOnSale {
		t.Fatalf("combined plan calls = update %#v get %q status %q/%q/%q", manager.update, plans.getID, manager.statusTenantID, manager.statusID, manager.status)
	}
}

func TestSubscriptionGroupNameResolutionRemainsOptional(t *testing.T) {
	items := subscriptionsToDTO(t.Context(), nil, []subscription.Subscription{{
		ID: "sub-1", GroupQuotaDebitMultipliers: map[string]float64{"group-1": 1},
	}}, time.Now())
	if len(items) != 1 || len(items[0].Groups) != 1 || items[0].Groups[0].Name != "" {
		t.Fatalf("DTOs without resolver = %#v", items)
	}
}

func TestSubscriptionRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, CoreHTTPDeps{})
	coreResponse := performSubscriptionRequest(coreRouter, http.MethodGet, "/api/v1/users/me/subscription-plans", "", "")
	if coreResponse.Code != http.StatusNotFound {
		t.Fatalf("core AI subscription route status = %d, want %d", coreResponse.Code, http.StatusNotFound)
	}

	subscriptionRouter, subscriptionAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterSubscriptions(subscriptionAPI, SubscriptionHTTPDeps{})
	subscriptionResponse := performSubscriptionRequest(subscriptionRouter, http.MethodGet, "/api/v1/users/me/subscription-plans", "", "")
	if subscriptionResponse.Code != http.StatusUnauthorized {
		t.Fatalf("independent subscription route status = %d, want %d", subscriptionResponse.Code, http.StatusUnauthorized)
	}
}

func performSubscriptionRequest(handler http.Handler, method, path, body, idempotencyKey string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

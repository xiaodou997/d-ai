package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

var (
	_ CommercialGroupCatalog        = (*commercial.Service)(nil)
	_ CommercialGroupManager        = (*commercial.Service)(nil)
	_ CommercialDispatchRuleManager = (*commercial.Service)(nil)
	_ CommercialGroupTargetManager  = (*commercial.Service)(nil)
	_ CommercialUserBindingManager  = (*commercial.Service)(nil)
	_ CommercialLimitPolicyManager  = (*commercial.Service)(nil)
)

type commercialGroupCatalogStub struct {
	groups              []commercial.Group
	tenantVisible       []commercial.AccessibleGroup
	userVisible         []commercial.AccessibleGroup
	listTenantID        string
	getScope            commercial.TenantGroupScope
	visibleTenantID     string
	visibleUserTenantID string
	visibleUserID       string
}

func (s *commercialGroupCatalogStub) ListGroups(_ context.Context, tenantID string) ([]commercial.Group, error) {
	s.listTenantID = tenantID
	return s.groups, nil
}

func (s *commercialGroupCatalogStub) GetGroup(_ context.Context, scope commercial.TenantGroupScope) (commercial.Group, error) {
	s.getScope = scope
	for _, group := range s.groups {
		if group.ID == scope.GroupID {
			return group, nil
		}
	}
	return commercial.Group{ID: scope.GroupID, TenantID: scope.TenantID}, nil
}

func (s *commercialGroupCatalogStub) ListVisibleGroupsForTenant(_ context.Context, tenantID string) ([]commercial.AccessibleGroup, error) {
	s.visibleTenantID = tenantID
	return s.tenantVisible, nil
}

func (s *commercialGroupCatalogStub) ListVisibleGroupsForUser(_ context.Context, tenantID, userID string) ([]commercial.AccessibleGroup, error) {
	s.visibleUserTenantID = tenantID
	s.visibleUserID = userID
	return s.userVisible, nil
}

type commercialGroupManagerStub struct {
	CommercialGroupManager
	createTenantID string
	createInput    commercial.GroupWrite
	routeScope     commercial.TenantGroupScope
	routeInput     commercial.GroupRoutePolicyWrite
}

func (s *commercialGroupManagerStub) CreateGroup(_ context.Context, tenantID string, input commercial.GroupWrite) (commercial.Group, error) {
	s.createTenantID = tenantID
	s.createInput = input
	return commercial.Group{ID: "group-created", TenantID: tenantID, Name: input.Name, RetailPriceBookID: input.RetailPriceBookID, DefaultUserMultiplier: input.DefaultUserMultiplier, Status: input.Status}, nil
}

func (s *commercialGroupManagerStub) UpdateGroupRoutePolicy(_ context.Context, scope commercial.TenantGroupScope, input commercial.GroupRoutePolicyWrite) (commercial.Group, error) {
	s.routeScope = scope
	s.routeInput = input
	return commercial.Group{ID: scope.GroupID, TenantID: scope.TenantID, Name: "route-policy-group", RoutePolicy: input.RoutePolicy, Status: commercial.StatusActive}, nil
}

type commercialDispatchRuleManagerStub struct {
	CommercialDispatchRuleManager
	listScope commercial.TenantGroupScope
}

func (s *commercialDispatchRuleManagerStub) ListDispatchRules(_ context.Context, scope commercial.TenantGroupScope) ([]commercial.DispatchRule, error) {
	s.listScope = scope
	return nil, nil
}

type commercialGroupTargetManagerStub struct {
	CommercialGroupTargetManager
	listScope    commercial.TenantGroupScope
	replaceScope commercial.TenantGroupScope
	replaceInput commercial.GroupTargetBatchWrite
}

func (s *commercialGroupTargetManagerStub) ListGroupTargetDetails(_ context.Context, scope commercial.TenantGroupScope) ([]commercial.GroupTargetDetail, error) {
	s.listScope = scope
	return nil, nil
}

func (s *commercialGroupTargetManagerStub) ReplaceGroupTargets(_ context.Context, scope commercial.TenantGroupScope, input commercial.GroupTargetBatchWrite) (commercial.GroupTargetBatchResult, error) {
	s.replaceScope = scope
	s.replaceInput = input
	return commercial.GroupTargetBatchResult{RoutePolicyVersion: input.ExpectedVersion + 1}, nil
}

type commercialUserBindingManagerStub struct {
	CommercialUserBindingManager
	listTenantID string
	listUserID   string
}

func (s *commercialUserBindingManagerStub) ListUserBindings(_ context.Context, tenantID, userID string) ([]commercial.UserGroupBinding, error) {
	s.listTenantID = tenantID
	s.listUserID = userID
	return []commercial.UserGroupBinding{{TenantID: tenantID, UserID: userID, GroupID: "group-1"}}, nil
}

type commercialLimitPolicyManagerStub struct {
	CommercialLimitPolicyManager
	items       []commercial.LimitPolicy
	listFilter  commercial.LimitPolicyFilter
	createInput commercial.LimitPolicyWrite
	updateID    string
	updateInput commercial.LimitPolicyWrite
}

func (s *commercialLimitPolicyManagerStub) ListLimitPolicies(_ context.Context, filter commercial.LimitPolicyFilter) ([]commercial.LimitPolicy, error) {
	s.listFilter = filter
	return s.items, nil
}

func (s *commercialLimitPolicyManagerStub) CreateLimitPolicy(_ context.Context, input commercial.LimitPolicyWrite) (commercial.LimitPolicy, error) {
	s.createInput = input
	return commercial.LimitPolicy{ID: "policy-created", ScopeType: input.ScopeType, ScopeID: input.ScopeID, ConcurrencyLimit: input.ConcurrencyLimit, Status: input.Status}, nil
}

func (s *commercialLimitPolicyManagerStub) UpdateLimitPolicy(_ context.Context, id string, input commercial.LimitPolicyWrite) (commercial.LimitPolicy, error) {
	s.updateID = id
	s.updateInput = input
	return commercial.LimitPolicy{ID: id, ScopeType: input.ScopeType, ScopeID: input.ScopeID, ConcurrencyLimit: input.ConcurrencyLimit, Status: input.Status}, nil
}

type tenantEndUserVerifierStub struct {
	tenantID string
	userID   string
}

func (s *tenantEndUserVerifierStub) CheckTenantEndUser(_ context.Context, tenantID, userID string) error {
	s.tenantID = tenantID
	s.userID = userID
	return nil
}

func TestCommercialRoutesUseSeparatedPorts(t *testing.T) {
	groups := &commercialGroupCatalogStub{groups: []commercial.Group{{
		ID: "group-1", TenantID: "tenant-1", Name: "Primary", RetailPriceBookID: "price-1", DefaultUserMultiplier: 1.2, Status: commercial.StatusActive,
	}}}
	groupManager := &commercialGroupManagerStub{}
	dispatchRules := &commercialDispatchRuleManagerStub{}
	groupTargets := &commercialGroupTargetManagerStub{}
	userBindings := &commercialUserBindingManagerStub{}
	limitPolicies := &commercialLimitPolicyManagerStub{items: []commercial.LimitPolicy{{
		ID: "policy-1", ScopeType: commercial.LimitScopeTenant, ScopeID: "tenant-1", Status: commercial.StatusActive,
	}}}
	endUsers := &tenantEndUserVerifierStub{}
	d := TenantGroupManagementHTTPDeps{
		Groups:           groups,
		GroupManager:     groupManager,
		DispatchRules:    dispatchRules,
		GroupTargets:     groupTargets,
		UserBindings:     userBindings,
		TenantEndUsers:   endUsers,
		TenantPriceBooks: nil,
	}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerGroups(api, d)
	limitRouter, limitAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerLimits(limitAPI, CoreHTTPDeps{LimitPolicies: limitPolicies})
	handler := withCommercialClaims(router, &auth.Claims{TenantID: "tenant-1", UserID: "admin-1"})
	limitHandler := withCommercialClaims(limitRouter, &auth.Claims{TenantID: "tenant-1", UserID: "admin-1"})

	listRecorder := performCommercialRequest(handler, http.MethodGet, "/api/v1/tenants/me/groups", "")
	requireCommercialStatus(t, listRecorder, http.StatusOK)
	var listResponse struct {
		Items []groupDTO `json:"items"`
		Total int        `json:"total"`
	}
	decodeCommercialResponse(t, listRecorder, &listResponse)
	if groups.listTenantID != "tenant-1" || listResponse.Total != 1 || listResponse.Items[0].ID != "group-1" {
		t.Fatalf("group catalog call = tenant %q response %#v", groups.listTenantID, listResponse)
	}

	createRecorder := performCommercialRequest(handler, http.MethodPost, "/api/v1/tenants/me/groups", `{
		"name":"Created","retail_price_book_id":"price-1","default_user_multiplier":1.5,
		"user_default_visible":true,"allow_protocol_conversion":false,"sort_order":2,"status":"active"
	}`)
	requireCommercialStatus(t, createRecorder, http.StatusOK)
	if groupManager.createTenantID != "tenant-1" || groupManager.createInput.Name != "Created" || groupManager.createInput.DefaultUserMultiplier != 1.5 {
		t.Fatalf("group create command = tenant %q input %#v", groupManager.createTenantID, groupManager.createInput)
	}

	requireCommercialStatus(t, performCommercialRequest(handler, http.MethodGet, "/api/v1/tenants/me/groups/group-1/dispatch-rules", ""), http.StatusOK)
	if dispatchRules.listScope != (commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}) {
		t.Fatalf("dispatch scope = %#v", dispatchRules.listScope)
	}

	requireCommercialStatus(t, performCommercialRequest(handler, http.MethodGet, "/api/v1/tenants/me/groups/group-1/targets", ""), http.StatusOK)
	if groupTargets.listScope != (commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}) {
		t.Fatalf("target scope = %#v", groupTargets.listScope)
	}
	replaceRecorder := performCommercialRequest(handler, http.MethodPut, "/api/v1/tenants/me/groups/group-1/targets", `{"expected_version":1,"targets":[{"account_id":"account-1"}]}`)
	requireCommercialStatus(t, replaceRecorder, http.StatusOK)
	if groupTargets.replaceScope != (commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}) || groupTargets.replaceInput.ExpectedVersion != 1 || len(groupTargets.replaceInput.Targets) != 1 || groupTargets.replaceInput.Targets[0].TargetID != "account-1" {
		t.Fatalf("target replacement command = scope %#v input %#v", groupTargets.replaceScope, groupTargets.replaceInput)
	}
	policyRecorder := performCommercialRequest(handler, http.MethodPatch, "/api/v1/tenants/me/groups/group-1/route-policy", `{"route_policy":"cost","route_policy_version":1}`)
	requireCommercialStatus(t, policyRecorder, http.StatusOK)
	if groupManager.routeScope != (commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: "group-1"}) || groupManager.routeInput.RoutePolicy != commercial.RoutePolicyCost {
		t.Fatalf("route policy command = scope %#v input %#v", groupManager.routeScope, groupManager.routeInput)
	}

	bindingsRecorder := performCommercialRequest(handler, http.MethodGet, "/api/v1/tenants/me/users/user-1/groups", "")
	requireCommercialStatus(t, bindingsRecorder, http.StatusOK)
	if userBindings.listTenantID != "tenant-1" || userBindings.listUserID != "user-1" || endUsers.tenantID != "tenant-1" || endUsers.userID != "user-1" {
		t.Fatalf("binding scope = tenant %q user %q, verifier tenant %q user %q", userBindings.listTenantID, userBindings.listUserID, endUsers.tenantID, endUsers.userID)
	}

	requireCommercialStatus(t, performCommercialRequest(limitHandler, http.MethodGet, "/api/v1/limit-policies", ""), http.StatusOK)
	if limitPolicies.listFilter.ScopeType != commercial.LimitScopeTenant {
		t.Fatalf("limit policy filter = %#v", limitPolicies.listFilter)
	}
}

func TestCommercialGroupReadAndWritePortsAreIndependent(t *testing.T) {
	groups := &commercialGroupCatalogStub{}
	readRouter, readAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerGroups(readAPI, TenantGroupManagementHTTPDeps{Groups: groups})
	readHandler := withCommercialClaims(readRouter, &auth.Claims{TenantID: "tenant-1"})
	requireCommercialStatus(t, performCommercialRequest(readHandler, http.MethodGet, "/api/v1/tenants/me/groups", ""), http.StatusOK)
	requireCommercialStatus(t, performCommercialRequest(readHandler, http.MethodPost, "/api/v1/tenants/me/groups", `{"name":"Created","retail_price_book_id":"price-1"}`), http.StatusServiceUnavailable)

	manager := &commercialGroupManagerStub{}
	writeRouter, writeAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerGroups(writeAPI, TenantGroupManagementHTTPDeps{GroupManager: manager})
	writeHandler := withCommercialClaims(writeRouter, &auth.Claims{TenantID: "tenant-1"})
	requireCommercialStatus(t, performCommercialRequest(writeHandler, http.MethodGet, "/api/v1/tenants/me/groups", ""), http.StatusServiceUnavailable)
	requireCommercialStatus(t, performCommercialRequest(writeHandler, http.MethodPost, "/api/v1/tenants/me/groups", `{"name":"Created","retail_price_book_id":"price-1"}`), http.StatusOK)
}

func TestAPIKeyCommercialHelpersUseCatalogAndLimitPorts(t *testing.T) {
	groups := &commercialGroupCatalogStub{
		tenantVisible: []commercial.AccessibleGroup{{Group: commercial.Group{ID: "tenant-group"}}},
		userVisible:   []commercial.AccessibleGroup{{Group: commercial.Group{ID: "user-group"}}},
	}
	if err := ensureAPIKeyGroupAccessible(t.Context(), groups, "tenant-1", "", "tenant-group"); err != nil {
		t.Fatalf("tenant group access: %v", err)
	}
	if groups.visibleTenantID != "tenant-1" || groups.visibleUserID != "" {
		t.Fatalf("tenant visibility call = tenant %q user %q", groups.visibleTenantID, groups.visibleUserID)
	}
	if err := ensureAPIKeyGroupAccessible(t.Context(), groups, "tenant-1", "user-1", "user-group"); err != nil {
		t.Fatalf("user group access: %v", err)
	}
	if groups.visibleUserTenantID != "tenant-1" || groups.visibleUserID != "user-1" {
		t.Fatalf("user visibility call = tenant %q user %q", groups.visibleUserTenantID, groups.visibleUserID)
	}

	limit := int32(4)
	policies := &commercialLimitPolicyManagerStub{}
	created, err := syncAPIKeyLimitPolicy(t.Context(), policies, "key-1", &scopedLimitPolicyWriteRequest{ConcurrencyLimit: &limit}, "admin-1")
	if err != nil {
		t.Fatalf("create API key limit: %v", err)
	}
	if created == nil || policies.createInput.ScopeID != "key-1" || policies.createInput.ScopeType != commercial.LimitScopeAPIKey || policies.createInput.ConcurrencyLimit == nil || *policies.createInput.ConcurrencyLimit != 4 {
		t.Fatalf("created policy = %#v, input = %#v", created, policies.createInput)
	}

	policies.items = []commercial.LimitPolicy{{ID: "policy-1", ScopeType: commercial.LimitScopeAPIKey, ScopeID: "key-1"}}
	if _, err := syncAPIKeyLimitPolicy(t.Context(), policies, "key-1", &scopedLimitPolicyWriteRequest{ConcurrencyLimit: &limit, Status: "disabled"}, "admin-1"); err != nil {
		t.Fatalf("update API key limit: %v", err)
	}
	if policies.updateID != "policy-1" || policies.updateInput.Status != commercial.StatusDisabled {
		t.Fatalf("updated policy = id %q input %#v", policies.updateID, policies.updateInput)
	}
}

func withCommercialClaims(handler http.Handler, claims *auth.Claims) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ctx := context.WithValue(request.Context(), authClaimsContextKey{}, claims)
		handler.ServeHTTP(w, request.WithContext(ctx))
	})
}

func performCommercialRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func requireCommercialStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, want, recorder.Body.String())
	}
}

func decodeCommercialResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

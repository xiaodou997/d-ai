package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/libs/go/server"
)

var _ UpstreamAccessManager = (*upstreamaccess.Service)(nil)

type upstreamAccessManagerStub struct {
	listTenantID    string
	replaceTenantID string
	items           []upstreamaccess.ResourceAccess
	policies        []upstreamaccess.TenantResourcePolicy
}

func (s *upstreamAccessManagerStub) ListForTenant(_ context.Context, tenantID string) ([]upstreamaccess.ResourceAccess, error) {
	s.listTenantID = tenantID
	return s.items, nil
}

func (s *upstreamAccessManagerStub) ReplacePolicies(_ context.Context, tenantID string, policies []upstreamaccess.TenantResourcePolicy) error {
	s.replaceTenantID = tenantID
	s.policies = policies
	return nil
}

func TestTenantUpstreamAccessUsesManagerPort(t *testing.T) {
	override := 1.25
	manager := &upstreamAccessManagerStub{items: []upstreamaccess.ResourceAccess{{
		ResourceRef:               upstreamaccess.ResourceRef{Kind: upstreamaccess.KindDirectUpstream, ID: "account-1"},
		InternalName:              "internal-account",
		TenantDisplayName:         "Tenant account",
		AccessMode:                upstreamaccess.ModeRestricted,
		Status:                    "active",
		AccessGranted:             true,
		Allowed:                   true,
		DefaultTenantMultiplier:   1.1,
		TenantMultiplierOverride:  &override,
		EffectiveTenantMultiplier: 1.25,
	}}}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerTenantUpstreamAccess(api, UpstreamAccessManagementHTTPDeps{UpstreamAccess: manager})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/tenant-1/upstream-access", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("list upstream access status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if manager.listTenantID != "tenant-1" {
		t.Fatalf("ListForTenant tenant ID = %q, want tenant-1", manager.listTenantID)
	}
	var body struct {
		Items []tenantUpstreamAccessDTO `json:"items"`
		Total int                       `json:"total"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("list response = %#v", body)
	}
	item := body.Items[0]
	if item.ResourceID != "account-1" || item.TenantDisplayName != "Tenant account" || item.TenantMultiplierOverride == nil || *item.TenantMultiplierOverride != override {
		t.Fatalf("list item = %#v", item)
	}
}

func TestReplaceTenantUpstreamAccessUsesManagerPort(t *testing.T) {
	manager := &upstreamAccessManagerStub{}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerTenantUpstreamAccess(api, UpstreamAccessManagementHTTPDeps{UpstreamAccess: manager})

	request := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/tenant-2/upstream-access", strings.NewReader(`{
		"policies":[{"resource_kind":"oauth_pool","resource_id":"pool-1","access_granted":true,"tenant_multiplier_override":1.5}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("replace upstream access status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if manager.replaceTenantID != "tenant-2" || len(manager.policies) != 1 {
		t.Fatalf("ReplacePolicies tenant = %q policies = %#v", manager.replaceTenantID, manager.policies)
	}
	policy := manager.policies[0]
	if policy.Kind != upstreamaccess.KindOAuthPool || policy.ID != "pool-1" || !policy.AccessGranted || policy.TenantMultiplierOverride == nil || *policy.TenantMultiplierOverride != 1.5 {
		t.Fatalf("ReplacePolicies policy = %#v", policy)
	}
}

func TestTenantUpstreamAccessRequiresManagerPort(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerTenantUpstreamAccess(api, UpstreamAccessManagementHTTPDeps{})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/tenant-1/upstream-access", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("list without manager status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

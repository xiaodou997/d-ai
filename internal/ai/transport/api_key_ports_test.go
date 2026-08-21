package transport

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"xiaodou/dai/internal/ai/commercial"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/libs/go/server"
)

var (
	_ APIKeyReader           = (*identitycontrol.Service)(nil)
	_ APIKeyWriter           = (*identitycontrol.Service)(nil)
	_ APIKeyLifecycleManager = (*identitycontrol.Service)(nil)
	_ APIKeySecretManager    = (*identitycontrol.Service)(nil)
)

type apiKeyReaderStub struct {
	tenantKeys     []coreidentity.APIKey
	userKeys       []coreidentity.APIKey
	tenantID       string
	userTenantID   string
	userID         string
	tenantListCall int
	userListCall   int
}

func (s *apiKeyReaderStub) ListForTenant(_ context.Context, tenantID string) ([]coreidentity.APIKey, error) {
	s.tenantID = tenantID
	s.tenantListCall++
	return s.tenantKeys, nil
}

func (s *apiKeyReaderStub) ListForUser(_ context.Context, tenantID, userID string) ([]coreidentity.APIKey, error) {
	s.userTenantID = tenantID
	s.userID = userID
	s.userListCall++
	return s.userKeys, nil
}

type apiKeyWriterStub struct {
	APIKeyWriter
	createInput identitycontrol.CreateInput
	created     identitycontrol.Created
}

func (s *apiKeyWriterStub) Create(_ context.Context, input identitycontrol.CreateInput) (identitycontrol.Created, error) {
	s.createInput = input
	return s.created, nil
}

type apiKeyLifecycleStub struct {
	APIKeyLifecycleManager
	statusID       string
	statusTenantID string
	status         string
	deletedID      string
	deleteTenantID string
}

func (s *apiKeyLifecycleStub) UpdateStatus(_ context.Context, id, tenantID, status string) (coreidentity.APIKey, error) {
	s.statusID = id
	s.statusTenantID = tenantID
	s.status = status
	return coreidentity.APIKey{ID: id, TenantID: tenantID, OwnerScope: coreidentity.ScopeTenant, Status: status}, nil
}

func (s *apiKeyLifecycleStub) Delete(_ context.Context, id, tenantID string) error {
	s.deletedID = id
	s.deleteTenantID = tenantID
	return nil
}

type apiKeySecretStub struct {
	revealedID       string
	revealedTenantID string
	rotatedID        string
	rotatedTenantID  string
}

func (s *apiKeySecretStub) Reveal(_ context.Context, id, tenantID string) (string, error) {
	s.revealedID = id
	s.revealedTenantID = tenantID
	return "dai-secret", nil
}

func (s *apiKeySecretStub) Rotate(_ context.Context, id, tenantID string) (identitycontrol.Created, error) {
	s.rotatedID = id
	s.rotatedTenantID = tenantID
	return identitycontrol.Created{
		Key:          coreidentity.APIKey{ID: id, TenantID: tenantID, OwnerScope: coreidentity.ScopeTenant, Status: "active"},
		PlaintextKey: "dai-rotated",
	}, nil
}

type failingAPIKeyLimitPolicies struct {
	CommercialLimitPolicyManager
}

func (s *failingAPIKeyLimitPolicies) ListLimitPolicies(context.Context, commercial.LimitPolicyFilter) ([]commercial.LimitPolicy, error) {
	return nil, nil
}

func (s *failingAPIKeyLimitPolicies) CreateLimitPolicy(context.Context, commercial.LimitPolicyWrite) (commercial.LimitPolicy, error) {
	return commercial.LimitPolicy{}, errors.New("limit policy write failed")
}

func TestAPIKeyRoutesUseSeparatedPorts(t *testing.T) {
	key := coreidentity.APIKey{ID: "key-1", TenantID: "tenant-1", OwnerScope: coreidentity.ScopeTenant, Name: "Primary", Status: "active"}
	reader := &apiKeyReaderStub{tenantKeys: []coreidentity.APIKey{key}}
	lifecycle := &apiKeyLifecycleStub{}
	secrets := &apiKeySecretStub{}
	policies := &commercialLimitPolicyManagerStub{}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerAPIKeys(api, AIDeps{
		IdentityDeps: IdentityDeps{
			APIKeys:         reader,
			APIKeyLifecycle: lifecycle,
			APIKeySecrets:   secrets,
		},
		CatalogDeps: CatalogDeps{LimitPolicies: policies},
	})

	requireCommercialStatus(t, performCommercialRequest(router, http.MethodGet, "/api/v1/tenants/tenant-1/api-keys", ""), http.StatusOK)
	requireCommercialStatus(t, performCommercialRequest(router, http.MethodPatch, "/api/v1/tenants/tenant-1/api-keys/key-1/status", `{"status":"disabled"}`), http.StatusOK)
	requireCommercialStatus(t, performCommercialRequest(router, http.MethodPost, "/api/v1/tenants/tenant-1/api-keys/key-1/reveal", ""), http.StatusOK)
	requireCommercialStatus(t, performCommercialRequest(router, http.MethodPost, "/api/v1/tenants/tenant-1/api-keys/key-1/rotate", ""), http.StatusOK)

	if reader.tenantID != "tenant-1" || reader.tenantListCall != 4 {
		t.Fatalf("reader scope = tenant %q calls %d", reader.tenantID, reader.tenantListCall)
	}
	if lifecycle.statusID != "key-1" || lifecycle.statusTenantID != "tenant-1" || lifecycle.status != "disabled" {
		t.Fatalf("status command = id %q tenant %q status %q", lifecycle.statusID, lifecycle.statusTenantID, lifecycle.status)
	}
	if secrets.revealedID != "key-1" || secrets.revealedTenantID != "tenant-1" || secrets.rotatedID != "key-1" || secrets.rotatedTenantID != "tenant-1" {
		t.Fatalf("secret commands = reveal %q/%q rotate %q/%q", secrets.revealedTenantID, secrets.revealedID, secrets.rotatedTenantID, secrets.rotatedID)
	}
}

func TestAPIKeyReadPortDoesNotEnableLifecycleOrSecretOperations(t *testing.T) {
	reader := &apiKeyReaderStub{tenantKeys: []coreidentity.APIKey{{ID: "key-1", TenantID: "tenant-1", OwnerScope: coreidentity.ScopeTenant}}}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerAPIKeys(api, AIDeps{
		IdentityDeps: IdentityDeps{APIKeys: reader},
		CatalogDeps:  CatalogDeps{LimitPolicies: &commercialLimitPolicyManagerStub{}},
	})

	requireCommercialStatus(t, performCommercialRequest(router, http.MethodGet, "/api/v1/tenants/tenant-1/api-keys", ""), http.StatusOK)
	requireCommercialStatus(t, performCommercialRequest(router, http.MethodPatch, "/api/v1/tenants/tenant-1/api-keys/key-1/status", `{"status":"disabled"}`), http.StatusServiceUnavailable)
	requireCommercialStatus(t, performCommercialRequest(router, http.MethodPost, "/api/v1/tenants/tenant-1/api-keys/key-1/reveal", ""), http.StatusServiceUnavailable)
}

func TestAPIKeyCreateCompensatesThroughLifecyclePort(t *testing.T) {
	writer := &apiKeyWriterStub{created: identitycontrol.Created{
		Key:          coreidentity.APIKey{ID: "key-created", TenantID: "tenant-1", OwnerScope: coreidentity.ScopeTenant, GroupID: "group-1", Name: "Primary", Status: "active"},
		PlaintextKey: "dai-created",
	}}
	lifecycle := &apiKeyLifecycleStub{}
	groups := &commercialGroupCatalogStub{tenantVisible: []commercial.AccessibleGroup{{Group: commercial.Group{ID: "group-1"}}}}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerAPIKeys(api, AIDeps{
		IdentityDeps: IdentityDeps{APIKeyWriter: writer, APIKeyLifecycle: lifecycle},
		CatalogDeps: CatalogDeps{
			Groups:        groups,
			LimitPolicies: &failingAPIKeyLimitPolicies{},
		},
	})

	recorder := performCommercialRequest(router, http.MethodPost, "/api/v1/tenants/tenant-1/api-keys", `{
		"name":"Primary","group_id":"group-1","limit_policy":{"concurrency_limit":2}
	}`)
	requireCommercialStatus(t, recorder, http.StatusInternalServerError)
	if writer.createInput.TenantID != "tenant-1" || writer.createInput.GroupID != "group-1" {
		t.Fatalf("create command = %#v", writer.createInput)
	}
	if lifecycle.deletedID != "key-created" || lifecycle.deleteTenantID != "tenant-1" {
		t.Fatalf("compensating delete = id %q tenant %q", lifecycle.deletedID, lifecycle.deleteTenantID)
	}
}

package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/server"
)

type credentialReaderStub struct {
	summary *domain.OAuthCredentialSummary
}

type credentialCreatorStub struct {
	poolID string
	input  domain.OAuthCredentialCreate
	id     string
}

type poolWriterStub struct {
	created domain.CredentialPoolCreate
	id      string
}

type poolHealthReaderStub struct {
	called bool
	rows   []domain.OAuthPoolHealthSummary
}

func (s *poolHealthReaderStub) GetPoolHealthSummary(context.Context) ([]domain.OAuthPoolHealthSummary, error) {
	s.called = true
	return s.rows, nil
}

func (s *poolWriterStub) CreatePool(_ context.Context, input domain.CredentialPoolCreate) (string, error) {
	s.created = input
	return s.id, nil
}

func (*poolWriterStub) UpdatePool(context.Context, string, domain.CredentialPoolUpdate) error {
	return nil
}

func (*poolWriterStub) UpdatePoolStatus(context.Context, string, string) error { return nil }

func (*poolWriterStub) DeletePool(context.Context, string) error { return nil }

func (s *credentialCreatorStub) Create(_ context.Context, poolID string, input domain.OAuthCredentialCreate) (string, error) {
	s.poolID = poolID
	s.input = input
	return s.id, nil
}

func (s credentialReaderStub) ListForPool(context.Context, string) ([]domain.OAuthCredentialSummary, error) {
	if s.summary == nil {
		return nil, nil
	}
	return []domain.OAuthCredentialSummary{*s.summary}, nil
}

func (s credentialReaderStub) GetSummaryByID(context.Context, string) (*domain.OAuthCredentialSummary, error) {
	return s.summary, nil
}

func TestImportPoolCredentialUsesDedicatedCreatorPort(t *testing.T) {
	creator := &credentialCreatorStub{id: "cred-1"}
	reader := credentialReaderStub{summary: &domain.OAuthCredentialSummary{ID: "cred-1", PoolID: "pool-1"}}
	poolReader := &poolReaderStub{pool: &domain.CredentialPool{
		ID:                "pool-1",
		FixedProviderType: domain.FixedProviderCodex,
	}}

	got, err := importPoolCredential(context.Background(), OAuthManagementHTTPDeps{
		CredentialCreator: creator,
		CredentialReader:  reader,
		PoolReader:        poolReader,
	}, "pool-1", poolCredentialWriteRequest{
		Name:        " Imported account ",
		AccessToken: " access-token ",
	})
	if err != nil {
		t.Fatalf("importPoolCredential() error = %v", err)
	}
	if got != reader.summary {
		t.Fatalf("importPoolCredential() summary = %#v, want %#v", got, reader.summary)
	}
	if creator.poolID != "pool-1" || poolReader.poolID != "pool-1" {
		t.Fatalf("port pool IDs = creator %q, reader %q", creator.poolID, poolReader.poolID)
	}
	if creator.input.ProviderType != domain.FixedProviderCodex || creator.input.AccessToken != "access-token" {
		t.Fatalf("creator input = %#v", creator.input)
	}
}

func TestCreateCredentialPoolUsesDedicatedPoolPorts(t *testing.T) {
	reader := &poolReaderStub{pool: &domain.CredentialPool{
		ID:                "pool-1",
		Name:              "Port pool",
		TenantDisplayName: "Port pool",
		TenantAccessMode:  "public",
		FixedProviderType: domain.FixedProviderCodex,
		OAuthStrategy:     "round_robin",
		Status:            "disabled",
	}}
	writer := &poolWriterStub{id: "pool-1"}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerOAuthPools(api, OAuthManagementHTTPDeps{
		PoolReader: reader,
		PoolWriter: writer,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/credential-pools", strings.NewReader(`{
		"name":" Port pool ",
		"fixed_provider_type":"codex"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create credential pool status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if reader.poolID != "pool-1" {
		t.Fatalf("GetPool() pool ID = %q, want pool-1", reader.poolID)
	}
	if writer.created.Name != "Port pool" || writer.created.FixedProviderType != domain.FixedProviderCodex {
		t.Fatalf("CreatePool() input = %#v", writer.created)
	}
}

func TestOAuthPoolHealthUsesDedicatedReaderPort(t *testing.T) {
	reader := &poolHealthReaderStub{rows: []domain.OAuthPoolHealthSummary{{
		PoolID:            "pool-1",
		PoolName:          "Health pool",
		FixedProviderType: domain.FixedProviderCodex,
		OAuthStrategy:     "round_robin",
		Total:             3,
		Active:            2,
		Invalid:           1,
	}}}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerOAuthPools(api, OAuthManagementHTTPDeps{PoolHealthReader: reader})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/oauth-pool-health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("oauth pool health status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !reader.called {
		t.Fatal("GetPoolHealthSummary() was not called")
	}
	var body struct {
		Items []oauthPoolHealthDTO `json:"items"`
		Total int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("health response = %#v", body)
	}
	item := body.Items[0]
	if item.PoolID != "pool-1" || item.FixedProviderType != string(domain.FixedProviderCodex) || item.Active != 2 {
		t.Fatalf("health item = %#v", item)
	}
}

func TestOAuthManagementRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/credential-pools"},
		{method: http.MethodPost, path: "/api/v1/credential-pools"},
		{method: http.MethodPatch, path: "/api/v1/credential-pools/pool-1"},
		{method: http.MethodPatch, path: "/api/v1/credential-pools/pool-1/status"},
		{method: http.MethodDelete, path: "/api/v1/credential-pools/pool-1"},
		{method: http.MethodGet, path: "/api/v1/credential-pools/pool-1/credentials"},
		{method: http.MethodPost, path: "/api/v1/credential-pools/pool-1/credentials"},
		{method: http.MethodPatch, path: "/api/v1/credential-pools/pool-1/credentials/cred-1"},
		{method: http.MethodPost, path: "/api/v1/credential-pools/pool-1/credentials/cred-1/refresh"},
		{method: http.MethodDelete, path: "/api/v1/credential-pools/pool-1/credentials/cred-1"},
		{method: http.MethodGet, path: "/api/v1/credential-pools/pool-1/available-models"},
		{method: http.MethodPost, path: "/api/v1/credential-pools/pool-1/import-available-models"},
		{method: http.MethodGet, path: "/api/v1/oauth-pool-health"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, route := range routes {
		recorder := performOAuthManagementRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI OAuth route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	oauthRouter, oauthAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterOAuthManagement(oauthAPI, OAuthManagementHTTPDeps{})
	for _, route := range routes {
		recorder := performOAuthManagementRequest(oauthRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent OAuth route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performOAuthManagementRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestPoolCredentialSummaryToDTOAllowsOnlyKnownIdentityMetadata(t *testing.T) {
	dto := poolCredentialSummaryToDTO(domain.OAuthCredentialSummary{
		ID:     "cred-1",
		Status: "active",
		AuthMetadata: map[string]any{
			"account_id":    "account-1",
			"plan_type":     "team",
			"access_token":  "must-not-leak",
			"private_key":   "must-not-leak",
			"authorization": "must-not-leak",
			"nested": map[string]any{
				"client_secret": "must-not-leak",
			},
		},
	})

	if dto.AuthMetadata["account_id"] != "account-1" {
		t.Fatalf("account metadata was lost: %#v", dto.AuthMetadata)
	}
	if _, ok := dto.AuthMetadata["access_token"]; ok {
		t.Fatal("access token metadata leaked into DTO")
	}
	for _, key := range []string{"private_key", "authorization", "nested"} {
		if _, ok := dto.AuthMetadata[key]; ok {
			t.Fatalf("metadata key %q leaked into DTO", key)
		}
	}
	if dto.AuthMetadata["plan_type"] != "team" {
		t.Fatalf("plan metadata was lost: %#v", dto.AuthMetadata)
	}
}

func TestGetPoolCredentialScopedUsesSummaryReader(t *testing.T) {
	reader := credentialReaderStub{summary: &domain.OAuthCredentialSummary{ID: "cred-1", PoolID: "pool-1"}}

	got, err := getPoolCredentialScoped(context.Background(), reader, "pool-1", "cred-1")
	if err != nil {
		t.Fatalf("getPoolCredentialScoped() error = %v", err)
	}
	if got.ID != "cred-1" {
		t.Fatalf("credential ID = %q, want %q", got.ID, "cred-1")
	}
}

func TestGetPoolCredentialScopedRejectsWrongPool(t *testing.T) {
	reader := credentialReaderStub{summary: &domain.OAuthCredentialSummary{ID: "cred-1", PoolID: "pool-2"}}

	if _, err := getPoolCredentialScoped(context.Background(), reader, "pool-1", "cred-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("getPoolCredentialScoped() error = %v, want domain.ErrNotFound", err)
	}
}

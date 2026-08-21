package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/server"
)

type poolReaderStub struct {
	poolID string
	pool   *domain.CredentialPool
	pools  []domain.CredentialPool
}

func TestNormalizeBatchDeleteBindingIDs(t *testing.T) {
	ids, err := normalizeBatchDeleteBindingIDs([]string{
		" 10000000-0000-0000-0000-0000000000AB ",
		"100000000000000000000000000000ab",
		"20000000-0000-0000-0000-000000000002",
	})
	if err != nil {
		t.Fatalf("normalizeBatchDeleteBindingIDs(): %v", err)
	}
	want := []string{
		"10000000-0000-0000-0000-0000000000ab",
		"20000000-0000-0000-0000-000000000002",
	}
	if len(ids) != len(want) {
		t.Fatalf("IDs = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("IDs = %v, want %v", ids, want)
		}
	}
}

func TestNormalizeBatchDeleteBindingIDsRejectsInvalidInput(t *testing.T) {
	tests := [][]string{
		nil,
		{"not-a-uuid"},
		{"urn:uuid:10000000-0000-0000-0000-000000000001"},
	}
	for _, values := range tests {
		if _, err := normalizeBatchDeleteBindingIDs(values); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("normalizeBatchDeleteBindingIDs(%v) error = %v, want validation error", values, err)
		}
	}
}

type modelBindingStoreStub struct {
	gotScope domain.UpstreamModelBindingScope
	gotWrite domain.UpstreamModelBindingWrite
	created  domain.UpstreamModelBinding
}

func (s *modelBindingStoreStub) List(_ context.Context, scope domain.UpstreamModelBindingScope) ([]domain.UpstreamModelBinding, error) {
	s.gotScope = scope
	return []domain.UpstreamModelBinding{s.created}, nil
}
func (s *modelBindingStoreStub) ListModelCodes(context.Context, domain.UpstreamModelBindingScope) ([]string, error) {
	return nil, nil
}
func (s *modelBindingStoreStub) FindByModel(context.Context, domain.UpstreamModelBindingScope, string) (domain.UpstreamModelBinding, error) {
	return s.created, nil
}
func (s *modelBindingStoreStub) Get(context.Context, domain.UpstreamModelBindingScope, string) (domain.UpstreamModelBinding, error) {
	return s.created, nil
}
func (s *modelBindingStoreStub) Create(_ context.Context, scope domain.UpstreamModelBindingScope, write domain.UpstreamModelBindingWrite) (domain.UpstreamModelBinding, error) {
	s.gotScope, s.gotWrite = scope, write
	return s.created, nil
}
func (s *modelBindingStoreStub) Update(context.Context, domain.UpstreamModelBindingScope, string, domain.UpstreamModelBindingWrite) (domain.UpstreamModelBinding, error) {
	return s.created, nil
}
func (s *modelBindingStoreStub) Delete(context.Context, domain.UpstreamModelBindingScope, string) error {
	return nil
}
func (s *modelBindingStoreStub) BatchDelete(context.Context, domain.UpstreamModelBindingScope, []string) (int64, error) {
	return 0, nil
}
func (s *modelBindingStoreStub) Import(context.Context, domain.UpstreamModelBindingScope, []domain.UpstreamModelBindingWrite) (domain.UpstreamModelBindingImportResult, error) {
	return domain.UpstreamModelBindingImportResult{}, nil
}

func (s *poolReaderStub) ListPools(context.Context) ([]domain.CredentialPool, error) {
	return s.pools, nil
}

func (s *poolReaderStub) GetPool(_ context.Context, poolID string) (*domain.CredentialPool, error) {
	s.poolID = poolID
	return s.pool, nil
}

func TestEnsurePoolExistsUsesReadOnlyPoolPort(t *testing.T) {
	reader := &poolReaderStub{pool: &domain.CredentialPool{ID: "pool-1"}}

	got, err := ensurePoolExists(context.Background(), reader, "pool-1")
	if err != nil {
		t.Fatalf("ensurePoolExists() error = %v", err)
	}
	if got != reader.pool {
		t.Fatalf("ensurePoolExists() pool = %#v, want %#v", got, reader.pool)
	}
	if reader.poolID != "pool-1" {
		t.Fatalf("GetPool() pool ID = %q, want %q", reader.poolID, "pool-1")
	}
}

func TestEnsurePoolExistsRequiresPoolReader(t *testing.T) {
	if _, err := ensurePoolExists(context.Background(), nil, "pool-1"); err == nil {
		t.Fatal("ensurePoolExists() error = nil, want unavailable error")
	}
}

func TestCreateUpstreamModelBindingUsesDomainStore(t *testing.T) {
	const accountID = "10000000-0000-0000-0000-000000000001"
	store := &modelBindingStoreStub{created: domain.UpstreamModelBinding{ID: "binding-1", ModelCode: "gpt-test"}}
	got, err := createUpstreamModelBinding(context.Background(), store, string(domain.UpstreamKindDirect), accountID, string(domain.EndpointProtocolOpenAICompatible), nil, upstreamModelBindingWriteRequest{
		ModelCode: "gpt-test",
	})
	if err != nil {
		t.Fatalf("createUpstreamModelBinding() error = %v", err)
	}
	if got.ID != "binding-1" {
		t.Fatalf("binding = %+v", got)
	}
	if store.gotScope.Kind != domain.UpstreamKindDirect || store.gotScope.ID != accountID {
		t.Fatalf("scope = %+v", store.gotScope)
	}
	if store.gotWrite.ModelCode != "gpt-test" || store.gotWrite.UpstreamModelName != "gpt-test" || store.gotWrite.Status != "active" {
		t.Fatalf("write = %+v", store.gotWrite)
	}
	if store.gotWrite.CapabilityType != "chat" || store.gotWrite.APIFormat != "openai_responses" || len(store.gotWrite.ConfigJSON) == 0 {
		t.Fatalf("normalized write = %+v", store.gotWrite)
	}
}

func TestModelBindingRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/upstream-accounts/account-1/model-bindings"},
		{method: http.MethodPost, path: "/api/v1/upstream-accounts/account-1/model-bindings"},
		{method: http.MethodPatch, path: "/api/v1/upstream-accounts/account-1/model-bindings/binding-1"},
		{method: http.MethodDelete, path: "/api/v1/upstream-accounts/account-1/model-bindings/binding-1"},
		{method: http.MethodPost, path: "/api/v1/upstream-accounts/account-1/model-bindings/batch-delete"},
		{method: http.MethodGet, path: "/api/v1/credential-pools/pool-1/model-bindings"},
		{method: http.MethodPost, path: "/api/v1/credential-pools/pool-1/model-bindings"},
		{method: http.MethodPatch, path: "/api/v1/credential-pools/pool-1/model-bindings/binding-1"},
		{method: http.MethodDelete, path: "/api/v1/credential-pools/pool-1/model-bindings/binding-1"},
		{method: http.MethodPost, path: "/api/v1/credential-pools/pool-1/model-bindings/batch-delete"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, route := range routes {
		recorder := performModelBindingRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI model-binding route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	modelBindingRouter, modelBindingAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterModelBindings(modelBindingAPI, ModelBindingHTTPDeps{})
	for _, route := range routes {
		recorder := performModelBindingRequest(modelBindingRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent model-binding route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performModelBindingRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestLoadUpstreamTestBindingUsesDomainStore(t *testing.T) {
	const accountID = "10000000-0000-0000-0000-000000000001"
	store := &modelBindingStoreStub{created: domain.UpstreamModelBinding{
		APIFormat:         "openai_images",
		UpstreamModelName: "vendor-image-model",
		CapabilityType:    "image",
		ConfigJSON:        []byte(`{"image_generation":{"stream_mode":"force_stream","upstream_response_format":"b64_json"}}`),
	}}

	got, err := loadUpstreamTestBinding(context.Background(), store, accountID, "image-model")
	if err != nil {
		t.Fatalf("loadUpstreamTestBinding() error = %v", err)
	}
	if got.APIFormat != "openai_images" || got.UpstreamModel != "vendor-image-model" || got.CapabilityType != "image" {
		t.Fatalf("binding = %+v", got)
	}
	if got.ImagePolicy.StreamMode != domain.ImageStreamModeForceStream || got.ImagePolicy.UpstreamResponseFormat != domain.ImageResponseFormatB64 {
		t.Fatalf("image policy = %+v", got.ImagePolicy)
	}
}

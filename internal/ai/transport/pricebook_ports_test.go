package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

var (
	_ PlatformPriceBookManager = (*billingcontrol.Service)(nil)
	_ TenantPriceBookManager   = (*billingcontrol.Service)(nil)
	_ PriceBookSyncManager     = (*billingcontrol.Service)(nil)
)

type platformPriceBookManagerStub struct {
	books            []domain.PriceBook
	entries          []domain.PriceBookEntry
	listCalls        int
	createName       string
	createDesc       string
	getID            string
	updateID         string
	deleteID         string
	listEntriesID    string
	upsertEntry      domain.PriceBookEntry
	deleteEntryID    string
	deleteModel      string
	deleteCapability string
}

func (s *platformPriceBookManagerStub) ListPriceBooks(context.Context) ([]domain.PriceBook, error) {
	s.listCalls++
	return s.books, nil
}

func (s *platformPriceBookManagerStub) CreatePriceBook(_ context.Context, name, description string) (domain.PriceBook, error) {
	s.createName = name
	s.createDesc = description
	return domain.PriceBook{ID: "platform-created", OwnerType: domain.PriceBookOwnerPlatform, Name: name, Description: description, Status: "active"}, nil
}

func (s *platformPriceBookManagerStub) GetPlatformPriceBook(_ context.Context, id string) (domain.PriceBook, error) {
	s.getID = id
	return domain.PriceBook{ID: id, OwnerType: domain.PriceBookOwnerPlatform, Name: "Platform", Status: "active"}, nil
}

func (s *platformPriceBookManagerStub) UpdatePriceBook(_ context.Context, id, name, description, status string) (domain.PriceBook, error) {
	s.updateID = id
	return domain.PriceBook{ID: id, OwnerType: domain.PriceBookOwnerPlatform, Name: name, Description: description, Status: status}, nil
}

func (s *platformPriceBookManagerStub) DeletePriceBook(_ context.Context, id string) error {
	s.deleteID = id
	return nil
}

func (s *platformPriceBookManagerStub) ListEntries(_ context.Context, priceBookID string) ([]domain.PriceBookEntry, error) {
	s.listEntriesID = priceBookID
	return s.entries, nil
}

func (s *platformPriceBookManagerStub) UpsertEntry(_ context.Context, entry domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	s.upsertEntry = entry
	entry.Source = "manual"
	entry.ManuallyEdited = true
	return entry, nil
}

func (s *platformPriceBookManagerStub) DeleteEntry(_ context.Context, priceBookID, modelCode, capabilityType string) error {
	s.deleteEntryID = priceBookID
	s.deleteModel = modelCode
	s.deleteCapability = capabilityType
	return nil
}

type tenantPriceBookManagerStub struct {
	books               []domain.PriceBook
	entries             []domain.PriceBookEntry
	listTenantID        string
	createTenantID      string
	getTenantID         string
	updateTenantID      string
	deleteTenantID      string
	entriesTenantID     string
	upsertTenantID      string
	deleteEntryTenantID string
	cloneTenantID       string
	cloneSourceID       string
}

func (s *tenantPriceBookManagerStub) ListVisiblePriceBooks(_ context.Context, tenantID string) ([]domain.PriceBook, error) {
	s.listTenantID = tenantID
	return s.books, nil
}

func (s *tenantPriceBookManagerStub) CreateTenantPriceBook(_ context.Context, tenantID, name, description string) (domain.PriceBook, error) {
	s.createTenantID = tenantID
	return domain.PriceBook{ID: "tenant-created", OwnerType: domain.PriceBookOwnerTenant, OwnerTenantID: tenantID, Name: name, Description: description, Status: "active"}, nil
}

func (s *tenantPriceBookManagerStub) GetVisiblePriceBook(_ context.Context, tenantID, id string) (domain.PriceBook, error) {
	s.getTenantID = tenantID
	return domain.PriceBook{ID: id, OwnerType: domain.PriceBookOwnerTenant, OwnerTenantID: tenantID, Name: "Tenant", Status: "active"}, nil
}

func (s *tenantPriceBookManagerStub) UpdateTenantPriceBook(_ context.Context, tenantID, id, name, description, status string) (domain.PriceBook, error) {
	s.updateTenantID = tenantID
	return domain.PriceBook{ID: id, OwnerType: domain.PriceBookOwnerTenant, OwnerTenantID: tenantID, Name: name, Description: description, Status: status}, nil
}

func (s *tenantPriceBookManagerStub) DeleteTenantPriceBook(_ context.Context, tenantID, _ string) error {
	s.deleteTenantID = tenantID
	return nil
}

func (s *tenantPriceBookManagerStub) ListVisibleEntries(_ context.Context, tenantID, _ string) ([]domain.PriceBookEntry, error) {
	s.entriesTenantID = tenantID
	return s.entries, nil
}

func (s *tenantPriceBookManagerStub) UpsertTenantEntry(_ context.Context, tenantID string, entry domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	s.upsertTenantID = tenantID
	return entry, nil
}

func (s *tenantPriceBookManagerStub) DeleteTenantEntry(_ context.Context, tenantID, _, _, _ string) error {
	s.deleteEntryTenantID = tenantID
	return nil
}

func (s *tenantPriceBookManagerStub) CloneVisiblePriceBook(_ context.Context, tenantID, sourceID, name string) (domain.PriceBook, error) {
	s.cloneTenantID = tenantID
	s.cloneSourceID = sourceID
	return domain.PriceBook{ID: "tenant-clone", OwnerType: domain.PriceBookOwnerTenant, OwnerTenantID: tenantID, Name: name, Status: "active"}, nil
}

type priceBookSyncManagerStub struct {
	models             []billingcontrol.LiteLLMModelInfo
	searchQuery        string
	searchLimit        int
	platformImportID   string
	platformSyncID     string
	tenantImportID     string
	tenantImportBookID string
	tenantSyncID       string
	tenantSyncBookID   string
}

func (s *priceBookSyncManagerStub) SearchLiteLLM(_ context.Context, query string, limit int) ([]billingcontrol.LiteLLMModelInfo, error) {
	s.searchQuery = query
	s.searchLimit = limit
	return s.models, nil
}

func (s *priceBookSyncManagerStub) ImportFromLiteLLM(_ context.Context, priceBookID string) (billingcontrol.ImportResult, error) {
	s.platformImportID = priceBookID
	return billingcontrol.ImportResult{Fetched: 2, Imported: 1, Skipped: 1}, nil
}

func (s *priceBookSyncManagerStub) SyncCommonModels(_ context.Context, priceBookID string) (billingcontrol.SyncResult, error) {
	s.platformSyncID = priceBookID
	return billingcontrol.SyncResult{Synced: 1}, nil
}

func (s *priceBookSyncManagerStub) ImportTenantFromLiteLLM(_ context.Context, tenantID, priceBookID string) (billingcontrol.ImportResult, error) {
	s.tenantImportID = tenantID
	s.tenantImportBookID = priceBookID
	return billingcontrol.ImportResult{Fetched: 1, Imported: 1}, nil
}

func (s *priceBookSyncManagerStub) SyncTenantCommonModels(_ context.Context, tenantID, priceBookID string) (billingcontrol.SyncResult, error) {
	s.tenantSyncID = tenantID
	s.tenantSyncBookID = priceBookID
	return billingcontrol.SyncResult{Synced: 1}, nil
}

func TestPlatformPriceBookRoutesUseManagementAndSyncPorts(t *testing.T) {
	manager := &platformPriceBookManagerStub{books: []domain.PriceBook{{
		ID: "platform-1", OwnerType: domain.PriceBookOwnerPlatform, Name: "Platform", Status: "active",
	}}}
	syncer := &priceBookSyncManagerStub{models: []billingcontrol.LiteLLMModelInfo{{ModelCode: "gpt-test", CapabilityType: "chat"}}}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerPriceBooks(api, AIDeps{CatalogDeps: CatalogDeps{PlatformPriceBooks: manager, PriceBookSync: syncer}})

	listRecorder := performPriceBookRequest(router, http.MethodGet, "/api/v1/price-books", "")
	requirePriceBookStatus(t, listRecorder, http.StatusOK)
	var listResponse struct {
		Items []priceBookDTO `json:"items"`
		Total int            `json:"total"`
	}
	decodePriceBookResponse(t, listRecorder, &listResponse)
	if manager.listCalls != 1 || listResponse.Total != 1 || !listResponse.Items[0].Writable {
		t.Fatalf("platform list calls = %d, response = %#v", manager.listCalls, listResponse)
	}

	createRecorder := performPriceBookRequest(router, http.MethodPost, "/api/v1/price-books", `{"name":"Created","description":"Managed"}`)
	requirePriceBookStatus(t, createRecorder, http.StatusOK)
	if manager.createName != "Created" || manager.createDesc != "Managed" {
		t.Fatalf("create command = name %q description %q", manager.createName, manager.createDesc)
	}

	entryRecorder := performPriceBookRequest(router, http.MethodPut, "/api/v1/price-books/platform-1/entries/gpt-test", `{
		"capability_type":"chat","token_price_tiers":[{"up_to_input_tokens":null,"input_per_1m_usd":1,"output_per_1m_usd":2,"cache_write_per_1m_usd":0,"cache_read_per_1m_usd":0}]
	}`)
	requirePriceBookStatus(t, entryRecorder, http.StatusOK)
	if manager.upsertEntry.PriceBookID != "platform-1" || manager.upsertEntry.ModelCode != "gpt-test" || manager.upsertEntry.CapabilityType != "chat" {
		t.Fatalf("upsert entry = %#v", manager.upsertEntry)
	}

	searchRecorder := performPriceBookRequest(router, http.MethodGet, "/api/v1/price-books/litellm/models?q=gpt&limit=12", "")
	requirePriceBookStatus(t, searchRecorder, http.StatusOK)
	if syncer.searchQuery != "gpt" || syncer.searchLimit != 12 {
		t.Fatalf("search command = query %q limit %d", syncer.searchQuery, syncer.searchLimit)
	}

	importRecorder := performPriceBookRequest(router, http.MethodPost, "/api/v1/price-books/platform-1/import-litellm", "")
	requirePriceBookStatus(t, importRecorder, http.StatusOK)
	if syncer.platformImportID != "platform-1" {
		t.Fatalf("platform import ID = %q", syncer.platformImportID)
	}
}

func TestTenantPriceBookRoutesUseAuthenticatedTenantAcrossPorts(t *testing.T) {
	manager := &tenantPriceBookManagerStub{books: []domain.PriceBook{{
		ID: "tenant-book", OwnerType: domain.PriceBookOwnerTenant, OwnerTenantID: "tenant-1", Name: "Tenant", Status: "active",
	}}}
	syncer := &priceBookSyncManagerStub{}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerTenantPriceBooks(api, TenantCatalogHTTPDeps{TenantPriceBooks: manager, PriceBookSync: syncer})
	handler := withPriceBookClaims(router, &auth.Claims{TenantID: "tenant-1", UserID: "user-1"})

	listRecorder := performPriceBookRequest(handler, http.MethodGet, "/api/v1/tenants/me/price-books", "")
	requirePriceBookStatus(t, listRecorder, http.StatusOK)
	if manager.listTenantID != "tenant-1" {
		t.Fatalf("tenant list ID = %q", manager.listTenantID)
	}

	cloneRecorder := performPriceBookRequest(handler, http.MethodPost, "/api/v1/tenants/me/price-books/platform-1/clone", `{"name":"Tenant clone"}`)
	requirePriceBookStatus(t, cloneRecorder, http.StatusCreated)
	if manager.cloneTenantID != "tenant-1" || manager.cloneSourceID != "platform-1" {
		t.Fatalf("clone command = tenant %q source %q", manager.cloneTenantID, manager.cloneSourceID)
	}

	importRecorder := performPriceBookRequest(handler, http.MethodPost, "/api/v1/tenants/me/price-books/tenant-book/import-litellm", "")
	requirePriceBookStatus(t, importRecorder, http.StatusOK)
	if syncer.tenantImportID != "tenant-1" || syncer.tenantImportBookID != "tenant-book" {
		t.Fatalf("tenant import command = tenant %q book %q", syncer.tenantImportID, syncer.tenantImportBookID)
	}
}

func TestPriceBookRoutesRequireTheirOwnPorts(t *testing.T) {
	managerOnlyRouter, managerOnlyAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerPriceBooks(managerOnlyAPI, AIDeps{CatalogDeps: CatalogDeps{PlatformPriceBooks: &platformPriceBookManagerStub{}}})
	requirePriceBookStatus(t, performPriceBookRequest(managerOnlyRouter, http.MethodGet, "/api/v1/price-books", ""), http.StatusOK)
	requirePriceBookStatus(t, performPriceBookRequest(managerOnlyRouter, http.MethodGet, "/api/v1/price-books/litellm/models", ""), http.StatusServiceUnavailable)

	syncOnlyRouter, syncOnlyAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerPriceBooks(syncOnlyAPI, AIDeps{CatalogDeps: CatalogDeps{PriceBookSync: &priceBookSyncManagerStub{}}})
	requirePriceBookStatus(t, performPriceBookRequest(syncOnlyRouter, http.MethodGet, "/api/v1/price-books", ""), http.StatusServiceUnavailable)
	requirePriceBookStatus(t, performPriceBookRequest(syncOnlyRouter, http.MethodGet, "/api/v1/price-books/litellm/models", ""), http.StatusOK)

	tenantManagerRouter, tenantManagerAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerTenantPriceBooks(tenantManagerAPI, TenantCatalogHTTPDeps{TenantPriceBooks: &tenantPriceBookManagerStub{}})
	tenantManagerHandler := withPriceBookClaims(tenantManagerRouter, &auth.Claims{TenantID: "tenant-1"})
	requirePriceBookStatus(t, performPriceBookRequest(tenantManagerHandler, http.MethodGet, "/api/v1/tenants/me/price-books", ""), http.StatusOK)
	requirePriceBookStatus(t, performPriceBookRequest(tenantManagerHandler, http.MethodGet, "/api/v1/tenants/me/price-books/litellm/models", ""), http.StatusServiceUnavailable)

	tenantSyncRouter, tenantSyncAPI := server.New(server.Options{Title: "test", Version: "test"})
	registerTenantPriceBooks(tenantSyncAPI, TenantCatalogHTTPDeps{PriceBookSync: &priceBookSyncManagerStub{}})
	tenantSyncHandler := withPriceBookClaims(tenantSyncRouter, &auth.Claims{TenantID: "tenant-1"})
	requirePriceBookStatus(t, performPriceBookRequest(tenantSyncHandler, http.MethodGet, "/api/v1/tenants/me/price-books", ""), http.StatusServiceUnavailable)
	requirePriceBookStatus(t, performPriceBookRequest(tenantSyncHandler, http.MethodGet, "/api/v1/tenants/me/price-books/litellm/models", ""), http.StatusOK)
}

func withPriceBookClaims(handler http.Handler, claims *auth.Claims) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ctx := context.WithValue(request.Context(), authClaimsContextKey{}, claims)
		handler.ServeHTTP(w, request.WithContext(ctx))
	})
}

func performPriceBookRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func requirePriceBookStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, want, recorder.Body.String())
	}
}

func decodePriceBookResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

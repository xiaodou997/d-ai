package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestTenantCatalogRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tenants/me/available-models"},
		{http.MethodGet, "/api/v1/tenants/me/groups/group-1/effective-prices"},
		{http.MethodGet, "/api/v1/tenants/me/upstream-resources"},
		{http.MethodGet, "/api/v1/tenants/me/price-books"},
		{http.MethodPost, "/api/v1/tenants/me/price-books"},
		{http.MethodGet, "/api/v1/tenants/me/price-books/book-1"},
		{http.MethodPatch, "/api/v1/tenants/me/price-books/book-1"},
		{http.MethodDelete, "/api/v1/tenants/me/price-books/book-1"},
		{http.MethodGet, "/api/v1/tenants/me/price-books/book-1/entries"},
		{http.MethodPut, "/api/v1/tenants/me/price-books/book-1/entries/gpt-test"},
		{http.MethodDelete, "/api/v1/tenants/me/price-books/book-1/entries/gpt-test"},
		{http.MethodGet, "/api/v1/tenants/me/price-books/litellm/models"},
		{http.MethodPost, "/api/v1/tenants/me/price-books/book-1/import-litellm"},
		{http.MethodPost, "/api/v1/tenants/me/price-books/book-1/sync-common"},
		{http.MethodPost, "/api/v1/tenants/me/price-books/book-1/clone"},
		{http.MethodGet, "/api/v1/tenants/me/price-books/book-1/export"},
		{http.MethodPost, "/api/v1/tenants/me/price-books/import"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, route := range routes {
		recorder := performTenantCatalogRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI tenant catalog route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	catalogRouter, catalogAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterTenantCatalog(catalogAPI, TenantCatalogHTTPDeps{})
	for _, route := range routes {
		recorder := performTenantCatalogRequest(catalogRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent tenant catalog route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performTenantCatalogRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

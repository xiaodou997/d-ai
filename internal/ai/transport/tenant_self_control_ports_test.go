package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

func TestTenantSelfControlRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tenant-api-keys"},
		{http.MethodPost, "/api/v1/tenants/me/api-keys"},
		{http.MethodPatch, "/api/v1/tenants/me/api-keys/key-1"},
		{http.MethodPatch, "/api/v1/tenants/me/api-keys/key-1/status"},
		{http.MethodPost, "/api/v1/tenants/me/api-keys/key-1/reveal"},
		{http.MethodPost, "/api/v1/tenants/me/api-keys/key-1/rotate"},
		{http.MethodDelete, "/api/v1/tenants/me/api-keys/key-1"},
		{http.MethodGet, "/api/v1/tenants/me/users/user-1/limit-policies"},
		{http.MethodPut, "/api/v1/tenants/me/users/user-1/limit-policies"},
		{http.MethodGet, "/api/v1/tenants/me/api-key-limit-policies"},
		{http.MethodPut, "/api/v1/tenants/me/api-keys/key-1/limit-policies"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, CoreHTTPDeps{})
	for _, route := range routes {
		recorder := performTenantSelfControlRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI tenant self-control route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	controlRouter, controlAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterTenantSelfControl(controlAPI, TenantSelfControlHTTPDeps{})
	for _, route := range routes {
		recorder := performTenantSelfControlRequest(controlRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent tenant self-control route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performTenantSelfControlRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func TestTenantSelfAPIKeyStaticRouteWinsOverPlatformDynamicRoute(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	verifier := tenantSelfRouteTokenVerifier{}
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	recent := auth.NewRecentAuthService(client)
	if err := recent.Mark(context.Background(), "user-1", "test"); err != nil {
		t.Fatal(err)
	}
	RegisterTenantSelfControl(api, TenantSelfControlHTTPDeps{
		Auth: HTTPAuthDeps{TokenVerifier: verifier, RecentAuth: recent},
	})
	RegisterAPIKeyManagement(api, APIKeyManagementHTTPDeps{
		Auth: HTTPAuthDeps{TokenVerifier: verifier},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/me/api-keys", nil)
	request.Header.Set("Authorization", "Bearer tenant-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("tenant self API-key route status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

type tenantSelfRouteTokenVerifier struct{}

func (tenantSelfRouteTokenVerifier) ParseToken(context.Context, string) (*auth.Claims, error) {
	return &auth.Claims{
		PrincipalType: "user",
		TokenUse:      "access",
		SessionID:     "session-1",
		TenantID:      "tenant-1",
		UserID:        "user-1",
		UserType:      3,
	}, nil
}

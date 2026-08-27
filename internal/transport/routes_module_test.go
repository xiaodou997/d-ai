package transport

import (
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-chi/chi/v5"
)

func TestPlatformModuleRegistersPlatformSurface(t *testing.T) {
	_, api := humatest.New(t)
	Register(api,
		NewMetaModule("test", nil),
		NewPlatformIdentityModule(PlatformIdentityModuleDeps{}),
		NewPlatformAdminModule(PlatformAdminModuleDeps{}),
		NewPlatformBillingModule(PlatformBillingModuleDeps{}),
		NewPlatformOperationsModule(PlatformOperationsModuleDeps{}),
	)

	paths := api.OpenAPI().Paths
	for _, route := range []struct {
		path string
		name string
		get  bool
		post bool
	}{
		{path: "/api/auth/login", name: "auth login", post: true},
		{path: "/api/v1/info", name: "service metadata", get: true},
		{path: "/api/v1/jwt-keys", name: "JWT key management", get: true},
		{path: "/api/v1/tenants", name: "tenant management", get: true, post: true},
		{path: "/api/v1/system-admins", name: "admin accounts", get: true, post: true},
		{path: "/api/v1/payments/topup-orders", name: "payment", post: true},
		{path: "/api/v1/admin/modules", name: "operations", get: true},
	} {
		item, ok := paths[route.path]
		if !ok || item == nil {
			t.Fatalf("%s route %q was not registered", route.name, route.path)
		}
		if route.get && item.Get == nil {
			t.Fatalf("%s route %q has no GET operation", route.name, route.path)
		}
		if route.post && item.Post == nil {
			t.Fatalf("%s route %q has no POST operation", route.name, route.path)
		}
	}
}

func TestRawModuleRegistersOnlyRawRouteDependencies(t *testing.T) {
	mux := chi.NewRouter()
	RegisterRaw(mux, RawDeps{})

	routes := map[string]bool{}
	for _, route := range mux.Routes() {
		routes[route.Pattern] = true
	}
	for _, pattern := range []string{
		"/api/v1/payments/wechat/notify",
		"/api/v1/public/tenant-brands/{tenantId}/favicon",
	} {
		if !routes[pattern] {
			t.Fatalf("raw route %q was not registered", pattern)
		}
	}
}

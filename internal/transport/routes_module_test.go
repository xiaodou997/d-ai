package transport

import (
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

func TestPlatformModuleRegistersPlatformSurface(t *testing.T) {
	_, api := humatest.New(t)
	modules := []Module{
		platformModule{deps: Deps{}},
		platformOperationsModule{},
	}
	for _, module := range modules {
		module.Register(api)
	}

	paths := api.OpenAPI().Paths
	for _, route := range []struct {
		path string
		name string
		get  bool
		post bool
	}{
		{path: "/api/auth/login", name: "auth login", post: true},
		{path: "/api/v1/tenants", name: "tenant management", get: true, post: true},
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

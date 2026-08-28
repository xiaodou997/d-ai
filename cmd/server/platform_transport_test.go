package main

import (
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
	"go.uber.org/zap"

	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/transport"
)

func TestBuildPlatformTransportModulesKeepsCompositionRootOwnership(t *testing.T) {
	platform := &platformModules{}
	cfg := &config.Config{App: config.AppConfig{Env: "production"}, Legal: config.LegalConfig{TermsVersion: "terms-v2", PrivacyVersion: "privacy-v2"}}
	logger := zap.NewNop()

	modules := buildPlatformTransportModules("test-version", cfg, platform, transport.AIHTTPDeps{}, logger)
	if len(modules) != 6 {
		t.Fatalf("platform module count = %d, want 6", len(modules))
	}
	for i, module := range modules {
		if module == nil {
			t.Fatalf("platform module %d is nil", i)
		}
	}
}

func TestBuildPlatformTransportModulesHandlesNilCompositionInputs(t *testing.T) {
	modules := buildPlatformTransportModules("dev", nil, nil, transport.AIHTTPDeps{}, nil)
	if len(modules) != 6 {
		t.Fatalf("nil composition module count = %d, want 6", len(modules))
	}
	for i, module := range modules {
		if module == nil {
			t.Fatalf("nil composition module %d is nil", i)
		}
	}
}

func TestBuildPlatformTransportModulesRegisterEverySurface(t *testing.T) {
	_, api := humatest.New(t)
	cfg := &config.Config{App: config.AppConfig{Env: "test"}}
	modules := buildPlatformTransportModules("test-version", cfg, &platformModules{}, transport.AIHTTPDeps{}, zap.NewNop())
	transport.Register(api, modules...)

	paths := api.OpenAPI().Paths
	for _, route := range []struct {
		path string
		name string
		get  bool
		post bool
	}{
		{path: "/api/v1/info", name: "metadata", get: true},
		{path: "/api/auth/login", name: "identity", post: true},
		{path: "/api/v1/system-admins", name: "admin", get: true, post: true},
		{path: "/api/v1/payments/topup-orders", name: "billing", post: true},
		{path: "/api/v1/admin/modules", name: "operations", get: true},
		{path: "/api/v1/admin/notifications/send", name: "notifications", post: true},
		{path: "/api/v1/tenants/me/groups", name: "ai", get: true},
	} {
		item, ok := paths[route.path]
		if !ok || item == nil {
			t.Fatalf("%s route %q was not registered by composition root", route.name, route.path)
		}
		if route.get && item.Get == nil {
			t.Fatalf("%s route %q has no GET operation", route.name, route.path)
		}
		if route.post && item.Post == nil {
			t.Fatalf("%s route %q has no POST operation", route.name, route.path)
		}
	}
}

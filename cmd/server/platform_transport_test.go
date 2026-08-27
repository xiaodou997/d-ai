package main

import (
	"testing"

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

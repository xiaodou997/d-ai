package main

import (
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/config"
)

func TestBuildPlatformTransportDepsKeepsCompositionRootOwnership(t *testing.T) {
	jwt := &auth.JWTService{}
	blacklist := &auth.BlacklistService{}
	platform := &platformModules{JWT: jwt, Blacklist: blacklist}
	cfg := &config.Config{App: config.AppConfig{Env: "production"}, Legal: config.LegalConfig{TermsVersion: "terms-v2", PrivacyVersion: "privacy-v2"}}
	logger := zap.NewNop()

	deps := buildPlatformTransportDeps("test-version", cfg, platform, logger)
	if deps.Version != "test-version" || deps.Logger != logger {
		t.Fatalf("infrastructure deps = %+v", deps.InfrastructureDeps)
	}
	if !deps.SecureCookies || deps.Legal != cfg.Legal {
		t.Fatalf("portal deps = %+v", deps.PortalDeps)
	}
	if deps.JWT != jwt || deps.Blacklist != blacklist {
		t.Fatalf("identity auth deps = jwt:%p blacklist:%p", deps.JWT, deps.Blacklist)
	}
}

func TestBuildPlatformTransportDepsHandlesNilCompositionInputs(t *testing.T) {
	deps := buildPlatformTransportDeps("dev", nil, nil, nil)
	if deps.Version != "dev" || deps.Logger == nil || deps.SecureCookies {
		t.Fatalf("nil composition projection = %+v", deps)
	}
}

package postgres

import (
	"testing"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/routing"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
)

func TestDispatchSurfaceProtocolMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		surface  surface.ID
		protocol domain.UpstreamProtocol
	}{
		{surface.OpenAIChat, domain.ProtocolOpenAIChat},
		{surface.OpenAIResponses, domain.ProtocolOpenAIResponses},
		{surface.OpenAIEmbeddings, domain.ProtocolOpenAIEmbeddings},
		{surface.AnthropicMessages, domain.ProtocolAnthropicMessages},
		{surface.GeminiText, domain.ProtocolGeminiGenerate},
		{surface.GeminiEmbeddings, domain.ProtocolGeminiEmbeddings},
		{surface.OpenAIImages, domain.ProtocolOpenAIImages},
		{surface.GeminiImages, domain.ProtocolGeminiGenerate},
	}

	for _, tc := range cases {
		gotProtocol, err := dispatchProtocolFromSurface(tc.surface)
		if err != nil {
			t.Fatalf("dispatchProtocolFromSurface(%q): %v", tc.surface, err)
		}
		if gotProtocol != tc.protocol {
			t.Fatalf("dispatchProtocolFromSurface(%q) = %q, want %q", tc.surface, gotProtocol, tc.protocol)
		}
	}

	if _, err := dispatchProtocolFromSurface(surface.ID("unknown_surface")); err == nil {
		t.Fatal("dispatchProtocolFromSurface(unknown_surface): want validation error")
	}
}

func TestRoutingScopeBridge(t *testing.T) {
	t.Parallel()

	scopeKey, err := encodeRoutingScope(routing.ScopeTenant, "tenant-1")
	if err != nil {
		t.Fatalf("encodeRoutingScope tenant: %v", err)
	}
	if scopeKey != "tenant:tenant-1" {
		t.Fatalf("encodeRoutingScope tenant = %q", scopeKey)
	}

	scopeType, scopeID := decodeRoutingScope(scopeKey)
	if scopeType != routing.ScopeTenant || scopeID != "tenant-1" {
		t.Fatalf("decodeRoutingScope(%q) = (%q, %q)", scopeKey, scopeType, scopeID)
	}

	scopeKey, err = encodeRoutingScope(routing.ScopeGlobal, "")
	if err != nil {
		t.Fatalf("encodeRoutingScope global: %v", err)
	}
	if scopeKey != "global" {
		t.Fatalf("encodeRoutingScope global = %q", scopeKey)
	}

	scopeType, scopeID = decodeRoutingScope(scopeKey)
	if scopeType != routing.ScopeGlobal || scopeID != "global" {
		t.Fatalf("decodeRoutingScope(global) = (%q, %q)", scopeType, scopeID)
	}
}

func TestProviderFamilyBridge(t *testing.T) {
	t.Parallel()

	if got := providerFamilyToLegacy(string(catalog.ProviderFamilyAnthropic)); got != "anthropic" {
		t.Fatalf("providerFamilyToLegacy anthropic = %q", got)
	}
	if got := legacyProviderFamilyToCatalog("openai_compatible"); got != catalog.ProviderFamilyOpenAICompatible {
		t.Fatalf("legacyProviderFamilyToCatalog openai_compatible = %q", got)
	}
}

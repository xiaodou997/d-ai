package runtimecompat

import (
	"testing"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
)

func TestProtocolSurfaceRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		protocol domain.UpstreamProtocol
		surface  surface.ID
	}{
		{domain.ProtocolOpenAIChat, surface.OpenAIChat},
		{domain.ProtocolOpenAIResponses, surface.OpenAIResponses},
		{domain.ProtocolOpenAIEmbeddings, surface.OpenAIEmbeddings},
		{domain.ProtocolAnthropicMessages, surface.AnthropicMessages},
		{domain.ProtocolGeminiGenerate, surface.GeminiText},
		{domain.ProtocolGeminiEmbeddings, surface.GeminiEmbeddings},
		{domain.ProtocolOpenAIImages, surface.OpenAIImages},
	}

	for _, tc := range cases {
		gotSurface, err := ProtocolToSurface(tc.protocol)
		if err != nil {
			t.Fatalf("ProtocolToSurface(%q): %v", tc.protocol, err)
		}
		if gotSurface != tc.surface {
			t.Fatalf("ProtocolToSurface(%q) = %q, want %q", tc.protocol, gotSurface, tc.surface)
		}
		gotProtocol, err := SurfaceToProtocol(tc.surface)
		if err != nil {
			t.Fatalf("SurfaceToProtocol(%q): %v", tc.surface, err)
		}
		if gotProtocol != tc.protocol {
			t.Fatalf("SurfaceToProtocol(%q) = %q, want %q", tc.surface, gotProtocol, tc.protocol)
		}
	}
}

func TestSurfaceToProtocolRejectsGeminiImages(t *testing.T) {
	t.Parallel()

	if _, err := SurfaceToProtocol(surface.GeminiImages); err == nil {
		t.Fatal("SurfaceToProtocol(gemini_images): want error")
	}
}

func TestCapabilityAwareGeminiImageProjection(t *testing.T) {
	t.Parallel()

	surfaceID, err := ProtocolToSurfaceForCapability(domain.ProtocolGeminiGenerate, catalog.CapabilityImageGeneration)
	if err != nil {
		t.Fatalf("ProtocolToSurfaceForCapability(gemini_generate, image_generation): %v", err)
	}
	if surfaceID != surface.GeminiImages {
		t.Fatalf("surface = %q, want gemini_images", surfaceID)
	}

	protocolID, err := SurfaceToProtocolForCapability(surface.GeminiImages, catalog.CapabilityImageGeneration)
	if err != nil {
		t.Fatalf("SurfaceToProtocolForCapability(gemini_images, image_generation): %v", err)
	}
	if protocolID != domain.ProtocolGeminiGenerate {
		t.Fatalf("protocol = %q, want gemini_generate", protocolID)
	}
}

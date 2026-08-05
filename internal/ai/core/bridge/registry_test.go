package bridge

import (
	"testing"

	"xiaodou/dai/internal/ai/core/surface"
)

func TestRegistrySupportsAndDeduplicates(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(
		Definition{Kind: IRKindImage, Source: surface.OpenAIImages, Target: surface.GeminiImages},
		Definition{Kind: IRKindImage, Source: surface.OpenAIImages, Target: surface.GeminiImages},
	)
	if !reg.Supports(IRKindImage, surface.OpenAIImages, surface.GeminiImages) {
		t.Fatal("expected registry to support openai_images -> gemini_images")
	}
	if got := len(reg.Definitions()); got != 1 {
		t.Fatalf("definitions len = %d, want 1", got)
	}
}

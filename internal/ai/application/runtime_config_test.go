package application

import "testing"

func TestParseRuntimeConfigChat(t *testing.T) {
	cfg := ParseRuntimeConfig(AppTypeChatAgent, map[string]any{
		"chat": map[string]any{"creativity": "creative", "allow_attachments": true},
	})
	if cfg.Chat == nil {
		t.Fatal("chat config is nil")
	}
	if cfg.Image != nil {
		t.Fatal("image config should be nil for a chat app")
	}
	if cfg.Chat.Creativity != CreativityCreative {
		t.Fatalf("creativity = %q, want creative", cfg.Chat.Creativity)
	}
	if got := cfg.Chat.Temperature(); got != 1.0 {
		t.Fatalf("temperature = %v, want 1.0", got)
	}
	if cfg.Chat.MaxAttachments() != MaxAppAttachments {
		t.Fatalf("max attachments = %d, want %d", cfg.Chat.MaxAttachments(), MaxAppAttachments)
	}
}

func TestParseRuntimeConfigChatDefaults(t *testing.T) {
	// Empty/unknown values clamp to balanced with attachments off.
	cfg := ParseRuntimeConfig(AppTypeChatAgent, map[string]any{"chat": map[string]any{"creativity": "bogus"}})
	if cfg.Chat.Creativity != CreativityBalanced {
		t.Fatalf("creativity = %q, want balanced", cfg.Chat.Creativity)
	}
	if cfg.Chat.Temperature() != 0.6 {
		t.Fatalf("temperature = %v, want 0.6", cfg.Chat.Temperature())
	}
	if cfg.Chat.AllowAttachments {
		t.Fatal("attachments should default to false")
	}
	if cfg.Chat.MaxAttachments() != 0 {
		t.Fatalf("max attachments = %d, want 0", cfg.Chat.MaxAttachments())
	}
}

func TestParseRuntimeConfigImage(t *testing.T) {
	cfg := ParseRuntimeConfig(AppTypeImageGenerationAgent, map[string]any{
		"image": map[string]any{
			"resolution":                  "2k",
			"aspect_ratio":                "2:3",
			"default_output_count":        2,
			"max_output_count":            4,
			"allow_output_count_override": true,
		},
	})
	if cfg.Image == nil {
		t.Fatal("image config is nil")
	}
	if cfg.Image.Resolution != ImageResolution2K || cfg.Image.AspectRatio != "2:3" {
		t.Fatalf("image settings = %#v, want 2k / 2:3", cfg.Image)
	}
	if cfg.Image.DefaultOutputCount != 2 || cfg.Image.MaxOutputCount != 4 || !cfg.Image.AllowOutputCountOverride {
		t.Fatalf("image output policy = %#v", cfg.Image)
	}
	if got, ok := cfg.Image.ResolveOutputCount(3); !ok || got != 3 {
		t.Fatalf("ResolveOutputCount(3) = %d, %v", got, ok)
	}
	if _, ok := cfg.Image.ResolveOutputCount(5); ok {
		t.Fatal("ResolveOutputCount(5) should exceed the app maximum")
	}
	// Unknown resolution clamps to the platform default.
	bad := ParseRuntimeConfig(AppTypeImageEditAgent, map[string]any{"image": map[string]any{"resolution": "9x9"}})
	if bad.Image.Resolution != DefaultImageResolution {
		t.Fatalf("resolution = %q, want %q", bad.Image.Resolution, DefaultImageResolution)
	}
	if bad.Image.DefaultOutputCount != 1 || bad.Image.MaxOutputCount != 1 {
		t.Fatalf("default image output policy = %#v", bad.Image)
	}
	if IsValidResolution("3840x2160") || IsValidResolution("2160x3840") {
		t.Fatal("4K resolutions must not be valid app settings")
	}
	legacy4K := ParseRuntimeConfig(AppTypeImageEditAgent, map[string]any{"image": map[string]any{"resolution": "3840x2160"}})
	if legacy4K.Image.Resolution != DefaultImageResolution {
		t.Fatalf("legacy 4K resolution = %q, want %q", legacy4K.Image.Resolution, DefaultImageResolution)
	}
}

func TestImageRuntimeConfigLockedCount(t *testing.T) {
	cfg := ParseRuntimeConfig(AppTypeImageGenerationAgent, map[string]any{
		"image": map[string]any{"default_output_count": 2, "max_output_count": 4},
	}).Image
	if got, ok := cfg.ResolveOutputCount(0); !ok || got != 2 {
		t.Fatalf("omitted count = %d, %v", got, ok)
	}
	if _, ok := cfg.ResolveOutputCount(3); ok {
		t.Fatal("locked app should reject a caller override")
	}
}

func TestRuntimeConfigToStored(t *testing.T) {
	stored := RuntimeConfig{Chat: &ChatRuntimeConfig{Creativity: CreativityPrecise, AllowAttachments: true}}.ToStored()
	chat, ok := stored["chat"].(map[string]any)
	if !ok {
		t.Fatalf("stored chat = %#v", stored["chat"])
	}
	if chat["creativity"] != CreativityPrecise || chat["allow_attachments"] != true {
		t.Fatalf("stored chat = %#v", chat)
	}
	// Round-trip through the parser yields the same values.
	reparsed := ParseRuntimeConfig(AppTypeChatAgent, stored)
	if reparsed.Chat.Creativity != CreativityPrecise || !reparsed.Chat.AllowAttachments {
		t.Fatalf("round-trip lost data: %#v", reparsed.Chat)
	}
}

func TestRenderTemplate(t *testing.T) {
	got := RenderTemplate("Hi {{name}} / {{ role }} / {{missing}}", map[string]string{"name": "A", "role": "B"})
	if got != "Hi A / B / {{missing}}" {
		t.Fatalf("RenderTemplate = %q", got)
	}
}

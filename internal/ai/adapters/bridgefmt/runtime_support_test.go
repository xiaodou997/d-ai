package bridgefmt

import (
	"testing"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
)

func TestRuntimeSupportPreferenceForChat(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime()
	cases := []struct {
		name         string
		client       surface.ID
		provider     surface.ID
		stream       bool
		wantOK       bool
		wantBucket   int
		wantPriority int
	}{
		{
			name:         "anthropic to openai chat",
			client:       surface.AnthropicMessages,
			provider:     surface.OpenAIChat,
			wantOK:       true,
			wantBucket:   3,
			wantPriority: 0,
		},
		{
			name:         "openai chat to responses",
			client:       surface.OpenAIChat,
			provider:     surface.OpenAIResponses,
			wantOK:       true,
			wantBucket:   2,
			wantPriority: 1,
		},
		{
			name:     "image surface is not a chat bridge",
			client:   surface.OpenAIImages,
			provider: surface.GeminiText,
			wantOK:   false,
		},
		{
			name:         "streaming chat bridge stays supported",
			client:       surface.AnthropicMessages,
			provider:     surface.OpenAIResponses,
			stream:       true,
			wantOK:       true,
			wantBucket:   3,
			wantPriority: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotBucket, gotPriority, ok := runtime.PreferenceForCapability(catalog.CapabilityChat, tc.client, tc.provider, tc.stream)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if gotBucket != tc.wantBucket || gotPriority != tc.wantPriority {
				t.Fatalf("bucket/priority = %d/%d, want %d/%d", gotBucket, gotPriority, tc.wantBucket, tc.wantPriority)
			}
		})
	}
}

func TestRuntimeSupportPreferenceForImageCapability(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime()
	bucket, priority, ok := runtime.PreferenceForCapability(
		catalog.CapabilityImageGeneration,
		surface.OpenAIImages,
		surface.GeminiImages,
		false,
	)
	if !ok {
		t.Fatal("expected openai_images -> gemini_images bridge to be supported")
	}
	if bucket != 1 || priority != 0 {
		t.Fatalf("bucket/priority = %d/%d, want 1/0", bucket, priority)
	}

	if _, _, ok := runtime.PreferenceForCapability(
		catalog.CapabilityImageGeneration,
		surface.OpenAIImages,
		surface.GeminiImages,
		true,
	); !ok {
		t.Fatal("expected streaming openai_images -> gemini_images bridge to be supported")
	}
}

func TestRuntimeSupportPreferenceForEmbeddingCapability(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime()
	bucket, priority, ok := runtime.PreferenceForCapability(
		catalog.CapabilityEmbedding,
		surface.OpenAIEmbeddings,
		surface.GeminiEmbeddings,
		false,
	)
	if !ok {
		t.Fatal("expected openai_embeddings -> gemini_embeddings bridge to be supported")
	}
	if bucket != 1 || priority != 0 {
		t.Fatalf("bucket/priority = %d/%d, want 1/0", bucket, priority)
	}

	bucket, priority, ok = runtime.PreferenceForCapability(
		catalog.CapabilityEmbedding,
		surface.GeminiEmbeddings,
		surface.OpenAIEmbeddings,
		false,
	)
	if !ok {
		t.Fatal("expected gemini_embeddings -> openai_embeddings bridge to be supported")
	}
	if bucket != 1 || priority != 1 {
		t.Fatalf("bucket/priority = %d/%d, want 1/1", bucket, priority)
	}

	if _, _, ok := runtime.PreferenceForCapability(
		catalog.CapabilityEmbedding,
		surface.OpenAIEmbeddings,
		surface.GeminiEmbeddings,
		true,
	); ok {
		t.Fatal("expected streaming embeddings bridge to stay unsupported")
	}
}

func TestRuntimeNeedsBridge(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime()
	if runtime.NeedsBridge(surface.OpenAIChat, surface.OpenAIChat) {
		t.Fatal("same-surface passthrough should not require a bridge")
	}
	if !runtime.NeedsBridge(surface.OpenAIChat, surface.AnthropicMessages) {
		t.Fatal("cross-surface call should require a bridge")
	}
}

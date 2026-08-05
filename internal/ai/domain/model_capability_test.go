package domain

import "testing"

func TestInferModelCapabilityAndProtocol(t *testing.T) {
	cases := []struct {
		name         string
		model        string
		endpoint     string
		wantCap      CapabilityType
		wantProtocol UpstreamProtocol
	}{
		// Gemini 原生生图 —— 之前被误判为 chat 的核心 bug。
		{"gemini flash image", "gemini-3.1-flash-image", "gemini", CapabilityImage, ProtocolGeminiGenerate},
		{"gemini flash image preview", "gemini-3.1-flash-image-preview", "gemini", CapabilityImage, ProtocolGeminiGenerate},
		{"gemini pro image", "gemini-3-pro-image", "gemini", CapabilityImage, ProtocolGeminiGenerate},
		{"gemini 2.5 flash image", "gemini-2.5-flash-image", "gemini", CapabilityImage, ProtocolGeminiGenerate},
		{"nano-banana alias", "nano-banana-2", "gemini", CapabilityImage, ProtocolGeminiGenerate},
		// Gemini 文本仍是 chat。
		{"gemini flash chat", "gemini-2.5-flash", "gemini", CapabilityChat, ProtocolGeminiGenerate},
		{"gemini pro chat", "gemini-3-pro", "gemini", CapabilityChat, ProtocolGeminiGenerate},
		{"gemma chat", "gemma-3-27b", "gemini", CapabilityChat, ProtocolGeminiGenerate},
		// OpenAI 系生图。
		{"gpt-image", "gpt-image-2", "", CapabilityImage, ProtocolOpenAIImages},
		{"dall-e", "dall-e-3", "", CapabilityImage, ProtocolOpenAIImages},
		{"imagen", "imagen-3.0", "", CapabilityImage, ProtocolOpenAIImages},
		// Embedding。
		{"openai embedding", "text-embedding-3-large", "", CapabilityEmbedding, ProtocolOpenAIEmbeddings},
		{"gemini embedding", "gemini-embedding-001", "gemini", CapabilityEmbedding, ProtocolGeminiEmbeddings},
		// 其它能力。
		{"whisper stt", "whisper-1", "", CapabilityAudioSTT, ProtocolOpenAIChat},
		{"tts", "tts-1-hd", "", CapabilityAudioTTS, ProtocolOpenAIChat},
		{"rerank", "bge-rerank-v2", "", CapabilityRerank, ProtocolOpenAIChat},
		// Chat 家族。
		{"claude anthropic", "claude-opus-4", "anthropic", CapabilityChat, ProtocolAnthropicMessages},
		{"claude via openai", "claude-opus-4", "", CapabilityChat, ProtocolOpenAIResponses},
		{"gpt chat default", "gpt-4o", "", CapabilityChat, ProtocolOpenAIResponses},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCap, gotProto := InferModelCapabilityAndProtocol(tc.model, tc.endpoint)
			if gotCap != tc.wantCap {
				t.Errorf("capability: got %q want %q", gotCap, tc.wantCap)
			}
			if gotProto != tc.wantProtocol {
				t.Errorf("protocol: got %q want %q", gotProto, tc.wantProtocol)
			}
		})
	}
}

func TestIsGeminiImageModel(t *testing.T) {
	yes := []string{"gemini-3.1-flash-image", "gemini-3-pro-image-preview", "nano-banana-2"}
	no := []string{"gemini-2.5-flash", "gpt-image-2", "gpt-4o", "dall-e-3"}
	for _, m := range yes {
		if !IsGeminiImageModel(m) {
			t.Errorf("IsGeminiImageModel(%q) = false, want true", m)
		}
	}
	for _, m := range no {
		if IsGeminiImageModel(m) {
			t.Errorf("IsGeminiImageModel(%q) = true, want false", m)
		}
	}
}

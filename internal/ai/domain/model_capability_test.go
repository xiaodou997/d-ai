package domain

import "testing"

func TestInferModelCapability(t *testing.T) {
	cases := []struct {
		name    string
		model   string
		wantCap CapabilityType
	}{
		{"gemini flash image", "gemini-3.1-flash-image", CapabilityImage},
		{"nano-banana alias", "nano-banana-2", CapabilityImage},
		{"gemini flash chat", "gemini-2.5-flash", CapabilityChat},
		{"gpt-image", "gpt-image-2", CapabilityImage},
		{"dall-e", "dall-e-3", CapabilityImage},
		{"openai embedding", "text-embedding-3-large", CapabilityEmbedding},
		{"gemini embedding", "gemini-embedding-001", CapabilityEmbedding},
		{"whisper stt", "whisper-1", CapabilityAudioSTT},
		{"tts", "tts-1-hd", CapabilityAudioTTS},
		{"rerank", "bge-rerank-v2", CapabilityRerank},
		{"gpt chat default", "gpt-4o", CapabilityChat},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InferModelCapability(tc.model); got != tc.wantCap {
				t.Errorf("capability: got %q want %q", got, tc.wantCap)
			}
		})
	}
}

func TestProtocolSupportsCapability(t *testing.T) {
	tests := []struct {
		protocol   UpstreamProtocol
		capability CapabilityType
		want       bool
	}{
		{ProtocolOpenAIResponses, CapabilityChat, true},
		{ProtocolGeminiGenerate, CapabilityImage, true},
		{ProtocolOpenAIResponses, CapabilityEmbedding, false},
		{ProtocolOpenAIEmbeddings, CapabilityEmbedding, true},
		{ProtocolOpenAIImages, CapabilityChat, false},
	}
	for _, test := range tests {
		if got := ProtocolSupportsCapability(test.protocol, test.capability); got != test.want {
			t.Fatalf("ProtocolSupportsCapability(%q, %q) = %v, want %v", test.protocol, test.capability, got, test.want)
		}
	}
}

func TestIsGeminiImageModel(t *testing.T) {
	yes := []string{"gemini-3.1-flash-image", "gemini-3-pro-image-preview", "nano-banana-2"}
	no := []string{"gemini-2.5-flash", "gpt-image-2", "gpt-4o", "dall-e-3"}
	for _, model := range yes {
		if !IsGeminiImageModel(model) {
			t.Errorf("IsGeminiImageModel(%q) = false, want true", model)
		}
	}
	for _, model := range no {
		if IsGeminiImageModel(model) {
			t.Errorf("IsGeminiImageModel(%q) = true, want false", model)
		}
	}
}

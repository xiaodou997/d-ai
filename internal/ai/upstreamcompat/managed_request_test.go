package upstreamcompat

import (
	"encoding/json"
	"strings"
	"testing"
	"xiaodou/dai/internal/ai/domain"
)

func TestManagedResponsesPreservesConversationAndOutputRoles(t *testing.T) {
	messages := []Message{{Role: "system", Content: "Use concise Chinese."}, {Role: "user", Content: "first question"}, {Role: "assistant", Content: "previous answer"}, {Role: "user", Content: "follow-up"}}
	raw, err := BuildChatRequest(domain.ProtocolOpenAIResponses, "model", messages, true, 4096)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	json.Unmarshal(raw, &body)
	if body["instructions"] != "Use concise Chinese." || body["store"] != false || body["stream"] != true || body["max_output_tokens"] != float64(4096) {
		t.Fatalf("body=%s", raw)
	}
	input := body["input"].([]any)
	if len(input) != 3 {
		t.Fatal("lost conversation or leaked system instruction")
	}
	assistant := input[1].(map[string]any)
	part := assistant["content"].([]any)[0].(map[string]any)
	if assistant["role"] != "assistant" || part["type"] != "output_text" || part["text"] != "previous answer" {
		t.Fatalf("assistant=%v", assistant)
	}
}

func TestManagedChatDefaultsUseNativeSystemFields(t *testing.T) {
	for _, protocol := range []domain.UpstreamProtocol{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses, domain.ProtocolAnthropicMessages, domain.ProtocolGeminiGenerate} {
		raw, err := BuildChatRequest(protocol, "model", []Message{{Role: "user", Content: "hello"}}, true, 0)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		json.Unmarshal(raw, &doc)
		if strings.Contains(string(raw), "SeenOrAdd") {
			t.Fatal("test task leaked into real conversation")
		}
		switch protocol {
		case domain.ProtocolOpenAIChat:
			messages := doc["messages"].([]any)
			if len(messages) != 2 || messages[0].(map[string]any)["content"] != DefaultInstructions {
				t.Fatalf("messages=%v", messages)
			}
		case domain.ProtocolOpenAIResponses:
			if doc["instructions"] != DefaultInstructions || doc["max_output_tokens"] != nil {
				t.Fatal("Responses defaults")
			}
		case domain.ProtocolAnthropicMessages:
			if doc["system"] != DefaultInstructions || doc["max_tokens"] != float64(2048) {
				t.Fatal("Anthropic defaults")
			}
		case domain.ProtocolGeminiGenerate:
			if doc["systemInstruction"] == nil || len(doc["contents"].([]any)) != 1 {
				t.Fatal("Gemini defaults")
			}
		}
	}
}

func TestManagedImagesPreserveOptionsWithoutChatFields(t *testing.T) {
	compression := 75
	raw, err := BuildImageRequest(domain.ProtocolOpenAIImages, "image-model", "用户的图片描述", ImageOptions{N: 2, Stream: true, Size: "1536x1024", ResponseFormat: "b64_json", Background: "transparent", OutputFormat: "webp", Moderation: "low", OutputCompression: &compression})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	json.Unmarshal(raw, &doc)
	if doc["prompt"] != "用户的图片描述" || doc["n"] != float64(2) || doc["stream"] != true || doc["output_compression"] != float64(75) || doc["moderation"] != "low" || doc["size"] != "1536x1024" {
		t.Fatalf("body=%s", raw)
	}
	for _, key := range []string{"instructions", "messages", "input", "system", "store"} {
		if _, ok := doc[key]; ok {
			t.Fatalf("image contains chat field %s", key)
		}
	}
}

func TestGeminiWireBodyUsesURLMetadataAndPreservesImageContents(t *testing.T) {
	original := []byte(`{"model":"local-model","stream":true,"contents":[{"role":"user","parts":[{"text":"edit this"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"]}}`)
	got, err := ApplyRequestBodyTransform(&domain.RouteCandidate{Protocol: domain.ProtocolGeminiGenerate}, RequestMeta{}, original)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	json.Unmarshal(got, &doc)
	if doc["model"] != nil || doc["stream"] != nil || !strings.Contains(string(doc["contents"]), "aW1hZ2U=") || doc["generationConfig"] == nil {
		t.Fatalf("body=%s", got)
	}
}

package console

import (
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestBuildConsoleProtocolBodyOmitsTemperature(t *testing.T) {
	protocols := []domain.UpstreamProtocol{
		domain.ProtocolOpenAIChat,
		domain.ProtocolOpenAIResponses,
		domain.ProtocolAnthropicMessages,
		domain.ProtocolGeminiGenerate,
	}

	for _, protocol := range protocols {
		t.Run(string(protocol), func(t *testing.T) {
			body, _, err := buildConsoleProtocolBody(protocol, "model-1", []consoleChatMessage{{Role: "user", Content: "hello"}}, 128)
			if err != nil {
				t.Fatalf("buildConsoleProtocolBody() error = %v", err)
			}

			var payload any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if containsTemperature(payload) {
				t.Fatalf("payload must not contain temperature: %s", body)
			}
		})
	}
}

func containsTemperature(value any) bool {
	switch item := value.(type) {
	case map[string]any:
		for key, nested := range item {
			if key == "temperature" {
				return true
			}
			if containsTemperature(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range item {
			if containsTemperature(nested) {
				return true
			}
		}
	}
	return false
}

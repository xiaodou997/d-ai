package console

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/workspace"
)

type chatPersistenceStub struct {
	routeCalls int
	lastRoute  workspace.ChatMessageRouteUpdate
}

func (s *chatPersistenceStub) CreateChatMessage(context.Context, workspace.Owner, string, workspace.ChatMessageWriteInput) (string, error) {
	return "message-1", nil
}

func (*chatPersistenceStub) UpdateChatMessageContent(context.Context, workspace.Owner, string, string) error {
	return nil
}

func (s *chatPersistenceStub) UpdateChatMessageRoute(_ context.Context, _ workspace.Owner, _ string, input workspace.ChatMessageRouteUpdate) error {
	s.routeCalls++
	s.lastRoute = input
	return nil
}

func (*chatPersistenceStub) UpdateChatSessionRoute(context.Context, workspace.Owner, string, surface.ID, string) error {
	return nil
}

func TestConsoleChatStreamPersistenceCloseIsIdempotent(t *testing.T) {
	writer := &chatPersistenceStub{}
	console := &Console{logger: zap.NewNop(), workspaceMessages: writer}
	capture := &captureResponseWriter{ResponseWriter: httptest.NewRecorder()}
	persistence := console.startConsoleChatStreamPersistence(
		context.Background(), workspace.Owner{UserID: "user-1"}, "session-1", domain.ProtocolOpenAIChat, capture,
	)
	if persistence == nil {
		t.Fatal("expected stream persistence")
	}

	persistence.close("", true)
	persistence.close("route-after-close", false)
	if writer.routeCalls != 1 {
		t.Fatalf("route update calls = %d, want 1", writer.routeCalls)
	}
	if writer.lastRoute.StreamStatus != workspace.ChatStreamStatusInterrupted {
		t.Fatalf("stream status = %q, want interrupted", writer.lastRoute.StreamStatus)
	}
}

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

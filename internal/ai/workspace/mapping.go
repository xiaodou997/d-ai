package workspace

import (
	"strings"

	"xiaodou/dai/internal/ai/core/surface"
)

func SessionTypeFromTargetKind(kind ThreadTargetKind) string {
	if kind == ThreadTargetApp {
		return "app"
	}
	return "model"
}

func TargetKindFromSessionType(targetType string) ThreadTargetKind {
	if strings.TrimSpace(targetType) == "app" {
		return ThreadTargetApp
	}
	return ThreadTargetModel
}

func SurfaceFromProtocol(protocol string) surface.ID {
	switch strings.TrimSpace(protocol) {
	case "openai_chat":
		return surface.OpenAIChat
	case "openai_responses":
		return surface.OpenAIResponses
	case "anthropic_messages":
		return surface.AnthropicMessages
	case "gemini_generate":
		return surface.GeminiText
	default:
		return ""
	}
}

func ProtocolFromSurface(id surface.ID) string {
	switch id {
	case surface.OpenAIChat:
		return "openai_chat"
	case surface.OpenAIResponses:
		return "openai_responses"
	case surface.AnthropicMessages:
		return "anthropic_messages"
	case surface.GeminiText:
		return "gemini_generate"
	default:
		return ""
	}
}

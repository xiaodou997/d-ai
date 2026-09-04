package serving

import (
	"encoding/json"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

// streamChunkStartsToken identifies the first semantic output event. Protocol
// preambles, usage-only frames and terminal markers are deliberately ignored.
// This is the TTFT milestone; first response byte is tracked independently.
func streamChunkStartsToken(data, eventType string, protocol domain.UpstreamProtocol) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" {
		return false
	}
	var payload map[string]any
	if json.Unmarshal([]byte(trimmed), &payload) != nil {
		return false
	}
	if eventType == "" {
		if value, ok := payload["type"].(string); ok {
			eventType = value
		}
	}
	if strings.Contains(strings.ToLower(eventType), "usage") || strings.Contains(strings.ToLower(eventType), "completed") || strings.Contains(strings.ToLower(eventType), "done") {
		return false
	}
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		return eventType == "content_block_delta" || eventType == "content_block_start" || hasNestedText(payload, "text", "input_json")
	case domain.ProtocolOpenAIResponses:
		return strings.Contains(eventType, "output_text.delta") || strings.Contains(eventType, "reasoning_summary_text.delta") || strings.Contains(eventType, "function_call_arguments.delta") || hasNestedText(payload, "delta", "text", "output_text")
	case domain.ProtocolGeminiGenerate:
		return hasNestedText(payload, "text", "functionCall", "function_call")
	default:
		// OpenAI Chat Completions and compatible streams: usage-only chunks have
		// no choices; content/tool-call deltas are the first token milestone.
		choices, ok := payload["choices"].([]any)
		if !ok || len(choices) == 0 {
			return false
		}
		return hasNestedText(payload, "content", "reasoning_content", "tool_calls", "function_call")
	}
}

func hasNestedText(value any, keys ...string) bool {
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	var walk func(any) bool
	walk = func(node any) bool {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if _, ok := allowed[key]; ok {
					switch v := child.(type) {
					case string:
						if strings.TrimSpace(v) != "" {
							return true
						}
					case []any:
						if len(v) > 0 {
							return true
						}
					case map[string]any:
						if len(v) > 0 && walk(v) {
							return true
						}
					}
				}
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range value {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}

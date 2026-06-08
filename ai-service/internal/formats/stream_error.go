package formats

import (
	"encoding/json"

	"xiaodou/unihub/ai-service/internal/domain"
)

// StreamErrorFrame builds a protocol-appropriate SSE error event. When a
// streaming response breaks AFTER the gateway has already committed 200 OK to
// the client, the gateway forwards this frame so the client SDK receives an
// explicit, parseable failure instead of a silently truncated stream.
//
// The frame is fully terminated (trailing blank line) and ready to write
// verbatim to the client.
func StreamErrorFrame(protocol domain.UpstreamProtocol, code, message string) []byte {
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		data, _ := json.Marshal(map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    "api_error",
				"message": message,
			},
		})
		return []byte("event: error\ndata: " + string(data) + "\n\n")

	case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
		data, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"code":    502,
				"message": message,
				"status":  "UNAVAILABLE",
			},
		})
		return []byte("data: " + string(data) + "\n\n")

	default: // OpenAI Chat / Responses / Completions / Embeddings / Images
		data, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": message,
				"type":    "api_error",
				"code":    code,
			},
		})
		return []byte("data: " + string(data) + "\n\n")
	}
}

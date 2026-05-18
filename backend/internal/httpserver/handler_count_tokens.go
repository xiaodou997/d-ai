package httpserver

import (
	"encoding/json"
	"io"
	"net/http"

	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/formats/claude"
)

// handleCountTokens implements Anthropic's POST /v1/messages/count_tokens.
//
// The request body shape matches Messages (model + messages + system + tools),
// but no upstream call is made: we return a character-based local estimate.
// This is intentional — Claude Code uses count_tokens to plan context window
// utilisation, not for billing, so an estimate is adequate. A future enhancement
// can route the call to the real Anthropic upstream when one is available.
//
// Response shape (Anthropic-compatible):
//
//	{"input_tokens": 1234}
func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 32*1024*1024))
	if err != nil {
		writeRuntimeErrorByProtocol(w, domain.ProtocolAnthropicMessages,
			http.StatusBadRequest, "Failed to read request body.", "body_read_error")
		return
	}

	var req claude.MessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRuntimeErrorByProtocol(w, domain.ProtocolAnthropicMessages,
			http.StatusBadRequest, "Invalid request body: "+err.Error(), "invalid_body")
		return
	}

	estimate := estimateInputTokens(&req)

	writeJSON(w, http.StatusOK, map[string]int{"input_tokens": estimate})
}

// estimateInputTokens returns a conservative byte-based token count using the
// project-wide bytes/3 heuristic. Sums system prompt + all message text +
// tool schemas.
func estimateInputTokens(req *claude.MessagesRequest) int {
	bytes := len(req.System)

	for _, m := range req.Messages {
		// Content can be a plain string or an array of blocks. Treat the raw
		// JSON length as a safe upper bound for text-heavy payloads.
		bytes += len(m.Content)
	}

	for _, t := range req.Tools {
		bytes += len(t.Name) + len(t.Description) + len(t.InputSchema)
	}

	if bytes == 0 {
		return 0
	}
	// bytes / 3, rounded up — matches fillEstimatedUsage in serving/execute.go.
	return (bytes + 2) / 3
}

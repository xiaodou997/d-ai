package gateway

import (
	"encoding/json"
	"io"
	"net/http"

	"xiaodou/unihub/ai-service/internal/domain"
)

// handleCountTokens implements Anthropic's POST /v1/messages/count_tokens.
//
// We return a character-based local estimate instead of calling upstream:
// Claude Code uses count_tokens to plan context-window utilisation, not for
// billing, so an estimate is adequate. The response shape mirrors Anthropic's:
//
//	{"input_tokens": 1234}
func (s *Gateway) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 32*1024*1024))
	if err != nil {
		WriteRuntimeErrorByProtocol(w, domain.ProtocolAnthropicMessages,
			http.StatusBadRequest, "Failed to read request body.", "body_read_error")
		return
	}

	// Use json.RawMessage to count bytes verbatim without re-modelling
	// the entire Anthropic request schema. This is robust to schema drift
	// (new fields just contribute their bytes to the estimate, which is the
	// desired behaviour for a byte-based heuristic).
	var req struct {
		System   json.RawMessage   `json:"system"`
		Messages []json.RawMessage `json:"messages"`
		Tools    []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		WriteRuntimeErrorByProtocol(w, domain.ProtocolAnthropicMessages,
			http.StatusBadRequest, "Invalid request body: "+err.Error(), "invalid_body")
		return
	}

	totalBytes := len(req.System)
	for _, m := range req.Messages {
		totalBytes += len(m)
	}
	for _, t := range req.Tools {
		totalBytes += len(t)
	}

	estimate := 0
	if totalBytes > 0 {
		// bytes / 3, rounded up — matches fillEstimatedUsage in serving/execute.go.
		estimate = (totalBytes + 2) / 3
	}

	writeJSON(w, http.StatusOK, map[string]int{"input_tokens": estimate})
}

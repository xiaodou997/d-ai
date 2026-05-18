package serving

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/formats/canonical"
	"uni-ai-api/backend/internal/formats/claude"
	"uni-ai-api/backend/internal/formats/openai"
	"uni-ai-api/backend/internal/formats/openai/responses"
)

// Relay is the protocol-specific writer that delivers an upstream response to
// the client. There is one implementation per supported client protocol.
//
// The execution layer parses the upstream response into canonical form and
// hands it to the Relay, which serialises it back into the client's chosen
// wire format. This decouples upstream protocol from client protocol so that
// any upstream can serve any client.
type Relay interface {
	// WriteSync writes a non-streaming response.
	WriteSync(w http.ResponseWriter, resp *canonical.ChatResponse) error
	// NewStream returns a stateful sink for incremental chunks.
	// Callers MUST invoke Start before any Push and Finish after the last Push.
	NewStream(w http.ResponseWriter, flush func(), responseID, model string) StreamSink
}

// StreamSink is a stateful per-request encoder for streaming output. Each
// implementation maps canonical StreamChunks onto its client protocol's SSE
// events.
type StreamSink interface {
	Start(promptTokens int) error
	Push(chunk *canonical.StreamChunk) error
	Finish() error
	// Error emits a protocol-appropriate error frame mid-stream. Use when the
	// upstream fails after the response header has been committed.
	Error(code, message string) error
}

// RelayFor returns the Relay matching the given client protocol. Falls back
// to OpenAI Chat for any unrecognised protocol so the gateway always returns
// something parseable.
func RelayFor(proto domain.UpstreamProtocol) Relay {
	switch proto {
	case domain.ProtocolAnthropicMessages:
		return anthropicRelay{}
	case domain.ProtocolOpenAIResponses:
		return openaiResponsesRelay{}
	default:
		return openaiChatRelay{}
	}
}

// ============================================================================
// OpenAI Chat Completions
// ============================================================================

type openaiChatRelay struct{}

func (openaiChatRelay) WriteSync(w http.ResponseWriter, resp *canonical.ChatResponse) error {
	body, err := json.Marshal(openai.ChatResponseFromCanonical(resp))
	if err != nil {
		return fmt.Errorf("marshal openai chat response: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return nil
}

func (openaiChatRelay) NewStream(w http.ResponseWriter, flush func(), responseID, model string) StreamSink {
	writeStreamHeaders(w)
	return &openaiChatSink{w: w, flush: flush, id: responseID, model: model}
}

type openaiChatSink struct {
	w        io.Writer
	flush    func()
	id       string
	model    string
	started  bool
	finished bool
}

func (s *openaiChatSink) Start(_ int) error { s.started = true; return nil }

func (s *openaiChatSink) Push(chunk *canonical.StreamChunk) error {
	if chunk == nil || s.finished {
		return nil
	}
	// Default identifiers when upstream doesn't supply them.
	if chunk.ID == "" {
		chunk.ID = s.id
	}
	if chunk.Model == "" {
		chunk.Model = s.model
	}
	if chunk.Object == "" {
		chunk.Object = "chat.completion.chunk"
	}
	body, err := json.Marshal(openai.StreamChunkFromCanonical(chunk))
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", body); err != nil {
		return err
	}
	if s.flush != nil {
		s.flush()
	}
	return nil
}

func (s *openaiChatSink) Finish() error {
	if s.finished {
		return nil
	}
	s.finished = true
	if _, err := fmt.Fprint(s.w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	if s.flush != nil {
		s.flush()
	}
	return nil
}

func (s *openaiChatSink) Error(code, message string) error {
	if s.finished {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    "server_error",
			"code":    code,
		},
	})
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", body)
	return s.Finish()
}

// ============================================================================
// OpenAI Responses API
// ============================================================================

type openaiResponsesRelay struct{}

func (openaiResponsesRelay) WriteSync(w http.ResponseWriter, resp *canonical.ChatResponse) error {
	body, err := responses.MarshalResponse(resp)
	if err != nil {
		return fmt.Errorf("marshal openai responses body: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return nil
}

func (openaiResponsesRelay) NewStream(w http.ResponseWriter, flush func(), responseID, model string) StreamSink {
	writeStreamHeaders(w)
	return responses.NewSSEWriter(w, flush, responseID, model)
}

// ============================================================================
// Anthropic Messages API
// ============================================================================

type anthropicRelay struct{}

func (anthropicRelay) WriteSync(w http.ResponseWriter, resp *canonical.ChatResponse) error {
	body, err := claude.MarshalMessagesResponse(resp)
	if err != nil {
		return fmt.Errorf("marshal anthropic response: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return nil
}

func (anthropicRelay) NewStream(w http.ResponseWriter, flush func(), responseID, model string) StreamSink {
	writeStreamHeaders(w)
	return claude.NewSSEWriter(w, flush, responseID, model)
}

// ============================================================================
// Helpers
// ============================================================================

func writeStreamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
}

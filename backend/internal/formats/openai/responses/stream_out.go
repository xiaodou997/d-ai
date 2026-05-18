package responses

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"uni-ai-api/backend/internal/formats/canonical"
)

// SSEWriter emits OpenAI Responses API SSE events from a canonical StreamChunk
// stream. State machine produces the following sequence:
//
//	response.created
//	response.output_item.added         (message item, on first text)
//	response.content_part.added        (output_text part)
//	response.output_text.delta * N
//	response.content_part.done
//	response.output_item.done
//	response.output_item.added         (function_call item, per tool call)
//	response.function_call_arguments.delta * N
//	response.output_item.done
//	response.completed
type SSEWriter struct {
	w        io.Writer
	flusher  func()
	id       string
	model    string
	started  bool
	finished bool

	textItemID    string
	textOpen      bool
	textIndex     int
	contentOpen   bool

	toolItems map[int]toolState // upstream tool index → state
	nextOut   int

	finalUsage *canonical.Usage
	stopReason string
}

type toolState struct {
	outputIndex int
	itemID      string
	callID      string
	name        string
}

// NewSSEWriter binds a writer to a chunked HTTP response stream.
func NewSSEWriter(w io.Writer, flush func(), responseID, model string) *SSEWriter {
	return &SSEWriter{
		w:         w,
		flusher:   flush,
		id:        ensureRespID(responseID),
		model:     model,
		toolItems: make(map[int]toolState),
	}
}

// Start emits response.created with an empty response shell. The
// promptTokens argument is accepted for interface symmetry with Anthropic's
// writer but Responses API does not surface input tokens until completion.
func (s *SSEWriter) Start(_ int) error {
	if s.started {
		return nil
	}
	s.started = true
	return s.writeEvent("response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         s.id,
			"object":     "response",
			"created_at": time.Now().Unix(),
			"model":      s.model,
			"status":     "in_progress",
			"output":     []any{},
		},
	})
}

// Push converts one canonical StreamChunk into the appropriate event(s).
func (s *SSEWriter) Push(chunk *canonical.StreamChunk) error {
	if chunk == nil || !s.started || s.finished {
		return nil
	}
	if chunk.Usage != nil {
		s.finalUsage = chunk.Usage
	}
	for _, choice := range chunk.Choices {
		if err := s.pushChoice(choice); err != nil {
			return err
		}
	}
	return nil
}

func (s *SSEWriter) pushChoice(choice canonical.Choice) error {
	if choice.FinishReason != "" {
		s.stopReason = choice.FinishReason
	}

	if text := choice.Delta.Content; text != "" {
		if err := s.ensureTextOpen(); err != nil {
			return err
		}
		if err := s.writeEvent("response.output_text.delta", map[string]any{
			"type":          "response.output_text.delta",
			"item_id":       s.textItemID,
			"output_index":  s.textIndex,
			"content_index": 0,
			"delta":         text,
		}); err != nil {
			return err
		}
	}

	for _, tc := range choice.Delta.ToolCalls {
		if err := s.pushToolCall(tc); err != nil {
			return err
		}
	}
	return nil
}

func (s *SSEWriter) ensureTextOpen() error {
	if s.textOpen {
		return nil
	}
	s.textOpen = true
	s.textIndex = s.nextOut
	s.nextOut++
	s.textItemID = ensureMsgID("")

	if err := s.writeEvent("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": s.textIndex,
		"item": map[string]any{
			"type":    "message",
			"id":      s.textItemID,
			"role":    "assistant",
			"status":  "in_progress",
			"content": []any{},
		},
	}); err != nil {
		return err
	}
	s.contentOpen = true
	return s.writeEvent("response.content_part.added", map[string]any{
		"type":          "response.content_part.added",
		"item_id":       s.textItemID,
		"output_index":  s.textIndex,
		"content_index": 0,
		"part": map[string]any{
			"type": "output_text",
			"text": "",
		},
	})
}

func (s *SSEWriter) closeText() error {
	if !s.textOpen {
		return nil
	}
	if s.contentOpen {
		if err := s.writeEvent("response.content_part.done", map[string]any{
			"type":          "response.content_part.done",
			"item_id":       s.textItemID,
			"output_index":  s.textIndex,
			"content_index": 0,
			"part": map[string]any{
				"type": "output_text",
				"text": "",
			},
		}); err != nil {
			return err
		}
		s.contentOpen = false
	}
	if err := s.writeEvent("response.output_item.done", map[string]any{
		"type":         "response.output_item.done",
		"output_index": s.textIndex,
		"item": map[string]any{
			"type":   "message",
			"id":     s.textItemID,
			"role":   "assistant",
			"status": "completed",
		},
	}); err != nil {
		return err
	}
	s.textOpen = false
	return nil
}

func (s *SSEWriter) pushToolCall(tc canonical.ToolCall) error {
	state, opened := s.toolItems[tc.Index]
	if !opened {
		if err := s.closeText(); err != nil {
			return err
		}
		state = toolState{
			outputIndex: s.nextOut,
			itemID:      ensureFCID(""),
			callID:      ensureCallID(tc.ID),
			name:        tc.Function.Name,
		}
		s.nextOut++
		s.toolItems[tc.Index] = state
		if err := s.writeEvent("response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": state.outputIndex,
			"item": map[string]any{
				"type":      "function_call",
				"id":        state.itemID,
				"call_id":   state.callID,
				"name":      state.name,
				"arguments": "",
				"status":    "in_progress",
			},
		}); err != nil {
			return err
		}
	}
	if args := tc.Function.Arguments; args != "" {
		if err := s.writeEvent("response.function_call_arguments.delta", map[string]any{
			"type":         "response.function_call_arguments.delta",
			"item_id":      state.itemID,
			"output_index": state.outputIndex,
			"call_id":      state.callID,
			"delta":        args,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *SSEWriter) closeAllTools() error {
	for _, st := range s.toolItems {
		if err := s.writeEvent("response.output_item.done", map[string]any{
			"type":         "response.output_item.done",
			"output_index": st.outputIndex,
			"item": map[string]any{
				"type":    "function_call",
				"id":      st.itemID,
				"call_id": st.callID,
				"name":    st.name,
				"status":  "completed",
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

// Finish closes any open items and emits response.completed.
func (s *SSEWriter) Finish() error {
	if !s.started || s.finished {
		return nil
	}
	s.finished = true

	if err := s.closeText(); err != nil {
		return err
	}
	if err := s.closeAllTools(); err != nil {
		return err
	}

	status := "completed"
	if s.stopReason == "length" {
		status = "incomplete"
	}

	respShell := map[string]any{
		"id":         s.id,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      s.model,
		"status":     status,
	}
	if s.finalUsage != nil {
		usage := map[string]any{
			"input_tokens":  s.finalUsage.PromptTokens,
			"output_tokens": s.finalUsage.CompletionTokens,
			"total_tokens":  s.finalUsage.TotalTokens,
		}
		if s.finalUsage.CacheReadTokens > 0 {
			usage["input_tokens_details"] = map[string]int{"cached_tokens": s.finalUsage.CacheReadTokens}
		}
		if s.finalUsage.ReasoningTokens > 0 {
			usage["output_tokens_details"] = map[string]int{"reasoning_tokens": s.finalUsage.ReasoningTokens}
		}
		respShell["usage"] = usage
	}
	return s.writeEvent("response.completed", map[string]any{
		"type":     "response.completed",
		"response": respShell,
	})
}

// Error emits a response.error SSE frame and marks the writer finished.
func (s *SSEWriter) Error(code, message string) error {
	if !s.started {
		_ = s.Start(0)
	}
	s.finished = true
	return s.writeEvent("response.error", map[string]any{
		"type": "response.error",
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func (s *SSEWriter) writeEvent(eventType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", eventType, err)
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, body); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher()
	}
	return nil
}

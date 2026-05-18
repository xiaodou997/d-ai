package claude

import (
	"encoding/json"
	"fmt"
	"io"

	"uni-ai-api/backend/internal/formats/canonical"
)

// SSEWriter writes a stream of canonical StreamChunks to a client as Anthropic
// Messages SSE events. It is stateful: callers MUST call Start exactly once
// before any Push, and Finish exactly once after the last Push.
//
// Event sequence produced:
//
//	event: message_start
//	event: content_block_start  (per block)
//	event: content_block_delta  (n times)
//	event: content_block_stop   (per block)
//	event: message_delta        (with stop_reason + final usage)
//	event: message_stop
type SSEWriter struct {
	w         io.Writer
	flusher   func()
	messageID string
	model     string

	started     bool
	finished    bool
	curTextOpen bool
	curTextIdx  int

	// tool_use blocks live in their own indexes after any text block.
	// Map upstream tool call index → our Anthropic content block index.
	toolBlocks map[int]int
	nextIndex  int

	stopReason  string
	stopSeq     string
	finalUsage  *canonical.Usage
}

// NewSSEWriter returns a writer bound to a chunked HTTP response stream.
// flush may be nil; if provided it is called after every event for prompt delivery.
func NewSSEWriter(w io.Writer, flush func(), messageID, model string) *SSEWriter {
	return &SSEWriter{
		w:          w,
		flusher:    flush,
		messageID:  ensureClaudeID(messageID),
		model:      model,
		toolBlocks: make(map[int]int),
	}
}

// Start emits message_start. The promptTokens estimate is sent in the initial
// usage block (real input_tokens are only known after the upstream finishes
// for some providers).
func (s *SSEWriter) Start(promptTokens int) error {
	if s.started {
		return nil
	}
	s.started = true
	payload := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         s.model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  promptTokens,
				"output_tokens": 0,
			},
		},
	}
	return s.writeEvent("message_start", payload)
}

// Push converts a canonical StreamChunk into the appropriate Anthropic SSE
// event(s). Chunks with no actionable content are ignored.
func (s *SSEWriter) Push(chunk *canonical.StreamChunk) error {
	if chunk == nil || !s.started || s.finished {
		return nil
	}

	if chunk.Usage != nil {
		// Stash for final message_delta.
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
		s.stopReason = openAIFinishToClaudeStop(choice.FinishReason, len(choice.Delta.ToolCalls) > 0)
	}

	// Text delta
	if text := choice.Delta.Content; text != "" {
		if err := s.ensureTextOpen(); err != nil {
			return err
		}
		if err := s.writeEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": s.curTextIdx,
			"delta": map[string]string{"type": "text_delta", "text": text},
		}); err != nil {
			return err
		}
	}

	// Tool call deltas — name+id arrive on the first delta for each tool call,
	// subsequent deltas only carry partial JSON args.
	for _, tc := range choice.Delta.ToolCalls {
		if err := s.pushToolCallDelta(tc); err != nil {
			return err
		}
	}
	return nil
}

func (s *SSEWriter) ensureTextOpen() error {
	if s.curTextOpen {
		return nil
	}
	s.curTextOpen = true
	s.curTextIdx = s.nextIndex
	s.nextIndex++
	return s.writeEvent("content_block_start", map[string]any{
		"type":  "content_block_start",
		"index": s.curTextIdx,
		"content_block": map[string]any{
			"type": "text",
			"text": "",
		},
	})
}

func (s *SSEWriter) pushToolCallDelta(tc canonical.ToolCall) error {
	blockIdx, opened := s.toolBlocks[tc.Index]
	if !opened {
		// Close any open text block before starting a new tool_use block.
		if s.curTextOpen {
			if err := s.closeBlock(s.curTextIdx); err != nil {
				return err
			}
			s.curTextOpen = false
		}
		blockIdx = s.nextIndex
		s.nextIndex++
		s.toolBlocks[tc.Index] = blockIdx
		if err := s.writeEvent("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": blockIdx,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    ensureToolUseID(tc.ID),
				"name":  tc.Function.Name,
				"input": map[string]any{},
			},
		}); err != nil {
			return err
		}
	}
	if args := tc.Function.Arguments; args != "" {
		if err := s.writeEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": blockIdx,
			"delta": map[string]string{
				"type":         "input_json_delta",
				"partial_json": args,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *SSEWriter) closeBlock(index int) error {
	return s.writeEvent("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": index,
	})
}

// Finish closes any open blocks and emits message_delta + message_stop.
// Safe to call multiple times; only the first call has effect.
func (s *SSEWriter) Finish() error {
	if !s.started || s.finished {
		return nil
	}
	s.finished = true

	if s.curTextOpen {
		if err := s.closeBlock(s.curTextIdx); err != nil {
			return err
		}
		s.curTextOpen = false
	}
	for _, idx := range s.toolBlocks {
		if err := s.closeBlock(idx); err != nil {
			return err
		}
	}

	stop := s.stopReason
	if stop == "" {
		stop = "end_turn"
	}
	delta := map[string]any{
		"stop_reason":   stop,
		"stop_sequence": nil,
	}
	if s.stopSeq != "" {
		delta["stop_sequence"] = s.stopSeq
	}
	msgDelta := map[string]any{
		"type":  "message_delta",
		"delta": delta,
	}
	if s.finalUsage != nil {
		msgDelta["usage"] = map[string]int{
			"input_tokens":                s.finalUsage.PromptTokens,
			"output_tokens":               s.finalUsage.CompletionTokens,
			"cache_creation_input_tokens": s.finalUsage.CacheWriteTokens,
			"cache_read_input_tokens":     s.finalUsage.CacheReadTokens,
		}
	}
	if err := s.writeEvent("message_delta", msgDelta); err != nil {
		return err
	}
	return s.writeEvent("message_stop", map[string]any{"type": "message_stop"})
}

// Error emits an `event: error` SSE frame. Use when an upstream failure
// occurs mid-stream and the response has already been committed.
func (s *SSEWriter) Error(errType, message string) error {
	if !s.started {
		// Best effort: emit a message_start so clients have at least a usable envelope.
		_ = s.Start(0)
	}
	if !s.finished && s.curTextOpen {
		_ = s.closeBlock(s.curTextIdx)
		s.curTextOpen = false
	}
	s.finished = true
	return s.writeEvent("error", map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    mapErrorType(errType),
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

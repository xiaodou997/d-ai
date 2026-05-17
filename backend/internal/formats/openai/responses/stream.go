package responses

import (
	"encoding/json"
	"time"

	"uni-ai-api/backend/internal/formats/canonical"
)

// StreamEvent is a single SSE data payload from the Responses API stream.
// The event type is carried both in the SSE "event:" header line and in the
// JSON "type" field; we read from the JSON to avoid relying on SSE header order.
type StreamEvent struct {
	Type         string          `json:"type"`
	// response.output_text.delta / response.function_call_arguments.delta
	Delta        string          `json:"delta,omitempty"`
	OutputIndex  int             `json:"output_index,omitempty"`
	ContentIndex int             `json:"content_index,omitempty"`
	// response.function_call_arguments.delta
	CallID       string          `json:"call_id,omitempty"`
	// response.completed / response.created
	Response     *Response       `json:"response,omitempty"`
	// response.output_item.added / done
	Item         *OutputItem     `json:"item,omitempty"`
	// response.content_part.added / done
	Part         *OutputContentPart `json:"part,omitempty"`
}

// StreamEventToCanonical converts one parsed StreamEvent to a canonical StreamChunk.
// Returns nil for events that carry no text content (e.g. ping, created, item.added).
func StreamEventToCanonical(data []byte, responseID, model string, index int) *canonical.StreamChunk {
	var event StreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil
	}

	switch event.Type {
	case "response.output_item.added":
		if event.Item == nil || event.Item.Type != "function_call" {
			return nil
		}
		return &canonical.StreamChunk{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []canonical.Choice{{
				Index: index,
				Delta: canonical.Message{
					Role: canonical.RoleAssistant,
					ToolCalls: []canonical.ToolCall{{
						ID:    event.Item.CallID,
						Type:  "function",
						Index: event.OutputIndex,
						Function: canonical.FuncCall{
							Name:      event.Item.Name,
							Arguments: "",
						},
					}},
				},
			}},
		}

	case "response.function_call_arguments.delta":
		if event.Delta == "" {
			return nil
		}
		return &canonical.StreamChunk{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []canonical.Choice{{
				Index: index,
				Delta: canonical.Message{
					Role: canonical.RoleAssistant,
					ToolCalls: []canonical.ToolCall{{
						Index: event.OutputIndex,
						Function: canonical.FuncCall{
							Arguments: event.Delta,
						},
					}},
				},
			}},
		}

	case "response.output_text.delta":
		if event.Delta == "" {
			return nil
		}
		return &canonical.StreamChunk{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []canonical.Choice{
				{
					Index: index,
					Delta: canonical.Message{
						Role:    canonical.RoleAssistant,
						Content: event.Delta,
					},
				},
			},
		}

	case "response.completed":
		if event.Response == nil {
			return nil
		}
		resp := event.Response
		chunk := &canonical.StreamChunk{
			ID:      resp.ID,
			Object:  "chat.completion.chunk",
			Created: resp.CreatedAt,
			Model:   model,
			Choices: []canonical.Choice{
				{Index: 0, FinishReason: "stop", Delta: canonical.Message{}},
			},
		}
		if resp.Usage != nil {
			u := &canonical.Usage{
				PromptTokens:     resp.Usage.InputTokens,
				CompletionTokens: resp.Usage.OutputTokens,
				TotalTokens:      resp.Usage.TotalTokens,
			}
			if resp.Usage.InputTokensDetails != nil {
				u.CacheReadTokens = resp.Usage.InputTokensDetails.CachedTokens
			}
			if resp.Usage.OutputTokensDetails != nil {
				u.ReasoningTokens = resp.Usage.OutputTokensDetails.ReasoningTokens
			}
			chunk.Usage = u
		}
		return chunk
	}

	return nil
}

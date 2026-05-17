// Package claude implements Anthropic Claude Messages API types and their
// bidirectional conversion to/from the gateway's canonical format.
package claude

import (
	"encoding/json"
	"fmt"

	"uni-ai-api/backend/internal/formats/canonical"
)

// ============================================================================
// Anthropic Messages API wire types
// ============================================================================

type MessagesRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	// Extended thinking (Claude 3.7+)
	Thinking  *ThinkingConfig `json:"thinking,omitempty"`
	// Sampling parameters
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	TopK        *int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
	Stream      bool     `json:"stream,omitempty"`
	Tools       []Tool   `json:"tools,omitempty"`
	ToolChoice  any      `json:"tool_choice,omitempty"`
	Metadata    *Metadata `json:"metadata,omitempty"`
}

type ThinkingConfig struct {
	Type         string `json:"type"`          // "enabled" | "disabled"
	BudgetTokens int    `json:"budget_tokens"` // max thinking tokens
}

type Metadata struct {
	UserID string `json:"user_id,omitempty"`
}

// Message supports multi-modal content (text + image blocks).
type Message struct {
	Role    string          `json:"role"` // "user" | "assistant"
	Content json.RawMessage `json:"content"` // string OR []Block
}

// Block is one element in an Anthropic content array.
type Block struct {
	Type string `json:"type"` // "text" | "image" | "tool_use" | "tool_result" | "thinking"

	// text block
	Text string `json:"text,omitempty"`

	// image block
	Source *ImageSource `json:"source,omitempty"`

	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result block
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string OR []Block

	// thinking block
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`       // "base64" | "url"
	MediaType string `json:"media_type"` // "image/jpeg" etc.
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ============================================================================
// Anthropic Messages API response types
// ============================================================================

type MessagesResponse struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"` // "message"
	Role         string  `json:"role"` // "assistant"
	Content      []Block `json:"content"`
	Model        string  `json:"model"`
	StopReason   string  `json:"stop_reason"`
	StopSequence string  `json:"stop_sequence,omitempty"`
	Usage        Usage   `json:"usage"`
}

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// Stream event types
type StreamEvent struct {
	Type         string          `json:"type"`
	Index        int             `json:"index,omitempty"`
	Delta        json.RawMessage `json:"delta,omitempty"`
	ContentBlock json.RawMessage `json:"content_block,omitempty"`
	Usage        *Usage          `json:"usage,omitempty"`
}

type ContentBlockDelta struct {
	Type        string `json:"type"` // "text_delta" | "input_json_delta" | "thinking_delta"
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
}

type ContentBlockStart struct {
	Type string `json:"type"` // "text" | "tool_use" | "thinking"
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ============================================================================
// Conversion: canonical → Claude (for forwarding to Anthropic upstream)
// ============================================================================

func MessagesRequestFromCanonical(req *canonical.ChatRequest, upstreamModel string) (*MessagesRequest, error) {
	out := &MessagesRequest{
		Model:       upstreamModel,
		MaxTokens:   4096,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	if req.MaxTokens != nil {
		out.MaxTokens = *req.MaxTokens
	}

	// Extended thinking
	if req.ThinkingBudget != nil && *req.ThinkingBudget > 0 {
		out.Thinking = &ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: *req.ThinkingBudget,
		}
	}

	// Map reasoning_effort to thinking budget if thinking not explicitly set
	if out.Thinking == nil && req.ReasoningEffort != nil {
		budget := reasoningEffortToBudget(*req.ReasoningEffort)
		out.Thinking = &ThinkingConfig{Type: "enabled", BudgetTokens: budget}
	}

	out.StopSequences = req.Stop

	// Tools
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, Tool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	if req.ToolChoice != nil {
		out.ToolChoice = req.ToolChoice
	}

	// Separate system messages and convert the rest
	for _, m := range req.Messages {
		if m.Role == canonical.RoleSystem {
			if out.System != "" {
				out.System += "\n"
			}
			out.System += m.TextContent()
			continue
		}
		cm, err := messageFromCanonical(m)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, cm)
	}

	return out, nil
}

func messageFromCanonical(m canonical.Message) (Message, error) {
	role := string(m.Role)
	if role == "tool" {
		role = "user" // tool results are "user" role in Claude
	}
	cm := Message{Role: role}

	// Handle tool calls (assistant message with tool_use blocks)
	if len(m.ToolCalls) > 0 {
		var blocks []Block
		if m.Content != "" {
			blocks = append(blocks, Block{Type: "text", Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			var input json.RawMessage
			if tc.Function.Arguments != "" {
				input = json.RawMessage(tc.Function.Arguments)
			} else {
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, Block{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
		b, err := json.Marshal(blocks)
		if err != nil {
			return cm, err
		}
		cm.Content = b
		return cm, nil
	}

	// Tool result message
	if m.Role == canonical.RoleTool && m.ToolCallID != "" {
		content, _ := json.Marshal(m.Content)
		block := Block{
			Type:      "tool_result",
			ToolUseID: m.ToolCallID,
			Content:   content,
		}
		b, err := json.Marshal([]Block{block})
		if err != nil {
			return cm, err
		}
		cm.Content = b
		return cm, nil
	}

	// Multi-modal content
	if m.IsMultiModal() {
		var blocks []Block
		for _, p := range m.Parts {
			switch p.Type {
			case "text":
				blocks = append(blocks, Block{Type: "text", Text: p.Text})
			case "image_url":
				if p.ImageURL != nil {
					blocks = append(blocks, imageURLToBlock(p.ImageURL.URL))
				}
			}
		}
		b, err := json.Marshal(blocks)
		if err != nil {
			return cm, err
		}
		cm.Content = b
		return cm, nil
	}

	// Plain text
	b, _ := json.Marshal(m.Content)
	cm.Content = b
	return cm, nil
}

func imageURLToBlock(url string) Block {
	return Block{
		Type: "image",
		Source: &ImageSource{
			Type: "url",
			URL:  url,
		},
	}
}

func reasoningEffortToBudget(effort string) int {
	switch effort {
	case "high":
		return 10000
	case "medium":
		return 5000
	case "low":
		return 2000
	default:
		return 5000
	}
}

// ============================================================================
// Conversion: Claude response → canonical
// ============================================================================

func MessagesResponseToCanonical(resp *MessagesResponse) *canonical.ChatResponse {
	cr := &canonical.ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Model:   resp.Model,
		Created: 0, // Anthropic does not return a timestamp
	}

	// Build a single choice from all content blocks
	choice := canonical.Choice{
		Index:        0,
		FinishReason: claudeStopReasonToOpenAI(resp.StopReason),
	}
	var textParts []string
	var toolCalls []canonical.ToolCall
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, canonical.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: canonical.FuncCall{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}

	msg := canonical.Message{Role: canonical.RoleAssistant}
	if len(textParts) > 0 {
		msg.Content = joinStrings(textParts)
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
		choice.FinishReason = "tool_calls"
	}
	choice.Message = msg
	cr.Choices = []canonical.Choice{choice}

	cr.Usage = &canonical.Usage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		CacheWriteTokens: resp.Usage.CacheCreationInputTokens,
		CacheReadTokens:  resp.Usage.CacheReadInputTokens,
	}

	return cr
}

func claudeStopReasonToOpenAI(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "stop_sequence":
		return "stop"
	default:
		return reason
	}
}

// ============================================================================
// Stream event → canonical stream chunk conversion
// ============================================================================

// StreamEventToCanonical converts a single Anthropic SSE event into a canonical
// stream chunk. Returns nil for events that don't produce output (e.g. ping).
func StreamEventToCanonical(event *StreamEvent, responseID, model string, index int) *canonical.StreamChunk {
	switch event.Type {
	case "content_block_start":
		var block ContentBlockStart
		if err := json.Unmarshal(event.ContentBlock, &block); err != nil || block.Type != "tool_use" {
			return nil
		}
		return &canonical.StreamChunk{
			ID:     responseID,
			Object: "chat.completion.chunk",
			Model:  model,
			Choices: []canonical.Choice{{
				Index: index,
				Delta: canonical.Message{
					Role: canonical.RoleAssistant,
					ToolCalls: []canonical.ToolCall{{
						ID:    block.ID,
						Type:  "function",
						Index: event.Index,
						Function: canonical.FuncCall{
							Name:      block.Name,
							Arguments: "",
						},
					}},
				},
			}},
		}

	case "content_block_delta":
		var delta ContentBlockDelta
		if err := json.Unmarshal(event.Delta, &delta); err != nil {
			return nil
		}
		chunk := &canonical.StreamChunk{
			ID:     responseID,
			Object: "chat.completion.chunk",
			Model:  model,
		}
		choice := canonical.Choice{Index: index}
		switch delta.Type {
		case "text_delta":
			choice.Delta = canonical.Message{
				Role:    canonical.RoleAssistant,
				Content: delta.Text,
			}
		case "input_json_delta":
			choice.Delta = canonical.Message{
				Role: canonical.RoleAssistant,
				ToolCalls: []canonical.ToolCall{{
					Index: event.Index,
					Function: canonical.FuncCall{
						Arguments: delta.PartialJSON,
					},
				}},
			}
		}
		chunk.Choices = []canonical.Choice{choice}
		return chunk

	case "message_delta":
		// May contain stop reason and final usage
		chunk := &canonical.StreamChunk{
			ID:     responseID,
			Object: "chat.completion.chunk",
			Model:  model,
		}
		if event.Usage != nil {
			chunk.Usage = &canonical.Usage{
				PromptTokens:     event.Usage.InputTokens,
				CompletionTokens: event.Usage.OutputTokens,
				TotalTokens:      event.Usage.InputTokens + event.Usage.OutputTokens,
				CacheWriteTokens: event.Usage.CacheCreationInputTokens,
				CacheReadTokens:  event.Usage.CacheReadInputTokens,
			}
		}
		return chunk

	case "message_stop":
		return &canonical.StreamChunk{
			ID:     responseID,
			Object: "chat.completion.chunk",
			Model:  model,
			Choices: []canonical.Choice{{
				Index:        0,
				FinishReason: "stop",
			}},
		}
	}
	return nil
}

func joinStrings(ss []string) string {
	if len(ss) == 1 {
		return ss[0]
	}
	out := ""
	for _, s := range ss {
		out += s
	}
	return out
}

// ExtractUsageFromResponse extracts token usage from a Claude response for billing.
func ExtractUsageFromResponse(resp *MessagesResponse) canonical.Usage {
	if resp == nil {
		return canonical.Usage{}
	}
	return canonical.Usage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		CacheWriteTokens: resp.Usage.CacheCreationInputTokens,
		CacheReadTokens:  resp.Usage.CacheReadInputTokens,
	}
}

// Ensure we use fmt for error wrapping.
var _ = fmt.Errorf

// Package openai implements OpenAI Chat Completions API request/response types
// and their conversion to/from the canonical format.
package openai

import (
	"encoding/json"
	"fmt"

	"uni-ai-api/backend/internal/formats/canonical"
)

// ============================================================================
// Wire types — matching OpenAI's Chat Completions API schema
// ============================================================================

type ChatRequest struct {
	Model            string            `json:"model"`
	Messages         []Message         `json:"messages"`
	MaxTokens        *int              `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int           `json:"max_completion_tokens,omitempty"`
	Temperature      *float64          `json:"temperature,omitempty"`
	TopP             *float64          `json:"top_p,omitempty"`
	N                *int              `json:"n,omitempty"`
	Stream           bool              `json:"stream,omitempty"`
	StreamOptions    *StreamOptions    `json:"stream_options,omitempty"`
	Stop             json.RawMessage   `json:"stop,omitempty"` // string or []string
	Tools            []Tool            `json:"tools,omitempty"`
	ToolChoice       json.RawMessage   `json:"tool_choice,omitempty"`
	ReasoningEffort  *string           `json:"reasoning_effort,omitempty"`
	// OpenAI o-series thinking budget (o3, o4-mini)
	Thinking         *ThinkingConfig   `json:"thinking,omitempty"`
	User             string            `json:"user,omitempty"`
	// Allow any additional fields to be forwarded to upstream unchanged.
	Extra            map[string]json.RawMessage `json:"-"`
}

type ThinkingConfig struct {
	Type         string `json:"type"`          // "enabled" | "disabled"
	BudgetTokens int    `json:"budget_tokens"` // max tokens for thinking
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"` // string OR []ContentPart
	Name       string          `json:"name,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function FuncCall `json:"function"`
	Index    int      `json:"index,omitempty"`
}

type FuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function FuncSpec `json:"function"`
}

type FuncSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type ChatResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Delta        Message `json:"delta,omitempty"`
	FinishReason string  `json:"finish_reason"`
	Logprobs     any     `json:"logprobs,omitempty"`
}

type Usage struct {
	PromptTokens            int                    `json:"prompt_tokens"`
	CompletionTokens        int                    `json:"completion_tokens"`
	TotalTokens             int                    `json:"total_tokens"`
	CompletionTokensDetails map[string]int         `json:"completion_tokens_details,omitempty"`
	PromptTokensDetails     map[string]int         `json:"prompt_tokens_details,omitempty"`
}

type StreamChunk struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

// ============================================================================
// Conversion: OpenAI → canonical
// ============================================================================

func ChatRequestToCanonical(req *ChatRequest) (*canonical.ChatRequest, error) {
	msgs := make([]canonical.Message, 0, len(req.Messages))
	for i, m := range req.Messages {
		cm, err := messageToCanonical(m)
		if err != nil {
			return nil, fmt.Errorf("message[%d]: %w", i, err)
		}
		msgs = append(msgs, cm)
	}

	out := &canonical.ChatRequest{
		Model:           req.Model,
		Messages:        msgs,
		MaxTokens:       req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		N:               req.N,
		Stream:          req.Stream,
		ReasoningEffort: req.ReasoningEffort,
	}

	// MaxCompletionTokens takes precedence (newer OpenAI field)
	if req.MaxCompletionTokens != nil {
		out.MaxTokens = req.MaxCompletionTokens
	}

	// Thinking config → ThinkingBudget
	if req.Thinking != nil && req.Thinking.Type == "enabled" && req.Thinking.BudgetTokens > 0 {
		out.ThinkingBudget = &req.Thinking.BudgetTokens
	}

	// Stop: unmarshal string or []string
	if len(req.Stop) > 0 {
		var ss string
		if json.Unmarshal(req.Stop, &ss) == nil {
			out.Stop = []string{ss}
		} else {
			var sl []string
			if err := json.Unmarshal(req.Stop, &sl); err == nil {
				out.Stop = sl
			}
		}
	}

	// Tools
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, canonical.Tool{
			Type: t.Type,
			Function: canonical.FuncSpec{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
				Strict:      t.Function.Strict,
			},
		})
	}
	if len(req.ToolChoice) > 0 {
		out.ToolChoice = req.ToolChoice
	}

	return out, nil
}

func messageToCanonical(m Message) (canonical.Message, error) {
	cm := canonical.Message{
		Role:       canonical.Role(m.Role),
		Name:       m.Name,
		ToolCallID: m.ToolCallID,
	}

	// Decode tool calls
	for _, tc := range m.ToolCalls {
		cm.ToolCalls = append(cm.ToolCalls, canonical.ToolCall{
			ID:    tc.ID,
			Type:  tc.Type,
			Index: tc.Index,
			Function: canonical.FuncCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	if len(m.Content) == 0 {
		return cm, nil
	}

	// Try string first
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		cm.Content = text
		return cm, nil
	}

	// Try array of content parts
	var parts []ContentPart
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return cm, fmt.Errorf("decode content: %w", err)
	}
	for _, p := range parts {
		cp := canonical.ContentPart{Type: p.Type, Text: p.Text}
		if p.ImageURL != nil {
			cp.ImageURL = &canonical.ImageURL{URL: p.ImageURL.URL, Detail: p.ImageURL.Detail}
		}
		cm.Parts = append(cm.Parts, cp)
	}
	return cm, nil
}

// ============================================================================
// Conversion: canonical → OpenAI (for forwarding to OpenAI-compatible upstreams)
// ============================================================================

func ChatRequestFromCanonical(req *canonical.ChatRequest, upstreamModel string) (*ChatRequest, error) {
	out := &ChatRequest{
		Model:           upstreamModel,
		MaxTokens:       req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		N:               req.N,
		Stream:          req.Stream,
		ReasoningEffort: req.ReasoningEffort,
	}

	if req.ThinkingBudget != nil {
		out.Thinking = &ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: *req.ThinkingBudget,
		}
	}

	for _, m := range req.Messages {
		om, err := messageFromCanonical(m)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, om)
	}

	if len(req.Stop) == 1 {
		b, _ := json.Marshal(req.Stop[0])
		out.Stop = b
	} else if len(req.Stop) > 1 {
		b, _ := json.Marshal(req.Stop)
		out.Stop = b
	}

	for _, t := range req.Tools {
		out.Tools = append(out.Tools, Tool{
			Type: t.Type,
			Function: FuncSpec{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
				Strict:      t.Function.Strict,
			},
		})
	}
	if req.ToolChoice != nil {
		b, _ := json.Marshal(req.ToolChoice)
		out.ToolChoice = b
	}

	return out, nil
}

func messageFromCanonical(m canonical.Message) (Message, error) {
	om := Message{
		Role:       string(m.Role),
		Name:       m.Name,
		ToolCallID: m.ToolCallID,
	}
	for _, tc := range m.ToolCalls {
		om.ToolCalls = append(om.ToolCalls, ToolCall{
			ID:    tc.ID,
			Type:  tc.Type,
			Index: tc.Index,
			Function: FuncCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	if m.IsMultiModal() {
		parts := make([]ContentPart, 0, len(m.Parts))
		for _, p := range m.Parts {
			cp := ContentPart{Type: p.Type, Text: p.Text}
			if p.ImageURL != nil {
				cp.ImageURL = &ImageURL{URL: p.ImageURL.URL, Detail: p.ImageURL.Detail}
			}
			parts = append(parts, cp)
		}
		b, err := json.Marshal(parts)
		if err != nil {
			return om, err
		}
		om.Content = b
	} else {
		b, _ := json.Marshal(m.Content)
		om.Content = b
	}
	return om, nil
}

// ============================================================================
// Conversion: canonical response → OpenAI response
// ============================================================================

func ChatResponseToCanonical(resp *ChatResponse) *canonical.ChatResponse {
	cr := &canonical.ChatResponse{
		ID:                resp.ID,
		Object:            resp.Object,
		Created:           resp.Created,
		Model:             resp.Model,
		SystemFingerprint: resp.SystemFingerprint,
	}
	for _, c := range resp.Choices {
		cm, _ := messageToCanonical(c.Message)
		cr.Choices = append(cr.Choices, canonical.Choice{
			Index:        c.Index,
			Message:      cm,
			FinishReason: c.FinishReason,
		})
	}
	if resp.Usage != nil {
		cr.Usage = usageToCanonical(resp.Usage)
	}
	return cr
}

func ChatResponseFromCanonical(resp *canonical.ChatResponse) *ChatResponse {
	out := &ChatResponse{
		ID:                resp.ID,
		Object:            resp.Object,
		Created:           resp.Created,
		Model:             resp.Model,
		SystemFingerprint: resp.SystemFingerprint,
	}
	for _, c := range resp.Choices {
		om, _ := messageFromCanonical(c.Message)
		out.Choices = append(out.Choices, Choice{
			Index:        c.Index,
			Message:      om,
			FinishReason: c.FinishReason,
		})
	}
	if resp.Usage != nil {
		out.Usage = usageFromCanonical(resp.Usage)
	}
	return out
}

func StreamChunkToCanonical(chunk *StreamChunk) *canonical.StreamChunk {
	cc := &canonical.StreamChunk{
		ID:      chunk.ID,
		Object:  chunk.Object,
		Created: chunk.Created,
		Model:   chunk.Model,
	}
	for _, c := range chunk.Choices {
		delta, _ := messageToCanonical(c.Delta)
		cc.Choices = append(cc.Choices, canonical.Choice{
			Index:        c.Index,
			Delta:        delta,
			FinishReason: c.FinishReason,
		})
	}
	if chunk.Usage != nil {
		cc.Usage = usageToCanonical(chunk.Usage)
	}
	return cc
}

func StreamChunkFromCanonical(chunk *canonical.StreamChunk) *StreamChunk {
	out := &StreamChunk{
		ID:      chunk.ID,
		Object:  chunk.Object,
		Created: chunk.Created,
		Model:   chunk.Model,
	}
	for _, c := range chunk.Choices {
		delta, _ := messageFromCanonical(c.Delta)
		out.Choices = append(out.Choices, Choice{
			Index:        c.Index,
			Delta:        delta,
			FinishReason: c.FinishReason,
		})
	}
	if chunk.Usage != nil {
		out.Usage = usageFromCanonical(chunk.Usage)
	}
	return out
}

func usageToCanonical(u *Usage) *canonical.Usage {
	cu := &canonical.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	// Extract reasoning tokens from CompletionTokensDetails if present
	if rt, ok := u.CompletionTokensDetails["reasoning_tokens"]; ok {
		cu.ReasoningTokens = rt
	}
	// Extract cache tokens from PromptTokensDetails if present
	if ct, ok := u.PromptTokensDetails["cached_tokens"]; ok {
		cu.CacheReadTokens = ct
	}
	return cu
}

func usageFromCanonical(u *canonical.Usage) *Usage {
	out := &Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	if u.ReasoningTokens > 0 {
		out.CompletionTokensDetails = map[string]int{
			"reasoning_tokens": u.ReasoningTokens,
		}
	}
	if u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 {
		out.PromptTokensDetails = map[string]int{
			"cached_tokens": u.CacheReadTokens,
		}
	}
	return out
}

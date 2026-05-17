// Package responses implements OpenAI Responses API types and conversions.
package responses

import (
	"encoding/json"

	"uni-ai-api/backend/internal/formats/canonical"
)

// Request is the OpenAI Responses API request body (/v1/responses).
type Request struct {
	Model           string      `json:"model"`
	Input           []InputItem `json:"input"`
	Instructions    string      `json:"instructions,omitempty"`
	MaxOutputTokens *int        `json:"max_output_tokens,omitempty"`
	Temperature     *float64    `json:"temperature,omitempty"`
	TopP            *float64    `json:"top_p,omitempty"`
	Stream          bool        `json:"stream,omitempty"`
	Store           *bool       `json:"store,omitempty"`
	Tools           []Tool      `json:"tools,omitempty"`
}

// InputItem is one turn in the conversation.
type InputItem struct {
	Role    string             `json:"role"`
	Content []InputContentPart `json:"content"`
}

// InputContentPart represents a single content block inside an InputItem.
type InputContentPart struct {
	Type string `json:"type"` // "input_text" | "input_image"
	Text string `json:"text,omitempty"`
}

// Tool wraps a function definition for the Responses API.
type Tool struct {
	Type     string   `json:"type"`
	Name     string   `json:"name,omitempty"`
	Function FuncSpec `json:"function"`
}

// FuncSpec mirrors the Chat Completions function spec.
type FuncSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// FromCanonical converts a canonical ChatRequest to a Responses API Request.
// System messages become the top-level Instructions field.
func FromCanonical(req *canonical.ChatRequest, upstreamModel string) *Request {
	out := &Request{
		Model:       upstreamModel,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}
	if req.MaxTokens != nil {
		out.MaxOutputTokens = req.MaxTokens
	}

	// Extract the first system message as instructions.
	for _, m := range req.Messages {
		if m.Role == canonical.RoleSystem {
			out.Instructions = m.Content
			break
		}
	}

	// Convert non-system messages to InputItems.
	for _, m := range req.Messages {
		if m.Role == canonical.RoleSystem {
			continue
		}
		item := InputItem{Role: string(m.Role)}
		switch {
		case len(m.Parts) > 0:
			for _, p := range m.Parts {
				if p.Text != "" {
					item.Content = append(item.Content, InputContentPart{Type: "input_text", Text: p.Text})
				}
			}
		case m.Content != "":
			item.Content = []InputContentPart{{Type: "input_text", Text: m.Content}}
		}
		out.Input = append(out.Input, item)
	}

	// Tools
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, Tool{
			Type: t.Type,
			Function: FuncSpec{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}

	return out
}

package responses

import (
	"strings"
	"time"

	"uni-ai-api/backend/internal/formats/canonical"
)

// Response is the OpenAI Responses API non-streaming response body.
type Response struct {
	ID        string         `json:"id"`
	Object    string         `json:"object"`
	CreatedAt int64          `json:"created_at"`
	Model     string         `json:"model"`
	Status    string         `json:"status"` // "completed" | "failed" | "incomplete"
	Output    []OutputItem   `json:"output"`
	Usage     *ResponseUsage `json:"usage,omitempty"`
	Error     *ResponseError `json:"error,omitempty"`
}

// OutputItem is one item in the Responses API output array.
type OutputItem struct {
	Type    string              `json:"type"`    // "message" | "function_call" | "reasoning"
	ID      string              `json:"id,omitempty"`
	Role    string              `json:"role,omitempty"`
	Content []OutputContentPart `json:"content,omitempty"`
	Status  string              `json:"status,omitempty"` // "completed" | "incomplete"
	// function_call fields
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallID    string `json:"call_id,omitempty"`
}

// OutputContentPart is one block inside an OutputItem.
type OutputContentPart struct {
	Type        string `json:"type"` // "output_text" | "refusal"
	Text        string `json:"text,omitempty"`
	Annotations []any  `json:"annotations,omitempty"`
}

// ResponseUsage mirrors the Responses API usage object.
type ResponseUsage struct {
	InputTokens         int                  `json:"input_tokens"`
	OutputTokens        int                  `json:"output_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	InputTokensDetails  *InputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *OutputTokensDetails `json:"output_tokens_details,omitempty"`
}

// InputTokensDetails holds cache-specific input token counts.
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// OutputTokensDetails holds reasoning-specific output token counts.
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ResponseError represents an error returned in the Responses API response.
type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ToCanonical converts a Responses API response to the canonical ChatResponse format.
func (r *Response) ToCanonical() *canonical.ChatResponse {
	created := r.CreatedAt
	if created == 0 {
		created = time.Now().Unix()
	}

	resp := &canonical.ChatResponse{
		ID:      r.ID,
		Object:  "chat.completion",
		Created: created,
		Model:   r.Model,
	}

	for i, item := range r.Output {
		if item.Type != "message" {
			continue
		}
		finishReason := "stop"
		if item.Status == "incomplete" {
			finishReason = "length"
		}
		resp.Choices = append(resp.Choices, canonical.Choice{
			Index: i,
			Message: canonical.Message{
				Role:    canonical.RoleAssistant,
				Content: extractOutputText(item),
			},
			FinishReason: finishReason,
		})
	}

	if r.Usage != nil {
		u := &canonical.Usage{
			PromptTokens:     r.Usage.InputTokens,
			CompletionTokens: r.Usage.OutputTokens,
			TotalTokens:      r.Usage.TotalTokens,
		}
		if r.Usage.InputTokensDetails != nil {
			u.CacheReadTokens = r.Usage.InputTokensDetails.CachedTokens
		}
		if r.Usage.OutputTokensDetails != nil {
			u.ReasoningTokens = r.Usage.OutputTokensDetails.ReasoningTokens
		}
		resp.Usage = u
	}

	return resp
}

func extractOutputText(item OutputItem) string {
	var sb strings.Builder
	for _, c := range item.Content {
		if c.Type == "output_text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}

package responses

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"uni-ai-api/backend/internal/formats/canonical"
)

// FromCanonicalResponse converts a canonical ChatResponse to a Responses API
// response body. Used when the client speaks Responses protocol (POST
// /v1/responses) regardless of what upstream protocol served the request.
func FromCanonicalResponse(resp *canonical.ChatResponse) *Response {
	out := &Response{
		ID:        ensureRespID(resp.ID),
		Object:    "response",
		CreatedAt: timeOrNow(resp.Created),
		Model:     resp.Model,
		Status:    "completed",
	}

	for i, choice := range resp.Choices {
		msg := choice.Message
		if text := msg.TextContent(); text != "" {
			out.Output = append(out.Output, OutputItem{
				Type:   "message",
				ID:     ensureMsgID(""),
				Role:   "assistant",
				Status: "completed",
				Content: []OutputContentPart{
					{Type: "output_text", Text: text},
				},
			})
		}
		for _, tc := range msg.ToolCalls {
			out.Output = append(out.Output, OutputItem{
				Type:      "function_call",
				ID:        ensureFCID(""),
				CallID:    ensureCallID(tc.ID),
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
				Status:    "completed",
			})
		}
		if choice.FinishReason == "length" && i == len(resp.Choices)-1 {
			out.Status = "incomplete"
		}
	}

	if resp.Usage != nil {
		out.Usage = &ResponseUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
		if resp.Usage.CacheReadTokens > 0 {
			out.Usage.InputTokensDetails = &InputTokensDetails{CachedTokens: resp.Usage.CacheReadTokens}
		}
		if resp.Usage.ReasoningTokens > 0 {
			out.Usage.OutputTokensDetails = &OutputTokensDetails{ReasoningTokens: resp.Usage.ReasoningTokens}
		}
	}
	return out
}

// MarshalResponse serialises a canonical response as Responses API JSON.
func MarshalResponse(resp *canonical.ChatResponse) ([]byte, error) {
	return json.Marshal(FromCanonicalResponse(resp))
}

func timeOrNow(t int64) int64 {
	if t > 0 {
		return t
	}
	return time.Now().Unix()
}

func ensureRespID(id string) string {
	if id != "" && len(id) >= 5 && id[:5] == "resp_" {
		return id
	}
	if id == "" {
		return "resp_" + randomID(24)
	}
	return "resp_" + id
}

func ensureMsgID(id string) string {
	if id != "" {
		return id
	}
	return "msg_" + randomID(24)
}

func ensureFCID(id string) string {
	if id != "" {
		return id
	}
	return "fc_" + randomID(24)
}

func ensureCallID(id string) string {
	if id != "" {
		return id
	}
	return "call_" + randomID(20)
}

func randomID(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

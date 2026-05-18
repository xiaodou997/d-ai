package claude

import (
	"encoding/json"

	"uni-ai-api/backend/internal/formats/canonical"
)

// MessagesResponseFromCanonical converts a canonical ChatResponse to the
// Anthropic Messages API response body. Used when the client speaks Anthropic
// protocol (POST /v1/messages) but the upstream may be any provider.
func MessagesResponseFromCanonical(resp *canonical.ChatResponse) *MessagesResponse {
	out := &MessagesResponse{
		ID:    ensureClaudeID(resp.ID),
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
	}

	if len(resp.Choices) == 0 {
		out.StopReason = "end_turn"
		out.Usage = usageToClaudeUsage(resp.Usage)
		return out
	}

	choice := resp.Choices[0]
	msg := choice.Message

	if text := msg.TextContent(); text != "" {
		out.Content = append(out.Content, Block{Type: "text", Text: text})
	}

	for _, tc := range msg.ToolCalls {
		input := json.RawMessage(tc.Function.Arguments)
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		out.Content = append(out.Content, Block{
			Type:  "tool_use",
			ID:    ensureToolUseID(tc.ID),
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	out.StopReason = openAIFinishToClaudeStop(choice.FinishReason, len(msg.ToolCalls) > 0)
	out.Usage = usageToClaudeUsage(resp.Usage)
	return out
}

// MarshalMessagesResponse serialises a canonical response as Anthropic JSON.
func MarshalMessagesResponse(resp *canonical.ChatResponse) ([]byte, error) {
	return json.Marshal(MessagesResponseFromCanonical(resp))
}

func openAIFinishToClaudeStop(reason string, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_use"
	}
	switch reason {
	case "stop", "":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "stop_sequence"
	default:
		return reason
	}
}

func usageToClaudeUsage(u *canonical.Usage) Usage {
	if u == nil {
		return Usage{}
	}
	return Usage{
		InputTokens:              u.PromptTokens,
		OutputTokens:             u.CompletionTokens,
		CacheCreationInputTokens: u.CacheWriteTokens,
		CacheReadInputTokens:     u.CacheReadTokens,
	}
}

func ensureClaudeID(id string) string {
	if id == "" {
		return "msg_" + randomHex(24)
	}
	if len(id) >= 4 && id[:4] == "msg_" {
		return id
	}
	return "msg_" + id
}

func ensureToolUseID(id string) string {
	if id == "" {
		return "toolu_" + randomHex(20)
	}
	if len(id) >= 6 && id[:6] == "toolu_" {
		return id
	}
	return "toolu_" + id
}

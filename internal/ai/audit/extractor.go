package audit

import (
	"encoding/json"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

// messageKey returns the JSON field that carries conversation messages for
// the given protocol.
func messageKey(protocol domain.UpstreamProtocol) string {
	switch protocol {
	case domain.ProtocolGeminiGenerate:
		return "contents"
	case domain.ProtocolGeminiEmbeddings:
		return "content"
	case domain.ProtocolOpenAIResponses:
		return "input"
	default: // openai_chat, openai_embeddings, anthropic_messages
		return "messages"
	}
}

// ExtractRequestPayload splits a raw request body into messages (the
// conversation content for the given protocol) and params (all remaining
// fields except "model" and "stream").
func ExtractRequestPayload(body []byte, protocol domain.UpstreamProtocol) (messages, params json.RawMessage) {
	if len(body) == 0 {
		return nil, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil || m == nil {
		return nil, nil
	}

	msgKey := messageKey(protocol)
	messages = m[msgKey]

	skip := map[string]bool{msgKey: true, "model": true, "stream": true}
	rest := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		if !skip[k] {
			rest[k] = v
		}
	}
	if len(rest) > 0 {
		b, _ := json.Marshal(rest)
		params = b
	}
	return
}

// ExtractSyncResponseMessage extracts the assistant reply from a sync upstream
// response body. Returns nil for embeddings, errors, or parse failures.
func ExtractSyncResponseMessage(body []byte, protocol domain.UpstreamProtocol) json.RawMessage {
	if len(body) == 0 {
		return nil
	}
	switch protocol {
	case domain.ProtocolOpenAIEmbeddings, domain.ProtocolGeminiEmbeddings:
		return nil
	case domain.ProtocolOpenAIChat:
		return extractOpenAIChatMessage(body)
	case domain.ProtocolOpenAIResponses:
		return extractOpenAIResponsesMessage(body)
	case domain.ProtocolAnthropicMessages:
		return extractAnthropicMessage(body)
	case domain.ProtocolGeminiGenerate:
		return extractGeminiMessage(body)
	default:
		return nil
	}
}

func extractOpenAIChatMessage(body []byte) json.RawMessage {
	var r struct {
		Choices []struct {
			Message json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &r); err != nil || len(r.Choices) == 0 {
		return nil
	}
	return r.Choices[0].Message
}

func extractOpenAIResponsesMessage(body []byte) json.RawMessage {
	var r struct {
		Output []json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(body, &r); err != nil || len(r.Output) == 0 {
		return nil
	}
	return r.Output[0]
}

func extractAnthropicMessage(body []byte) json.RawMessage {
	var r struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.Role == "" {
		return nil
	}
	out, _ := json.Marshal(map[string]json.RawMessage{
		"role":    json.RawMessage(`"` + r.Role + `"`),
		"content": r.Content,
	})
	return out
}

func extractGeminiMessage(body []byte) json.RawMessage {
	var r struct {
		Candidates []struct {
			Content json.RawMessage `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &r); err != nil || len(r.Candidates) == 0 {
		return nil
	}
	return r.Candidates[0].Content
}

// MaskAuthorization masks a bearer token, keeping only the last 8 characters.
// "Bearer sk-abc...xyz" → "Bearer ****...xyz" (last 8 of token preserved).
// Non-Bearer headers are returned as-is.
func MaskAuthorization(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return header
	}
	token := header[len(prefix):]
	if len(token) <= 8 {
		return header
	}
	return prefix + "****" + token[len(token)-8:]
}

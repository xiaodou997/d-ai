package riskcontrol

import (
	"encoding/json"
	"slices"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

// contentPartKey is the field each protocol's message objects use to carry
// content blocks: "content" for OpenAI Chat / Anthropic / OpenAI Responses,
// "parts" for Gemini.
func contentPartKey(protocol domain.UpstreamProtocol) string {
	if protocol == domain.ProtocolGeminiGenerate {
		return "parts"
	}
	return "content"
}

// ExtractLastUserText returns the plain-text content of the last user-role
// message in a request's extracted messages payload (as returned by
// audit.ExtractRequestPayload). v1 moderation is text-only: image/audio
// parts are skipped. Returns "" when no user text is found.
func ExtractLastUserText(protocol domain.UpstreamProtocol, messages json.RawMessage) string {
	if len(messages) == 0 {
		return ""
	}

	// OpenAI Responses allows `input` to be a bare string.
	var asString string
	if err := json.Unmarshal(messages, &asString); err == nil {
		return strings.TrimSpace(asString)
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(messages, &items); err != nil || len(items) == 0 {
		return ""
	}

	partKey := contentPartKey(protocol)
	for _, item := range slices.Backward(items) {
		role := stringField(item["role"])
		// Gemini/Responses items without an explicit role are treated as
		// user turns (single-turn requests commonly omit it).
		if role != "" && role != "user" {
			continue
		}
		if text := extractText(item[partKey]); text != "" {
			return text
		}
	}
	return ""
}

func stringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// extractText pulls plain text out of a content field that is either a bare
// string or an array of typed blocks/parts (e.g. [{"type":"text","text":".."}]
// or Gemini's [{"text":".."}]). Non-text blocks (images, tool calls, ...) are
// skipped.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}

	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, block := range blocks {
		blockType := stringField(block["type"])
		if blockType != "" && blockType != "text" && blockType != "input_text" {
			continue
		}
		text := stringField(block["text"])
		if text == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(text)
	}
	return strings.TrimSpace(sb.String())
}

package claude

import (
	"encoding/json"
	"strings"
)

// SanitizeOAuthRequestBody removes thinking/redacted_thinking blocks that lack
// a real signature, which the Claude Code OAuth endpoint rejects with 400.
// It returns the original bytes unchanged if no modification is needed.
func SanitizeOAuthRequestBody(body []byte) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body, nil
	}
	if obj == nil {
		return body, nil
	}

	thinkingEnabled := isThinkingEnabled(obj["thinking"])

	messages, ok := obj["messages"].([]any)
	if !ok {
		return body, nil
	}

	modified := false
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		content, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(content))
		for _, b := range content {
			block, ok := b.(map[string]any)
			if !ok {
				filtered = append(filtered, b)
				continue
			}
			if keepBlock(block, role, thinkingEnabled) {
				filtered = append(filtered, b)
			} else {
				modified = true
			}
		}
		if len(filtered) != len(content) {
			msg["content"] = filtered
		}
	}

	if !modified {
		return body, nil
	}
	return marshalStable(obj)
}

func isThinkingEnabled(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	t, _ := m["type"].(string)
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "enabled", "adaptive":
		return true
	}
	return false
}

func keepBlock(block map[string]any, role string, thinkingEnabled bool) bool {
	bt, _ := block["type"].(string)
	bt = strings.TrimSpace(bt)
	if bt == "thinking" || bt == "redacted_thinking" {
		if !thinkingEnabled || !strings.EqualFold(role, "assistant") {
			return false
		}
		sig, _ := block["signature"].(string)
		sig = strings.TrimSpace(sig)
		return sig != "" && sig != "skip_thought_signature_validator"
	}
	// Block without explicit type but carrying a stray "thinking" field — drop.
	if bt == "" {
		if _, has := block["thinking"]; has {
			return false
		}
	}
	return true
}

// marshalStable is a small helper to keep map key order stable in tests.
func marshalStable(v any) ([]byte, error) {
	return json.Marshal(v)
}

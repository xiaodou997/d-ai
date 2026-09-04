package promptaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"xiaodou/dai/internal/ai/privacy"
)

var ErrNoPrompt = errors.New("prompt audit request contains no text")

type segment struct{ Text, Role string }

func ExtractSnapshot(in Input, latestOnly bool) (Snapshot, error) {
	var root map[string]any
	if json.Unmarshal(in.Body, &root) != nil {
		return Snapshot{}, errors.New("prompt audit request JSON is invalid")
	}
	segments := extractSegments(strings.ToLower(in.Protocol), root)
	segments = normalizeSegments(segments)
	if latestOnly {
		segments = latestTurn(segments)
	} else {
		segments = latestUserFirst(segments)
	}
	if len(segments) == 0 {
		return Snapshot{}, ErrNoPrompt
	}
	texts := make([]string, 0, len(segments))
	for _, s := range segments {
		texts = append(texts, s.Text)
	}
	scanText := strings.Join(texts, "\n\n")
	sum := sha256.Sum256([]byte(scanText))
	return Snapshot{RequestID: in.RequestID, TenantID: in.TenantID, UserID: in.UserID, APIKeyID: in.APIKeyID, ModelCode: in.ModelCode, CapabilityType: in.CapabilityType, Protocol: in.Protocol, PromptHash: hex.EncodeToString(sum[:]), RedactedPreview: RedactedPreview(scanText), PromptLength: utf8.RuneCountInString(scanText), MessageCount: len(segments), ScanText: scanText}, nil
}

func extractSegments(protocol string, root map[string]any) []segment {
	if root == nil {
		return nil
	}
	switch protocol {
	case "openai_chat", "openai_chat_completions":
		return messageSegments(root["messages"])
	case "anthropic_messages":
		return append(systemSegments(root["system"]), messageSegments(root["messages"])...)
	case "openai_responses":
		return append(systemSegments(root["instructions"]), responseSegments(root["input"])...)
	case "gemini_generate", "gemini":
		return append(geminiSystemSegments(root), geminiSegments(root["contents"])...)
	case "openai_images":
		if text, _ := root["prompt"].(string); strings.TrimSpace(text) != "" {
			return []segment{{Text: text, Role: "user"}}
		}
	}
	if out := messageSegments(root["messages"]); len(out) > 0 {
		return out
	}
	return responseSegments(root["input"])
}

func messageSegments(value any) []segment {
	items, _ := value.([]any)
	out := make([]segment, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]any)
		role := strings.ToLower(stringValue(m["role"]))
		if !allowedRole(role) {
			continue
		}
		for _, text := range contentTexts(m["content"]) {
			out = append(out, segment{Text: text, Role: role})
		}
	}
	return out
}

func responseSegments(value any) []segment {
	switch v := value.(type) {
	case string:
		return []segment{{Text: v, Role: "user"}}
	case []any:
		out := []segment{}
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				role := strings.ToLower(stringValue(m["role"]))
				if role == "" {
					role = "user"
				}
				if !allowedRole(role) {
					continue
				}
				for _, text := range contentTexts(m["content"]) {
					out = append(out, segment{Text: text, Role: role})
				}
			}
		}
		return out
	}
	return nil
}

func geminiSegments(value any) []segment {
	items, _ := value.([]any)
	out := []segment{}
	for _, item := range items {
		m, _ := item.(map[string]any)
		role := strings.ToLower(stringValue(m["role"]))
		if role == "model" {
			role = "model"
		} else if role == "" {
			role = "user"
		}
		if !allowedRole(role) {
			continue
		}
		for _, text := range contentTexts(m["parts"]) {
			out = append(out, segment{Text: text, Role: role})
		}
	}
	return out
}
func geminiSystemSegments(root map[string]any) []segment {
	out := systemSegments(root["systemInstruction"])
	return append(out, systemSegments(root["system_instruction"])...)
}
func systemSegments(value any) []segment {
	out := []segment{}
	for _, text := range contentTexts(value) {
		out = append(out, segment{Text: text, Role: "system"})
	}
	return out
}

func contentTexts(value any) []string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			return []string{v}
		}
	case []any:
		out := []string{}
		for _, item := range v {
			out = append(out, contentTexts(item)...)
		}
		return out
	case map[string]any:
		if text, ok := v["text"].(string); ok && strings.TrimSpace(text) != "" {
			return []string{text}
		}
		if content, ok := v["content"]; ok {
			return contentTexts(content)
		}
	}
	return nil
}

func allowedRole(role string) bool {
	switch role {
	case "user", "system", "developer", "assistant", "tool", "model":
		return true
	}
	return false
}
func stringValue(v any) string { s, _ := v.(string); return strings.TrimSpace(s) }
func normalizeSegments(values []segment) []segment {
	out := make([]segment, 0, len(values))
	for _, s := range values {
		s.Text = strings.TrimSpace(s.Text)
		if s.Text != "" {
			out = append(out, s)
		}
	}
	return out
}
func latestUserFirst(values []segment) []segment {
	idx := -1
	for i := len(values) - 1; i >= 0; i-- {
		if values[i].Role == "user" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return values
	}
	return append([]segment{values[idx]}, append(append([]segment{}, values[:idx]...), values[idx+1:]...)...)
}
func latestTurn(values []segment) []segment {
	idx := -1
	for i := len(values) - 1; i >= 0; i-- {
		if values[i].Role == "user" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return values
	}
	out := []segment{values[idx]}
	for i := idx - 1; i >= 0; i-- {
		if values[i].Role == "assistant" || values[i].Role == "model" {
			return append(out, values[i])
		}
	}
	return out
}

func RedactedPreview(value string) string {
	return privacy.AuditPreview(value, 96)
}

func splitRunes(value string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	r := []rune(value)
	out := []string{}
	for start := 0; start < len(r); start += limit {
		end := start + limit
		if end > len(r) {
			end = len(r)
		}
		out = append(out, string(r[start:end]))
	}
	return out
}

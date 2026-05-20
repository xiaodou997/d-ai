package claude

import (
	"encoding/json"
	"sort"
	"strings"
)

// Claude OAuth (claude_oauth fixed provider) requires specific headers and body
// massaging because the OAuth token is only valid through the Claude Code CLI
// path. See Anthropic's claude-code OAuth contract.
const (
	claudeOAuthAnthropicVersion = "2023-06-01"
	claudeOAuthXApp             = "cli"
	claudeOAuthUserAgent        = "claude-cli/1.0.85 (external, cli)"
)

// requiredBetaTokens must always be present in anthropic-beta for OAuth requests.
var requiredBetaTokens = []string{
	"claude-code-20250219",
	"oauth-2025-04-20",
	"interleaved-thinking-2025-05-14",
}

// excludedBetaTokens must be stripped from incoming anthropic-beta because they
// are rejected when paired with the OAuth required tokens.
var excludedBetaTokens = map[string]struct{}{
	"context-1m-2025-08-07": {},
}

// ApplyOAuthHeaders injects the Claude Code OAuth-specific HTTP headers into
// the outgoing request. The caller is responsible for setting Authorization.
//
// incomingBeta is the value of the client's anthropic-beta header (may be "").
// Its tokens are merged with the required set, then excluded tokens are dropped.
func ApplyOAuthHeaders(headers map[string]string, incomingBeta string) {
	headers["anthropic-version"] = claudeOAuthAnthropicVersion
	headers["anthropic-beta"] = mergeBetaTokens(incomingBeta)
	headers["anthropic-dangerous-direct-browser-access"] = "true"
	headers["x-app"] = claudeOAuthXApp
	if _, ok := headers["user-agent"]; !ok {
		headers["user-agent"] = claudeOAuthUserAgent
	}
}

func mergeBetaTokens(incoming string) string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(requiredBetaTokens)+4)

	add := func(tok string) {
		t := strings.TrimSpace(tok)
		if t == "" {
			return
		}
		key := strings.ToLower(t)
		if _, bad := excludedBetaTokens[key]; bad {
			return
		}
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}

	for _, t := range requiredBetaTokens {
		add(t)
	}
	for t := range strings.SplitSeq(incoming, ",") {
		add(t)
	}
	return strings.Join(out, ",")
}

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
	// json.Marshal already orders map keys alphabetically; sort import kept
	// to make this deterministic even if future changes use a different path.
	_ = sort.Strings
	return json.Marshal(v)
}

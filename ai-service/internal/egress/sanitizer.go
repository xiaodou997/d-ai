package egress

import (
	"bytes"
	"encoding/json"
	"strings"

	"xiaodou/uni-ai-api/internal/domain"
)

// SanitizeJSON rewrites user-visible JSON according to policy. It is safe to
// call for every runtime response body: non-JSON and irrelevant payloads are
// returned unchanged.
func SanitizeJSON(body []byte, policy Policy) []byte {
	if !policy.IsConfigured() || len(body) == 0 || !maybeContainsModelIdentity(body, policy) {
		return body
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return body
	}

	if !sanitizeValue(payload, policy) {
		return body
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

// SanitizeText removes internal routing identity from runtime error messages.
func SanitizeText(text string, policy Policy) string {
	if !policy.IsConfigured() || text == "" {
		return text
	}
	out := text
	for _, term := range policy.SensitiveTerms() {
		out = strings.ReplaceAll(out, term, policy.PublicModel)
	}
	if policy.AllowVersionSuffix && policy.PublicModel != "" {
		out = replaceVersionedPublicModels(out, policy.PublicModel)
	}
	return out
}

func maybeContainsModelIdentity(body []byte, policy Policy) bool {
	if !bytes.Contains(body, []byte("model")) && !bytes.Contains(body, []byte("modelVersion")) {
		return false
	}
	if policy.UpstreamModel != "" && bytes.Contains(body, []byte(policy.UpstreamModel)) {
		return true
	}
	if policy.PublicModel != "" && bytes.Contains(body, []byte(policy.PublicModel)) {
		return true
	}
	for _, alias := range policy.Aliases {
		if alias != "" && bytes.Contains(body, []byte(alias)) {
			return true
		}
	}
	return false
}

func sanitizeValue(v any, policy Policy) bool {
	changed := false
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if isModelIdentityKey(k, policy.Protocol) {
				if s, ok := child.(string); ok && policy.IsModelIdentity(s) {
					x[k] = policy.PublicModel
					changed = true
					continue
				}
			}
			if sanitizeValue(child, policy) {
				changed = true
			}
		}
	case []any:
		for _, child := range x {
			if sanitizeValue(child, policy) {
				changed = true
			}
		}
	}
	return changed
}

func isModelIdentityKey(key string, protocol domain.UpstreamProtocol) bool {
	if key == "model" {
		return true
	}
	if key != modelResponseField(protocol) {
		return false
	}
	return protocol == domain.ProtocolGeminiGenerate || protocol == domain.ProtocolGeminiEmbeddings
}

func modelResponseField(protocol domain.UpstreamProtocol) string {
	switch protocol {
	case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
		return "modelVersion"
	default:
		return "model"
	}
}

func replaceVersionedPublicModels(text, publicModel string) string {
	var b strings.Builder
	i := 0
	for i < len(text) {
		idx := strings.Index(text[i:], publicModel+"-")
		if idx < 0 {
			b.WriteString(text[i:])
			break
		}
		idx += i
		b.WriteString(text[i:idx])
		j := idx + len(publicModel) + 1
		if j >= len(text) || text[j] < '0' || text[j] > '9' {
			b.WriteString(text[idx:j])
			i = j
			continue
		}
		for j < len(text) && isModelTokenChar(text[j]) {
			j++
		}
		b.WriteString(publicModel)
		i = j
	}
	return b.String()
}

func isModelTokenChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.'
}

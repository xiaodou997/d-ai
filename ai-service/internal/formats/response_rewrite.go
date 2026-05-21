package formats

import (
	"bytes"
	"encoding/json"
	"strings"

	"xiaodou/uni-ai-api/internal/domain"
)

// modelResponseField returns the JSON field name that carries the model
// identifier in an upstream response for the given protocol.
//
// Gemini uses "modelVersion"; every other protocol uses "model".
func modelResponseField(protocol domain.UpstreamProtocol) string {
	switch protocol {
	case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
		return "modelVersion"
	default:
		return "model"
	}
}

// SanitizePublicModelJSON replaces upstream model identifiers in a user-visible
// JSON response with publicModel. It recursively walks the payload so nested
// envelopes such as OpenAI Responses streaming events
// {"response":{"model":"..."}} are covered by the same logic as top-level
// Chat Completions responses.
//
// Returns body unchanged when:
//   - publicModel is empty
//   - upstreamModel/publicModel are absent from body
//   - the body does not parse as JSON
//   - no model identity field equals upstreamModel
//
// If upstreamModel is empty, any non-public model identity field is rewritten.
// Runtime call sites should pass the selected candidate's upstream model to
// avoid touching unrelated model-like values.
func SanitizePublicModelJSON(body []byte, publicModel, upstreamModel string, protocol domain.UpstreamProtocol) []byte {
	if publicModel == "" || len(body) == 0 {
		return body
	}
	if upstreamModel != "" &&
		!bytes.Contains(body, []byte(upstreamModel)) &&
		!bytes.Contains(body, []byte(publicModel)) {
		return body
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return body
	}

	if !sanitizePublicModelValue(payload, publicModel, upstreamModel, protocol) {
		return body
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

// RewriteSyncResponseModel is kept for tests and legacy callers. Runtime paths
// should call SanitizePublicModelJSON with a non-empty upstreamModel.
func RewriteSyncResponseModel(body []byte, publicModel string, protocol domain.UpstreamProtocol) []byte {
	return SanitizePublicModelJSON(body, publicModel, "", protocol)
}

// PublicModelSanitizer applies the same public model rewrite to every
// user-visible SSE data payload. It is stateless by design: each frame is
// inspected independently so a first frame without a model field cannot disable
// rewriting for later frames.
type PublicModelSanitizer struct {
	publicModel   string
	upstreamModel string
	protocol      domain.UpstreamProtocol
}

// NewPublicModelSanitizer creates a sanitizer for sync/stream response payloads.
func NewPublicModelSanitizer(publicModel, upstreamModel string, protocol domain.UpstreamProtocol) *PublicModelSanitizer {
	return &PublicModelSanitizer{
		publicModel:   publicModel,
		upstreamModel: upstreamModel,
		protocol:      protocol,
	}
}

// Sanitize returns data with upstream model identity fields replaced by the
// public model name.
func (s *PublicModelSanitizer) Sanitize(data []byte) []byte {
	return SanitizePublicModelJSON(data, s.publicModel, s.upstreamModel, s.protocol)
}

func sanitizePublicModelValue(v any, publicModel, upstreamModel string, protocol domain.UpstreamProtocol) bool {
	changed := false
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if isModelIdentityKey(k, protocol) {
				if s, ok := child.(string); ok && shouldRewriteModelValue(s, publicModel, upstreamModel) {
					x[k] = publicModel
					changed = true
					continue
				}
			}
			if sanitizePublicModelValue(child, publicModel, upstreamModel, protocol) {
				changed = true
			}
		}
	case []any:
		for _, child := range x {
			if sanitizePublicModelValue(child, publicModel, upstreamModel, protocol) {
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

func shouldRewriteModelValue(value, publicModel, upstreamModel string) bool {
	if value == "" || value == publicModel {
		return false
	}
	if upstreamModel == "" {
		return true
	}
	return value == upstreamModel || isVersionedPublicModel(value, publicModel)
}

func isVersionedPublicModel(value, publicModel string) bool {
	if publicModel == "" || !strings.HasPrefix(value, publicModel+"-") {
		return false
	}
	suffix := strings.TrimPrefix(value, publicModel+"-")
	if suffix == "" {
		return false
	}
	// Treat date/snapshot suffixes as provider-specific versions of the public
	// model. This covers values like gpt-5.4-mini-2026-03-17 and
	// claude-sonnet-4-20250514 without folding names like gpt-4o into gpt-4.
	return suffix[0] >= '0' && suffix[0] <= '9'
}

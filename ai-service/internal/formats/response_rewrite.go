package formats

import (
	"bytes"
	"encoding/json"

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

// RewriteSyncResponseModel replaces the model identifier in a sync upstream
// response body with publicModel so the client always sees the logical model
// name it requested, never the upstream-internal name.
//
// Returns body unchanged when:
//   - publicModel is empty
//   - the body does not parse as a JSON object
//   - the body already contains publicModel in the relevant field
//
// Protocol-aware field selection:
//   - Gemini: "modelVersion"
//   - All others: "model"
//
// Unknown fields are preserved via map[string]json.RawMessage round-trip so
// vendor-specific extensions (cache_control, usage, etc.) are not dropped.
func RewriteSyncResponseModel(body []byte, publicModel string, protocol domain.UpstreamProtocol) []byte {
	if publicModel == "" || len(body) == 0 {
		return body
	}
	key := modelResponseField(protocol)

	// Fast path: already carries the public model name → zero allocation.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil || probe == nil {
		return body
	}

	actualRaw, ok := probe[key]
	if !ok {
		// Anthropic streaming wraps the model inside a "message" object.
		// For sync responses the field IS top-level, so this branch handles
		// edge cases where the field is absent (e.g. error bodies).
		if msgRaw, mok := probe["message"]; mok {
			var msg map[string]json.RawMessage
			if json.Unmarshal(msgRaw, &msg) == nil {
				actualRaw, ok = msg[key]
			}
		}
		if !ok {
			return body
		}
	}

	var actualModel string
	if err := json.Unmarshal(actualRaw, &actualModel); err != nil || actualModel == publicModel {
		return body
	}

	newVal, err := json.Marshal(publicModel)
	if err != nil {
		return body
	}
	probe[key] = newVal
	out, err := json.Marshal(probe)
	if err != nil {
		return body
	}
	return out
}

// StreamModelRewriter rewrites the model identifier field in SSE data payloads
// (the bytes after "data: ", without the newline).
//
// On the first call it JSON-parses the payload to discover the actual model
// value the upstream is sending. Subsequent calls use a pre-built byte
// replacement so JSON parsing is paid only once per stream.
//
// Usage:
//
//	rw := NewStreamModelRewriter(req.ModelCode, cand.Protocol)
//	// inside the forward loop:
//	finalData := rw.Rewrite(unwrappedData)
type StreamModelRewriter struct {
	publicModel string
	protocol    domain.UpstreamProtocol

	// Set after initialization.
	initialized bool
	noop        bool   // true when no rewrite is needed for this stream
	oldKV       []byte // e.g. `"model":"gpt-4-turbo-preview"`
	newKV       []byte // e.g. `"model":"gpt-4"`
	oldKVSpace  []byte // e.g. `"model": "gpt-4-turbo-preview"` (pretty-printed upstreams)
	newKVSpace  []byte // e.g. `"model": "gpt-4"`
}

// NewStreamModelRewriter creates a rewriter that will replace the upstream
// model name with publicModel in every streaming data frame.
func NewStreamModelRewriter(publicModel string, protocol domain.UpstreamProtocol) *StreamModelRewriter {
	return &StreamModelRewriter{
		publicModel: publicModel,
		protocol:    protocol,
	}
}

// Rewrite returns data with the model field replaced by the public model name.
// Returns data unchanged when:
//   - publicModel is empty
//   - the model field is absent in the first frame
//   - the upstream already sends the public model name
func (r *StreamModelRewriter) Rewrite(data []byte) []byte {
	if r.publicModel == "" {
		return data
	}
	if r.noop {
		return data
	}
	if !r.initialized {
		r.init(data)
	}
	if r.noop {
		return data
	}

	// Try compact form first (most APIs), then pretty-printed.
	if out := bytes.Replace(data, r.oldKV, r.newKV, 1); !bytes.Equal(out, data) {
		return out
	}
	return bytes.Replace(data, r.oldKVSpace, r.newKVSpace, 1)
}

// init discovers the upstream model value from the first data frame and
// pre-computes the byte patterns used for subsequent replacements.
func (r *StreamModelRewriter) init(data []byte) {
	r.initialized = true

	key := modelResponseField(r.protocol)
	actualModel := extractModelFromData(data, key)
	if actualModel == "" {
		// No model field in this frame (e.g. Anthropic content_block_delta).
		// Mark noop so we skip JSON probing on every subsequent frame. If a
		// later frame carries the field the byte-replace will simply not match
		// (safe — we will never have initialized the KV pairs).
		r.noop = true
		return
	}
	if actualModel == r.publicModel {
		r.noop = true
		return
	}

	keyJSON, err := json.Marshal(key)
	if err != nil {
		r.noop = true
		return
	}
	oldValJSON, err := json.Marshal(actualModel)
	if err != nil {
		r.noop = true
		return
	}
	newValJSON, err := json.Marshal(r.publicModel)
	if err != nil {
		r.noop = true
		return
	}

	// Compact:  "model":"gpt-4-turbo-preview"
	r.oldKV = concat(keyJSON, []byte{':'}, oldValJSON)
	r.newKV = concat(keyJSON, []byte{':'}, newValJSON)
	// Space:    "model": "gpt-4-turbo-preview"
	r.oldKVSpace = concat(keyJSON, []byte{':', ' '}, oldValJSON)
	r.newKVSpace = concat(keyJSON, []byte{':', ' '}, newValJSON)
}

// extractModelFromData probes a JSON data payload for the model identifier.
// It checks the top-level key first, then falls back to the nested
// "message" object used by Anthropic's message_start streaming event.
func extractModelFromData(data []byte, key string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil || obj == nil {
		return ""
	}

	if raw, ok := obj[key]; ok {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}

	// Anthropic: {"type":"message_start","message":{"model":"claude-..."}}
	if msgRaw, ok := obj["message"]; ok {
		var msg map[string]json.RawMessage
		if json.Unmarshal(msgRaw, &msg) == nil {
			if raw, ok := msg[key]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil {
					return s
				}
			}
		}
	}
	return ""
}

func concat(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

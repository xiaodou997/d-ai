package formats

import (
	"encoding/json"
	"fmt"
)

// RequestMeta is the minimum we parse from a client request body for routing
// purposes — model code + streaming flag — without consuming or normalising
// the rest of the body. The original bytes are preserved verbatim for
// passthrough to the upstream (subject only to RewriteModel and protocol-
// specific transforms like Codex / Claude OAuth body sanitisation).
type RequestMeta struct {
	Model  string
	Stream bool
}

// ParseRequestMeta extracts the `model` and `stream` fields from a client
// request body. We deliberately ignore every other field — strict 1:1
// routing means the body is passed through to the upstream as-is, and the
// gateway has no business semantically re-interpreting things like
// `messages`, `input`, `system`, tool definitions, prompt caching markers, etc.
func ParseRequestMeta(body []byte) (RequestMeta, error) {
	if len(body) == 0 {
		return RequestMeta{}, fmt.Errorf("empty request body")
	}
	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return RequestMeta{}, fmt.Errorf("parse request meta: %w", err)
	}
	return RequestMeta{Model: meta.Model, Stream: meta.Stream}, nil
}

// RewriteModel returns a new body whose top-level `model` field equals
// upstreamModel. If the body already declares that model (or upstreamModel
// is empty), the original bytes are returned unchanged so passthrough is
// strict-zero-copy in the common case.
//
// Unknown fields are preserved by using json.RawMessage for everything but
// `model` — round-tripping through a typed struct would silently drop fields
// like Anthropic's cache_control, Codex's previous_response_id, Gemini's
// safetySettings / responseSchema, etc.
func RewriteModel(body []byte, upstreamModel string) ([]byte, error) {
	if upstreamModel == "" {
		return body, nil
	}
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, fmt.Errorf("rewrite model: %w", err)
	}
	if probe.Model == upstreamModel {
		return body, nil
	}
	// Decode into an ordered key→raw map so we can replace `model` without
	// touching anything else. encoding/json's map ordering is undefined, but
	// upstream APIs are not field-order sensitive in practice.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("rewrite model: %w", err)
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	raw, err := json.Marshal(upstreamModel)
	if err != nil {
		return nil, err
	}
	fields["model"] = raw
	return json.Marshal(fields)
}

// ApplyCodexRequestModifications mutates a Responses-API request body in
// place (returns a new slice) to match Codex's upstream contract:
//   - Strip max_output_tokens / temperature / top_p (Codex rejects them).
//   - Force store=false (Codex does not support persistent storage).
//   - Default instructions to "You are ChatGPT." when caller omits them.
//
// The function operates on raw bytes via a generic map so that arbitrary
// Responses-API fields the client supplied (previous_response_id, reasoning,
// tools, etc.) are preserved untouched.
func ApplyCodexRequestModifications(body []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("codex modify: %w", err)
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	delete(fields, "max_output_tokens")
	delete(fields, "temperature")
	delete(fields, "top_p")

	storeFalse, _ := json.Marshal(false)
	fields["store"] = storeFalse

	if _, ok := fields["instructions"]; !ok {
		defaultInstr, _ := json.Marshal("You are ChatGPT.")
		fields["instructions"] = defaultInstr
	}
	return json.Marshal(fields)
}

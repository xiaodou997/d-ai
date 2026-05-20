package formats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strconv"
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
// request body. JSON endpoints carry these as top-level fields; image edit
// requests carry `model` as a multipart form field. We deliberately ignore
// every other field — strict 1:1 routing means the body is passed through to
// the upstream as-is, and the gateway has no business semantically
// re-interpreting things like `messages`, `input`, `system`, tool definitions,
// prompt caching markers, uploaded image parts, etc.
func ParseRequestMeta(body []byte, contentType string) (RequestMeta, error) {
	if len(body) == 0 {
		return RequestMeta{}, fmt.Errorf("empty request body")
	}
	if isMultipartForm(contentType) {
		return parseMultipartRequestMeta(body, contentType)
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
func RewriteModel(body []byte, upstreamModel, contentType string) ([]byte, error) {
	if upstreamModel == "" {
		return body, nil
	}
	if isMultipartForm(contentType) {
		return rewriteMultipartModel(body, upstreamModel, contentType)
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

// MultipartScalarFields extracts small non-file form values without retaining
// uploaded file content. It is intended for request metadata and billing hints
// such as model, n and size.
func MultipartScalarFields(body []byte, contentType string, maxValueBytes int64) (map[string]string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("parse multipart content type: %w", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("missing multipart boundary")
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	out := map[string]string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart part: %w", err)
		}
		name := part.FormName()
		if name == "" || part.FileName() != "" {
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		value, readErr := io.ReadAll(io.LimitReader(part, maxValueBytes+1))
		_ = part.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read multipart field %q: %w", name, readErr)
		}
		if int64(len(value)) > maxValueBytes {
			return nil, fmt.Errorf("multipart field %q exceeds metadata limit", name)
		}
		if _, exists := out[name]; !exists {
			out[name] = string(value)
		}
	}
}

func parseMultipartRequestMeta(body []byte, contentType string) (RequestMeta, error) {
	fields, err := MultipartScalarFields(body, contentType, 1<<20)
	if err != nil {
		return RequestMeta{}, err
	}
	stream, _ := strconv.ParseBool(fields["stream"])
	return RequestMeta{Model: fields["model"], Stream: stream}, nil
}

func rewriteMultipartModel(body []byte, upstreamModel, contentType string) ([]byte, error) {
	meta, err := parseMultipartRequestMeta(body, contentType)
	if err != nil {
		return nil, fmt.Errorf("rewrite multipart model: %w", err)
	}
	if meta.Model == upstreamModel {
		return body, nil
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("rewrite multipart model: %w", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("rewrite multipart model: missing multipart boundary")
	}

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	var out bytes.Buffer
	mw := multipart.NewWriter(&out)
	if err := mw.SetBoundary(boundary); err != nil {
		return nil, fmt.Errorf("rewrite multipart model: preserve boundary: %w", err)
	}

	modelWritten := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = mw.Close()
			return nil, fmt.Errorf("rewrite multipart model: read part: %w", err)
		}
		hdr := cloneMIMEHeader(part.Header)
		dst, err := mw.CreatePart(hdr)
		if err != nil {
			_ = part.Close()
			_ = mw.Close()
			return nil, fmt.Errorf("rewrite multipart model: create part: %w", err)
		}
		if part.FormName() == "model" && part.FileName() == "" {
			_, err = io.WriteString(dst, upstreamModel)
			modelWritten = true
		} else {
			_, err = io.Copy(dst, part)
		}
		_ = part.Close()
		if err != nil {
			_ = mw.Close()
			return nil, fmt.Errorf("rewrite multipart model: copy part: %w", err)
		}
	}
	if !modelWritten {
		if err := mw.WriteField("model", upstreamModel); err != nil {
			_ = mw.Close()
			return nil, fmt.Errorf("rewrite multipart model: add model: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("rewrite multipart model: close writer: %w", err)
	}
	return out.Bytes(), nil
}

func isMultipartForm(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "multipart/form-data"
}

func cloneMIMEHeader(h textproto.MIMEHeader) textproto.MIMEHeader {
	out := make(textproto.MIMEHeader, len(h))
	for k, values := range h {
		out[k] = append([]string(nil), values...)
	}
	return out
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

package riskcontrol

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"strings"
)

// promptField is the request field carrying the user-authored text of an image
// request. Both generation and edit use `prompt` on both transports
// (see internal/imageedit/codec.go).
const promptField = "prompt"

// ExtractImagePrompt returns the user-authored text of an image
// generation/edit request. Both transports are handled because both reach the
// runtime: JSON (`{"model":...,"prompt":...,"images":[...]}`) and
// multipart/form-data (OpenAI's native images.edit shape).
//
// Why this exists: image requests carry their text in `prompt`, not in
// `messages`, so the conversation extractor (audit.ExtractRequestPayload keyed
// by protocol) returns nothing for them — which silently turned the content
// moderation step into a no-op for every generated and edited image. Text is
// returned untruncated: moderation matches keywords over the whole prompt, and
// a truncated tail is a bypass.
func ExtractImagePrompt(body []byte, contentType string) string {
	if len(body) == 0 {
		return ""
	}
	if isMultipart(contentType) {
		return extractMultipartPrompt(body, contentType)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return ""
	}
	return jsonString(doc[promptField])
}

func isMultipart(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return strings.HasPrefix(mediaType, "multipart/")
}

// extractMultipartPrompt walks the form parts and returns the first prompt
// field. File parts are drained to io.Discard rather than buffered: an edit
// request carries full-size images and the reader has to be advanced part by
// part, but their bytes are of no interest here.
//
// A part named `prompt` that carries a filename is skipped rather than read,
// which matches the decoder: imageedit rejects any file field other than
// `image[]`/`mask` outright ("unsupported multipart file field"), so such a
// part never reaches an upstream and moderation would only be buffering an
// arbitrarily large upload into memory — moderation runs before the request is
// decoded.
func extractMultipartPrompt(body []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" {
		return ""
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err != nil {
			return ""
		}
		if part.FileName() != "" || part.FormName() != promptField {
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		payload, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil {
			return ""
		}
		if text := strings.TrimSpace(string(payload)); text != "" {
			return text
		}
	}
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

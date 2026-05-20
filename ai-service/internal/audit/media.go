package audit

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"xiaodou/uni-ai-api/internal/domain"
)

// MediaBlob is a single media item extracted from a request/response payload.
// Data is the decoded binary content; SHA256 is its hex-encoded digest.
type MediaBlob struct {
	SHA256      string
	Data        []byte
	ContentType string // e.g. "image/png", "audio/mpeg"
}

// Placeholder returns the canonical reference string written into the JSON
// in place of the original base64 data.
func (b *MediaBlob) Placeholder() string {
	return "audit-blob:sha256:" + b.SHA256
}

// MediaRef is serialised into the media_refs JSONB column so callers can
// look up which blobs were extracted from a given record.
type MediaRef struct {
	Placeholder string `json:"placeholder"`
	ContentType string `json:"content_type"`
	SHA256      string `json:"sha256"`
}

// ExtractFromMessages replaces inline base64 media in the messages JSON with
// audit-blob:sha256:... placeholders, returning the modified JSON and the
// extracted blobs. Unparseable input is returned unmodified with no blobs.
func ExtractFromMessages(messages json.RawMessage, protocol domain.UpstreamProtocol) (json.RawMessage, []MediaBlob) {
	if len(messages) == 0 {
		return messages, nil
	}
	switch protocol {
	case domain.ProtocolOpenAIChat, domain.ProtocolOpenAICompletions, domain.ProtocolOpenAIResponses:
		return extractOAIMessages(messages)
	case domain.ProtocolAnthropicMessages:
		return extractAnthropicMessages(messages)
	case domain.ProtocolGeminiGenerate:
		return extractGeminiContents(messages)
	default:
		return messages, nil
	}
}

// ExtractFromImagesResponse extracts b64_json images from an OpenAI Images
// API response body and replaces them with placeholders.
func ExtractFromImagesResponse(body json.RawMessage) (json.RawMessage, []MediaBlob) {
	if len(body) == 0 {
		return body, nil
	}
	return extractImagesResponse(body)
}

// BuildMediaRefs converts extracted blobs into the JSON array stored in
// the media_refs JSONB column.
func BuildMediaRefs(blobs []MediaBlob) json.RawMessage {
	if len(blobs) == 0 {
		return nil
	}
	refs := make([]MediaRef, len(blobs))
	for i, b := range blobs {
		refs[i] = MediaRef{Placeholder: b.Placeholder(), ContentType: b.ContentType, SHA256: b.SHA256}
	}
	out, _ := json.Marshal(refs)
	return out
}

// ============================================================================
// OpenAI Chat / Completions / Responses
// ============================================================================

func extractOAIMessages(messages json.RawMessage) (json.RawMessage, []MediaBlob) {
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(messages, &msgs); err != nil {
		return messages, nil
	}

	var blobs []MediaBlob
	for i, msg := range msgs {
		contentRaw, ok := msg["content"]
		if !ok {
			continue
		}
		var parts []map[string]json.RawMessage
		if err := json.Unmarshal(contentRaw, &parts); err != nil {
			continue // string content — no media
		}
		changed := false
		for j, part := range parts {
			var partType string
			if err := json.Unmarshal(part["type"], &partType); err != nil {
				continue
			}
			switch partType {
			case "image_url":
				if b, newPart := extractOAIImageURL(part); b != nil {
					blobs = append(blobs, *b)
					parts[j] = newPart
					changed = true
				}
			case "input_audio":
				if b, newPart := extractOAIInputAudio(part); b != nil {
					blobs = append(blobs, *b)
					parts[j] = newPart
					changed = true
				}
			}
		}
		if changed {
			if raw, err := json.Marshal(parts); err == nil {
				msg["content"] = raw
				msgs[i] = msg
			}
		}
	}

	if len(blobs) == 0 {
		return messages, nil
	}
	out, err := json.Marshal(msgs)
	if err != nil {
		return messages, blobs
	}
	return out, blobs
}

func extractOAIImageURL(part map[string]json.RawMessage) (*MediaBlob, map[string]json.RawMessage) {
	imgRaw, ok := part["image_url"]
	if !ok {
		return nil, nil
	}
	var imgURL struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"`
	}
	if err := json.Unmarshal(imgRaw, &imgURL); err != nil {
		return nil, nil
	}
	blob, placeholder := extractDataURI(imgURL.URL)
	if blob == nil {
		return nil, nil
	}
	imgURL.URL = placeholder
	newImgRaw, _ := json.Marshal(imgURL)
	p := copyMap(part)
	p["image_url"] = newImgRaw
	return blob, p
}

func extractOAIInputAudio(part map[string]json.RawMessage) (*MediaBlob, map[string]json.RawMessage) {
	audioRaw, ok := part["input_audio"]
	if !ok {
		return nil, nil
	}
	var audio struct {
		Data   string `json:"data"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(audioRaw, &audio); err != nil || audio.Data == "" {
		return nil, nil
	}
	data, err := decodeBase64(audio.Data)
	if err != nil {
		return nil, nil
	}
	blob := makeBlob(data, audioFormatToMIME(audio.Format))
	audio.Data = blob.Placeholder()
	newAudioRaw, _ := json.Marshal(audio)
	p := copyMap(part)
	p["input_audio"] = newAudioRaw
	return blob, p
}

// ============================================================================
// Anthropic Messages
// ============================================================================

func extractAnthropicMessages(messages json.RawMessage) (json.RawMessage, []MediaBlob) {
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(messages, &msgs); err != nil {
		return messages, nil
	}

	var blobs []MediaBlob
	for i, msg := range msgs {
		contentRaw, ok := msg["content"]
		if !ok {
			continue
		}
		var blocks []map[string]json.RawMessage
		if err := json.Unmarshal(contentRaw, &blocks); err != nil {
			continue
		}
		changed := false
		for j, block := range blocks {
			var blockType string
			_ = json.Unmarshal(block["type"], &blockType)
			if blockType != "image" && blockType != "document" {
				continue
			}
			if b, newBlock := extractAnthropicSource(block); b != nil {
				blobs = append(blobs, *b)
				blocks[j] = newBlock
				changed = true
			}
		}
		if changed {
			if raw, err := json.Marshal(blocks); err == nil {
				msg["content"] = raw
				msgs[i] = msg
			}
		}
	}

	if len(blobs) == 0 {
		return messages, nil
	}
	out, err := json.Marshal(msgs)
	if err != nil {
		return messages, blobs
	}
	return out, blobs
}

func extractAnthropicSource(block map[string]json.RawMessage) (*MediaBlob, map[string]json.RawMessage) {
	sourceRaw, ok := block["source"]
	if !ok {
		return nil, nil
	}
	var source struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil || source.Type != "base64" {
		return nil, nil
	}
	data, err := decodeBase64(source.Data)
	if err != nil {
		return nil, nil
	}
	blob := makeBlob(data, source.MediaType)
	source.Data = blob.Placeholder()
	newSourceRaw, _ := json.Marshal(source)
	b := copyMap(block)
	b["source"] = newSourceRaw
	return blob, b
}

// ============================================================================
// Gemini Generate
// ============================================================================

func extractGeminiContents(contents json.RawMessage) (json.RawMessage, []MediaBlob) {
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(contents, &msgs); err != nil {
		return contents, nil
	}

	var blobs []MediaBlob
	for i, msg := range msgs {
		partsRaw, ok := msg["parts"]
		if !ok {
			continue
		}
		var parts []map[string]json.RawMessage
		if err := json.Unmarshal(partsRaw, &parts); err != nil {
			continue
		}
		changed := false
		for j, part := range parts {
			inlineRaw, ok := part["inline_data"]
			if !ok {
				continue
			}
			if b, newPart := extractGeminiInlineData(inlineRaw, part); b != nil {
				blobs = append(blobs, *b)
				parts[j] = newPart
				changed = true
			}
		}
		if changed {
			if raw, err := json.Marshal(parts); err == nil {
				msg["parts"] = raw
				msgs[i] = msg
			}
		}
	}

	if len(blobs) == 0 {
		return contents, nil
	}
	out, err := json.Marshal(msgs)
	if err != nil {
		return contents, blobs
	}
	return out, blobs
}

func extractGeminiInlineData(inlineRaw json.RawMessage, part map[string]json.RawMessage) (*MediaBlob, map[string]json.RawMessage) {
	var inline struct {
		MIMEType string `json:"mime_type"`
		Data     string `json:"data"`
	}
	if err := json.Unmarshal(inlineRaw, &inline); err != nil || inline.Data == "" {
		return nil, nil
	}
	data, err := decodeBase64(inline.Data)
	if err != nil {
		return nil, nil
	}
	blob := makeBlob(data, inline.MIMEType)
	inline.Data = blob.Placeholder()
	newInlineRaw, _ := json.Marshal(inline)
	p := copyMap(part)
	p["inline_data"] = newInlineRaw
	return blob, p
}

// ============================================================================
// OpenAI Images response
// ============================================================================

func extractImagesResponse(body json.RawMessage) (json.RawMessage, []MediaBlob) {
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(body, &resp); err != nil {
		return body, nil
	}
	dataRaw, ok := resp["data"]
	if !ok {
		return body, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(dataRaw, &items); err != nil {
		return body, nil
	}

	var blobs []MediaBlob
	changed := false
	for i, item := range items {
		b64Raw, ok := item["b64_json"]
		if !ok {
			continue
		}
		var b64 string
		if err := json.Unmarshal(b64Raw, &b64); err != nil || b64 == "" {
			continue
		}
		data, err := decodeBase64(b64)
		if err != nil {
			continue
		}
		blob := makeBlob(data, "image/png")
		newItem := copyMap(item)
		newItem["b64_json"], _ = json.Marshal(blob.Placeholder())
		items[i] = newItem
		blobs = append(blobs, *blob)
		changed = true
	}

	if !changed {
		return body, nil
	}
	newDataRaw, err := json.Marshal(items)
	if err != nil {
		return body, blobs
	}
	resp["data"] = newDataRaw
	out, err := json.Marshal(resp)
	if err != nil {
		return body, blobs
	}
	return out, blobs
}

// ============================================================================
// Helpers
// ============================================================================

func makeBlob(data []byte, contentType string) *MediaBlob {
	h := sha256.Sum256(data)
	return &MediaBlob{
		SHA256:      fmt.Sprintf("%x", h),
		Data:        data,
		ContentType: contentType,
	}
}

// extractDataURI parses a "data:TYPE;base64,DATA" URI.
// Returns (nil,"") for non-data-URI or malformed input.
func extractDataURI(uri string) (*MediaBlob, string) {
	if !strings.HasPrefix(uri, "data:") {
		return nil, ""
	}
	rest := uri[5:]
	semi := strings.IndexByte(rest, ';')
	if semi < 0 {
		return nil, ""
	}
	contentType := rest[:semi]
	after := rest[semi+1:]
	comma := strings.IndexByte(after, ',')
	if comma < 0 || after[:comma] != "base64" {
		return nil, ""
	}
	data, err := decodeBase64(after[comma+1:])
	if err != nil {
		return nil, ""
	}
	blob := makeBlob(data, contentType)
	return blob, blob.Placeholder()
}

// decodeBase64 tries StdEncoding then RawStdEncoding.
func decodeBase64(s string) ([]byte, error) {
	if data, err := base64.StdEncoding.DecodeString(s); err == nil {
		return data, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func audioFormatToMIME(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "ogg":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	case "aac":
		return "audio/aac"
	default:
		if format != "" {
			return "audio/" + format
		}
		return "audio/mpeg"
	}
}

func copyMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

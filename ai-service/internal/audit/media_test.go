package audit

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

// ============================================================================
// Helpers
// ============================================================================

func b64(data []byte) string { return base64.StdEncoding.EncodeToString(data) }

func hexSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

func dataURI(contentType string, data []byte) string {
	return "data:" + contentType + ";base64," + b64(data)
}

func mustUnmarshalBlob(t *testing.T, messages json.RawMessage, path ...string) string {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(messages, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cur := v
	for _, key := range path {
		switch c := cur.(type) {
		case map[string]interface{}:
			cur = c[key]
		case []interface{}:
			if key == "0" {
				cur = c[0]
			} else if key == "1" {
				cur = c[1]
			}
		}
	}
	s, ok := cur.(string)
	if !ok {
		t.Fatalf("expected string at path %v, got %T: %v", path, cur, cur)
	}
	return s
}

// ============================================================================
// OpenAI Chat — image_url data URI
// ============================================================================

func TestExtractOAIImageURL(t *testing.T) {
	imgData := []byte("fake-png-bytes")
	uri := dataURI("image/png", imgData)
	messages := json.RawMessage(`[{"role":"user","content":[{"type":"image_url","image_url":{"url":"` + uri + `"}}]}]`)

	out, blobs := ExtractFromMessages(messages, domain.ProtocolOpenAIChat)

	if len(blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(blobs))
	}
	b := blobs[0]
	if b.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", b.ContentType)
	}
	if b.SHA256 != hexSHA256(imgData) {
		t.Errorf("SHA256 mismatch")
	}
	// Placeholder must be written into the JSON
	placeholder := mustUnmarshalBlob(t, out, "0", "content", "0", "image_url", "url")
	if placeholder != b.Placeholder() {
		t.Errorf("placeholder in JSON = %q, want %q", placeholder, b.Placeholder())
	}
}

func TestExtractOAIImageURL_HTTPURLUnchanged(t *testing.T) {
	messages := json.RawMessage(`[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/img.png"}}]}]`)
	out, blobs := ExtractFromMessages(messages, domain.ProtocolOpenAIChat)
	if len(blobs) != 0 {
		t.Fatalf("expected 0 blobs for HTTP URL, got %d", len(blobs))
	}
	if string(out) != string(messages) {
		t.Errorf("messages should be unmodified for HTTP URLs")
	}
}

// ============================================================================
// OpenAI Chat — input_audio
// ============================================================================

func TestExtractOAIInputAudio(t *testing.T) {
	audioData := []byte("fake-audio-bytes")
	messages := json.RawMessage(`[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"` + b64(audioData) + `","format":"mp3"}}]}]`)

	_, blobs := ExtractFromMessages(messages, domain.ProtocolOpenAIChat)

	if len(blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(blobs))
	}
	if blobs[0].ContentType != "audio/mpeg" {
		t.Errorf("ContentType = %q, want audio/mpeg", blobs[0].ContentType)
	}
}

// ============================================================================
// Anthropic Messages — base64 image source
// ============================================================================

func TestExtractAnthropicImageSource(t *testing.T) {
	imgData := []byte("fake-jpg-bytes")
	messages := json.RawMessage(`[{"role":"user","content":[
		{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"` + b64(imgData) + `"}}
	]}]`)

	out, blobs := ExtractFromMessages(messages, domain.ProtocolAnthropicMessages)

	if len(blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(blobs))
	}
	if blobs[0].ContentType != "image/jpeg" {
		t.Errorf("ContentType = %q, want image/jpeg", blobs[0].ContentType)
	}
	// source.data must be replaced
	placeholder := mustUnmarshalBlob(t, out, "0", "content", "0", "source", "data")
	if !strings.HasPrefix(placeholder, "audit-blob:sha256:") {
		t.Errorf("source.data = %q, expected audit-blob prefix", placeholder)
	}
}

func TestExtractAnthropicURLSourceUnchanged(t *testing.T) {
	messages := json.RawMessage(`[{"role":"user","content":[
		{"type":"image","source":{"type":"url","url":"https://example.com/img.jpg"}}
	]}]`)
	_, blobs := ExtractFromMessages(messages, domain.ProtocolAnthropicMessages)
	if len(blobs) != 0 {
		t.Fatalf("URL source should not be extracted, got %d blobs", len(blobs))
	}
}

func TestExtractAnthropicTextBlockUnchanged(t *testing.T) {
	messages := json.RawMessage(`[{"role":"user","content":[{"type":"text","text":"hello"}]}]`)
	_, blobs := ExtractFromMessages(messages, domain.ProtocolAnthropicMessages)
	if len(blobs) != 0 {
		t.Fatalf("text block should not produce blobs, got %d", len(blobs))
	}
}

// ============================================================================
// Gemini Generate — inline_data
// ============================================================================

func TestExtractGeminiInlineData(t *testing.T) {
	imgData := []byte("fake-webp-bytes")
	contents := json.RawMessage(`[{"role":"user","parts":[
		{"inline_data":{"mime_type":"image/webp","data":"` + b64(imgData) + `"}}
	]}]`)

	out, blobs := ExtractFromMessages(contents, domain.ProtocolGeminiGenerate)

	if len(blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(blobs))
	}
	if blobs[0].ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp", blobs[0].ContentType)
	}
	placeholder := mustUnmarshalBlob(t, out, "0", "parts", "0", "inline_data", "data")
	if !strings.HasPrefix(placeholder, "audit-blob:sha256:") {
		t.Errorf("inline_data.data = %q, expected audit-blob prefix", placeholder)
	}
}

func TestExtractGeminiTextPartUnchanged(t *testing.T) {
	contents := json.RawMessage(`[{"role":"user","parts":[{"text":"hello"}]}]`)
	_, blobs := ExtractFromMessages(contents, domain.ProtocolGeminiGenerate)
	if len(blobs) != 0 {
		t.Fatalf("text part should not produce blobs, got %d", len(blobs))
	}
}

// ============================================================================
// OpenAI Images response
// ============================================================================

func TestExtractImagesResponse(t *testing.T) {
	imgData := []byte("fake-png-image")
	body := json.RawMessage(`{"created":1234,"data":[{"b64_json":"` + b64(imgData) + `","revised_prompt":"a cat"}]}`)

	out, blobs := ExtractFromImagesResponse(body)

	if len(blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(blobs))
	}
	if blobs[0].ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", blobs[0].ContentType)
	}
	// b64_json must be replaced with placeholder
	placeholder := mustUnmarshalBlob(t, out, "data", "0", "b64_json")
	if !strings.HasPrefix(placeholder, "audit-blob:sha256:") {
		t.Errorf("b64_json = %q, expected audit-blob prefix", placeholder)
	}
	// Other fields preserved
	revisedPrompt := mustUnmarshalBlob(t, out, "data", "0", "revised_prompt")
	if revisedPrompt != "a cat" {
		t.Errorf("revised_prompt = %q, want 'a cat'", revisedPrompt)
	}
}

func TestExtractImagesResponseNoBase64Unchanged(t *testing.T) {
	body := json.RawMessage(`{"created":1234,"data":[{"url":"https://example.com/img.png"}]}`)
	out, blobs := ExtractFromImagesResponse(body)
	if len(blobs) != 0 {
		t.Fatalf("URL-only response should produce no blobs, got %d", len(blobs))
	}
	if string(out) != string(body) {
		t.Errorf("body should be unchanged")
	}
}

// ============================================================================
// SHA-256 dedup: same data → same digest
// ============================================================================

func TestSHA256Dedup(t *testing.T) {
	data := []byte("identical-content")
	uri := dataURI("image/png", data)
	messages := json.RawMessage(`[{"role":"user","content":[
		{"type":"image_url","image_url":{"url":"` + uri + `"}},
		{"type":"image_url","image_url":{"url":"` + uri + `"}}
	]}]`)

	_, blobs := ExtractFromMessages(messages, domain.ProtocolOpenAIChat)

	if len(blobs) != 2 {
		t.Fatalf("expected 2 blob entries (dedup is caller's job), got %d", len(blobs))
	}
	if blobs[0].SHA256 != blobs[1].SHA256 {
		t.Errorf("same content must produce same SHA256")
	}
}

// ============================================================================
// BuildMediaRefs
// ============================================================================

func TestBuildMediaRefs(t *testing.T) {
	data := []byte("some-data")
	blob := makeBlob(data, "image/png")
	refs := BuildMediaRefs([]MediaBlob{*blob})
	if refs == nil {
		t.Fatal("BuildMediaRefs returned nil")
	}
	var arr []MediaRef
	if err := json.Unmarshal(refs, &arr); err != nil {
		t.Fatalf("unmarshal MediaRefs: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(arr))
	}
	if arr[0].SHA256 != blob.SHA256 {
		t.Errorf("SHA256 mismatch")
	}
	if arr[0].ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", arr[0].ContentType)
	}
	if arr[0].Placeholder != blob.Placeholder() {
		t.Errorf("Placeholder = %q, want %q", arr[0].Placeholder, blob.Placeholder())
	}
}

func TestBuildMediaRefsNilOnEmpty(t *testing.T) {
	if BuildMediaRefs(nil) != nil {
		t.Error("expected nil for empty blobs")
	}
}

// ============================================================================
// Embeddings and other no-op protocols
// ============================================================================

func TestNoopProtocols(t *testing.T) {
	for _, proto := range []domain.UpstreamProtocol{
		domain.ProtocolOpenAIEmbeddings,
		domain.ProtocolGeminiEmbeddings,
		domain.ProtocolOpenAIImages,
	} {
		messages := json.RawMessage(`[{"role":"user","content":"hello"}]`)
		out, blobs := ExtractFromMessages(messages, proto)
		if len(blobs) != 0 {
			t.Errorf("protocol %s: expected 0 blobs, got %d", proto, len(blobs))
		}
		if string(out) != string(messages) {
			t.Errorf("protocol %s: messages should be unchanged", proto)
		}
	}
}

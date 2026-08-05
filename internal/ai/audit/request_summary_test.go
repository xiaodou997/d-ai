package audit

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestExtractRequestPayloadWithContentTypeSummarizesImageMultipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-2"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("prompt", "touch up"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image[]", "reference.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	messages, params := ExtractRequestPayloadWithContentType(body.Bytes(), domain.ProtocolOpenAIImages, writer.FormDataContentType())
	if len(messages) != 0 {
		t.Fatalf("messages = %s, want empty", messages)
	}
	var got map[string]any
	if err := json.Unmarshal(params, &got); err != nil {
		t.Fatalf("unmarshal params: %v; params=%s", err, params)
	}
	if got["prompt"] != "touch up" {
		t.Fatalf("prompt = %#v", got["prompt"])
	}
	images, ok := got["image[]"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("image[] = %#v", got["image[]"])
	}
	image, ok := images[0].(map[string]any)
	if !ok {
		t.Fatalf("image summary = %#v", images[0])
	}
	if image["filename"] != "reference.png" || image["content_type"] != "application/octet-stream" || image["size_bytes"] != float64(5) {
		t.Fatalf("image summary = %#v", image)
	}
	preview, _ := image["value_preview"].(string)
	if !strings.HasPrefix(preview, "data:application/octet-stream;base64,aGVsbG8=") {
		t.Fatalf("value_preview = %q", preview)
	}
}

func TestSummarizeRequestBodyTruncatesLongImageURLWithoutLosingShape(t *testing.T) {
	longURL := "data:image/png;base64," + strings.Repeat("A", 4096)
	body, err := json.Marshal(map[string]any{
		"model":  "gpt-image-2",
		"images": []any{map[string]any{"image_url": longURL}},
	})
	if err != nil {
		t.Fatal(err)
	}

	summary := SummarizeRequestBody(body, "application/json")
	if !strings.Contains(summary, `"images"`) || !strings.Contains(summary, `"image_url"`) {
		t.Fatalf("summary lost request shape: %s", summary)
	}
	if !strings.Contains(summary, "data:image/png;base64,AAAA") || !strings.Contains(summary, "<truncated") {
		t.Fatalf("summary did not retain a short data URL preview: %s", summary)
	}
	if len(summary) >= len(body) {
		t.Fatalf("summary was not shortened: summary=%d body=%d", len(summary), len(body))
	}
}

func TestSummarizeRequestBodyRedactsSignedURLQuery(t *testing.T) {
	body := []byte(`{"image_url":"https://cdn.example.test/reference.png?token=secret#fragment"}`)
	summary := SummarizeRequestBody(body, "application/json")
	if strings.Contains(summary, "secret") || strings.Contains(summary, "fragment") {
		t.Fatalf("summary leaked signed URL credentials: %s", summary)
	}
	if !strings.Contains(summary, "https://cdn.example.test/reference.png?<redacted>") {
		t.Fatalf("summary lost the safe URL prefix: %s", summary)
	}
}

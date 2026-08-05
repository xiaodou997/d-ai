package gateway

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"testing"

	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/imageedit"
	"xiaodou/dai/internal/ai/serving"
)

func TestNormalizeOpenAIImageRequestRejectsJSONPartialImages(t *testing.T) {
	_, _, err := normalizeOpenAIImageRequest(
		[]byte(`{"model":"gpt-image-1","prompt":"poster","partial_images":1}`),
		"application/json",
	)
	if err == nil {
		t.Fatal("expected partial_images to be rejected")
	}
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Message != openAIImagePartialImagesUnsupportedMessage {
		t.Fatalf("message = %q", apiErr.Message)
	}
}

func TestNormalizeOpenAIImageRequestRejectsMultipartPartialImages(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("partial_images", "1"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image[]", "image.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	_, _, err = normalizeOpenAIImageRequest(body.Bytes(), writer.FormDataContentType())
	if err == nil {
		t.Fatal("expected partial_images to be rejected")
	}
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Message != openAIImagePartialImagesUnsupportedMessage {
		t.Fatalf("message = %q", apiErr.Message)
	}
}

func TestDecodeRunImageGenerationRequestBodyRejectsPartialImages(t *testing.T) {
	_, err := decodeRunImageGenerationRequestBody(
		[]byte(`{"input":"poster","partial_images":1}`),
	)
	if err == nil {
		t.Fatal("expected partial_images to be rejected")
	}
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Message != openAIImagePartialImagesUnsupportedMessage {
		t.Fatalf("message = %q", apiErr.Message)
	}
}

func TestDecodeRunImageGenerationRequestBodyRejectsPromptField(t *testing.T) {
	_, err := decodeRunImageGenerationRequestBody([]byte(`{"input":"poster","prompt":"legacy"}`))
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Message != "prompt is not supported for app runs; use input" {
		t.Fatalf("message = %q", apiErr.Message)
	}
}

func TestDecodeAppRunImageEditRequestBodyRejectsPromptField(t *testing.T) {
	_, _, err := decodeAppRunImageEditRequestBody(
		[]byte(`{"input":"retouch","prompt":"legacy","images":[{"image_url":"https://example.com/ref.png"}]}`),
		imageedit.TransportJSON,
	)
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Message != "prompt is not supported for app runs; use input" {
		t.Fatalf("message = %q", apiErr.Message)
	}
}

func TestValidateOpenAIImageInputSizeRejectsOversizedBase64(t *testing.T) {
	encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 33))
	err := validateOpenAIImageInputSize(
		[]byte(`{"images":[{"image_url":"`+encoded+`"}]}`),
		"application/json",
		32,
	)
	assertImageInputTooLarge(t, err)
}

func TestValidateOpenAIImageInputSizeRejectsOversizedMultipartFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image[]", "large.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 33)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	err = validateOpenAIImageInputSize(body.Bytes(), writer.FormDataContentType(), 32)
	assertImageInputTooLarge(t, err)
}

func assertImageInputTooLarge(t *testing.T, err error) {
	t.Helper()
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", apiErr.Status, http.StatusRequestEntityTooLarge)
	}
}

func TestNormalizeOpenAIImageRequestFiltersJSONClientOptions(t *testing.T) {
	body, contentType, err := normalizeOpenAIImageRequest(
		[]byte(`{"model":"gpt-image-1","prompt":"poster","n":1,"size":"1024x1024","quality":"high","style":"vivid"}`),
		"application/json",
	)
	if err != nil {
		t.Fatalf("normalizeOpenAIImageRequest: %v", err)
	}
	if contentType != "application/json" {
		t.Fatalf("content type = %q", contentType)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal sanitized body: %v", err)
	}
	for _, field := range []string{"quality", "style"} {
		if _, ok := payload[field]; ok {
			t.Fatalf("normalized body should not include %s: %#v", field, payload[field])
		}
	}
	if payload["n"] != float64(1) {
		t.Fatalf("n = %#v, want 1", payload["n"])
	}
	if payload["size"] != "1024x1024" {
		t.Fatalf("size = %#v, want 1024x1024", payload["size"])
	}
}

func TestNormalizeOpenAIImageRequestFiltersMultipartClientOptions(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("prompt", "poster"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("n", "1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("size", "1024x1024"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("quality", "high"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("style", "vivid"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image[]", "image.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	sanitized, contentType, err := normalizeOpenAIImageRequest(body.Bytes(), writer.FormDataContentType())
	if err != nil {
		t.Fatalf("normalizeOpenAIImageRequest: %v", err)
	}
	fields, err := formats.MultipartScalarFields(sanitized, contentType, 1<<20)
	if err != nil {
		t.Fatalf("MultipartScalarFields: %v", err)
	}
	for _, field := range []string{"quality", "style"} {
		if _, ok := fields[field]; ok {
			t.Fatalf("normalized multipart should not include %s", field)
		}
	}
	if fields["n"] != "1" {
		t.Fatalf("n = %q, want 1", fields["n"])
	}
	if fields["size"] != "1024x1024" {
		t.Fatalf("size = %q, want 1024x1024", fields["size"])
	}
}

func TestNormalizeOpenAIImageRequestAcceptsMultipleOutputsWithinPlatformLimit(t *testing.T) {
	body, _, err := normalizeOpenAIImageRequest([]byte(`{"model":"gpt-image-1","prompt":"poster","n":4}`), "application/json")
	if err != nil {
		t.Fatalf("normalizeOpenAIImageRequest: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload["n"] != float64(4) {
		t.Fatalf("normalized n = %#v, err = %v", payload["n"], err)
	}
	if _, _, err := normalizeOpenAIImageRequest([]byte(`{"n":11}`), "application/json"); err == nil {
		t.Fatal("expected n=11 to be rejected")
	}
}

func TestSanitizeGeminiImageCountPreservesValidCandidateCount(t *testing.T) {
	body, err := sanitizeGeminiImageCount([]byte(`{
		"generationConfig": {
			"responseModalities": ["TEXT", "IMAGE"],
			"candidateCount": 4
		},
		"contents": [{"role":"user","parts":[{"text":"draw a poster"}]}]
	}`))
	if err != nil {
		t.Fatalf("sanitizeGeminiImageCount: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal sanitized body: %v", err)
	}
	generationConfig, _ := payload["generationConfig"].(map[string]any)
	if generationConfig["candidateCount"] != float64(4) {
		t.Fatalf("candidateCount = %#v, want 4", generationConfig["candidateCount"])
	}
	if _, err := sanitizeGeminiImageCount([]byte(`{"generationConfig":{"responseModalities":["IMAGE"],"candidateCount":11}}`)); err == nil {
		t.Fatal("expected candidateCount=11 to be rejected")
	}
}

func TestParseImageBillingMetaUsesRequestedOutputCount(t *testing.T) {
	count, size := parseImageBillingMeta([]byte(`{"n":4,"size":"1536x1024"}`), "application/json")
	if count != 4 || size != "1536x1024" {
		t.Fatalf("OpenAI billing meta = (%d, %q)", count, size)
	}
	count, size = parseImageBillingMeta([]byte(`{"generationConfig":{"candidateCount":3,"imageSize":"1024x1024"}}`), "application/json")
	if count != 3 || size != "1024x1024" {
		t.Fatalf("Gemini billing meta = (%d, %q)", count, size)
	}
}

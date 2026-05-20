package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFakeUpstreamChatNonStream(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"local-chat-test","messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()

	newHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"total_tokens":12`) {
		t.Fatalf("body missing stable usage: %s", rec.Body.String())
	}
}

func TestFakeUpstreamResponsesStream(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"local-responses-test","input":"hi","stream":true}`))
	rec := httptest.NewRecorder()

	newHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"response.output_text.delta"`) {
		t.Fatalf("stream missing delta event: %s", body)
	}
	if !strings.Contains(body, `"type":"response.completed"`) {
		t.Fatalf("stream missing completed event: %s", body)
	}
}

func TestFakeUpstreamEmbeddings(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"local-embedding-test","input":["a"]}`))
	rec := httptest.NewRecorder()

	newHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"prompt_tokens":9`) {
		t.Fatalf("body missing embedding usage: %s", rec.Body.String())
	}
}

func TestFakeUpstreamImages(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"local-image-test","prompt":"cat","n":2}`))
	rec := httptest.NewRecorder()

	newHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Count(rec.Body.String(), "https://fake.local/images/") != 2 {
		t.Fatalf("body does not contain two fake image URLs: %s", rec.Body.String())
	}
}

func TestFakeUpstreamImageEditsMultipart(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("model", "local-image-test"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("prompt", "replace background"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("n", "3"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()

	newHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Count(rec.Body.String(), "https://fake.local/images/") != 3 {
		t.Fatalf("body does not contain three fake image URLs: %s", rec.Body.String())
	}
}

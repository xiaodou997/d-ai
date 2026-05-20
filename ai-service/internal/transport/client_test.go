package transport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/serving"
)

func TestResolveURLGeminiAppendsKey(t *testing.T) {
	req := &serving.UpstreamRequest{
		URL:      "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent",
		Protocol: domain.ProtocolGeminiGenerate,
		Headers:  map[string]string{"x-gemini-api-key": "AIza123"},
	}
	got := resolveURL(req)
	if !strings.Contains(got, "key=AIza123") {
		t.Fatalf("expected ?key= in url, got %q", got)
	}
	// resolveURL must NOT mutate Headers — a retry must still see the key.
	if req.Headers["x-gemini-api-key"] != "AIza123" {
		t.Fatalf("resolveURL must not delete the synthetic key header")
	}
}

func TestResolveURLGeminiStreamAddsSSE(t *testing.T) {
	req := &serving.UpstreamRequest{
		URL:      "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent",
		Protocol: domain.ProtocolGeminiGenerate,
		Headers: map[string]string{
			"x-gemini-api-key": "AIza",
			"Accept":           "text/event-stream",
		},
	}
	got := resolveURL(req)
	if !strings.Contains(got, ":streamGenerateContent") {
		t.Fatalf("stream variant missing in url: %q", got)
	}
	if !strings.Contains(got, "alt=sse") {
		t.Fatalf("alt=sse missing in url: %q", got)
	}
}

func TestResolveURLNonGeminiUnchanged(t *testing.T) {
	in := "https://api.openai.com/v1/chat/completions"
	req := &serving.UpstreamRequest{
		URL:      in,
		Protocol: domain.ProtocolOpenAIChat,
		Headers:  map[string]string{},
	}
	if got := resolveURL(req); got != in {
		t.Fatalf("non-gemini url should be unchanged, got %q", got)
	}
}

func TestClientDoStripsSyntheticHeader(t *testing.T) {
	// Spin up a fake upstream that records the Headers it receives.
	var gotKeyHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKeyHeader = r.Header.Get("x-gemini-api-key")
		_, _ = io.WriteString(w, "{}")
	}))
	defer srv.Close()

	c := NewClient(2 * time.Second)
	req := &serving.UpstreamRequest{
		Method:   "POST",
		URL:      srv.URL + "/v1beta/models/g:generateContent",
		Protocol: domain.ProtocolGeminiGenerate,
		Headers:  map[string]string{"x-gemini-api-key": "secret-AIza"},
	}
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if gotKeyHeader != "" {
		t.Fatalf("synthetic x-gemini-api-key leaked to wire: %q", gotKeyHeader)
	}
}

package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"xiaodou/dai/internal/ai/domain"
)

func TestUpstreamDiagnosticValidatesCompleteResponsesStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["stream"] != true || r.Header.Get("Accept") != "text/event-stream" || r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected request %v %v", body, r.Header)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n")
		io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1,\"total_tokens\":4}}}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // completion must not wait for EOF
	}))
	defer server.Close()
	result := runUpstreamAccountTest(context.Background(), NewClient(0).DiagnosticClient(), upstreamTestConfig{BaseURL: server.URL, APIFormat: string(domain.ProtocolOpenAIResponses), UpstreamModel: "model", Capability: "chat"})
	if !result.OK || result.ReplyText != "OK" || result.TotalTokens != 4 {
		t.Fatalf("result=%+v", result)
	}
}

func TestUpstreamDiagnosticRejectsWrongProtocolAndIncompleteStream(t *testing.T) {
	for _, tc := range []struct{ name, body, contains string }{
		{"plain", "Hi! What can I help you with?", "JSON"},
		{"provider error", `{"error":{"message":"model unavailable"}}`, "model unavailable"},
		{"truncated", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n", "未收到完成事件"},
		{"failed after text", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"message\":\"quota exceeded\"}}}\n\n", "quota exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result upstreamTestResult
			parseUpstreamTestChatResponse(&result, string(domain.ProtocolOpenAIResponses), []byte(tc.body))
			if result.OK || !strings.Contains(result.Error, tc.contains) {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

type diagnosticProxySelector struct {
	url   *url.URL
	calls int
}

func (s *diagnosticProxySelector) SelectProxy(context.Context) (*url.URL, error) {
	s.calls++
	return s.url, nil
}
func TestDiagnosticClientUsesBusinessProxySelector(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "upstream.invalid" {
			t.Errorf("host=%s", r.URL.Host)
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	defer proxy.Close()
	endpoint, _ := url.Parse(proxy.URL)
	selector := &diagnosticProxySelector{url: endpoint}
	client := NewClient(0)
	client.SetProxySelector(selector)
	req, _ := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1/responses", strings.NewReader(`{}`))
	res, err := client.DiagnosticClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || selector.calls != 1 {
		t.Fatalf("status=%d selector=%d", res.StatusCode, selector.calls)
	}
}

func TestDiagnosticChatAndAnthropicStreams(t *testing.T) {
	for _, tc := range []struct{ format, body string }{
		{string(domain.ProtocolOpenAIChat), "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"},
		{string(domain.ProtocolAnthropicMessages), "event: content_block_delta\ndata: {\"delta\":{\"text\":\"OK\"}}\n\nevent: message_stop\ndata: {}\n\n"},
	} {
		var r upstreamTestResult
		parseUpstreamTestChatResponse(&r, tc.format, []byte(tc.body))
		if !r.OK || r.ReplyText != "OK" {
			t.Fatalf("%s: %+v", tc.format, r)
		}
	}
}

func TestDiagnosticRejectsMalformedEventsBeforeCompletion(t *testing.T) {
	for _, bad := range []string{"null", "not-json"} {
		body := "event: response.output_text.delta\ndata: " + bad + "\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"OK\"}}\n\n"
		var result upstreamTestResult
		parseUpstreamTestChatResponse(&result, string(domain.ProtocolOpenAIResponses), []byte(body))
		if result.OK || result.Error == "" {
			t.Fatalf("malformed event accepted: %+v", result)
		}
	}
}

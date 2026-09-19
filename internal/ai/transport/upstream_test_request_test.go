package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xiaodou/dai/internal/ai/domain"
)

func TestResponsesDiagnosticUsesContextAndNoTinyOutputCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var doc map[string]any
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		input := doc["input"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
		prompt, _ := input["text"].(string)
		if !strings.Contains(prompt, "func (c *Cache) SeenOrAdd") || doc["instructions"] == nil || doc["store"] != false || doc["stream"] != true {
			t.Errorf("missing representative Responses request")
		}
		if _, ok := doc["max_output_tokens"]; ok {
			t.Error("diagnostic imposed an output token cap")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-ID", "upstream-review-1")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"Duplicate reads extend expiration.\"}}\n\n")
	}))
	defer server.Close()
	result := runUpstreamAccountTest(context.Background(), NewClient(0).DiagnosticClient(), upstreamTestConfig{BaseURL: server.URL, APIFormat: string(domain.ProtocolOpenAIResponses), UpstreamModel: "bound-model", Capability: "chat"})
	if !result.OK || result.ResponseContentType != "text/event-stream" || result.UpstreamRequestID != "upstream-review-1" {
		t.Fatalf("result=%+v", result)
	}
}

func TestExplicitPromptReplacesDefaultDiagnosticContext(t *testing.T) {
	req, err := buildUpstreamTestRequest(context.Background(), upstreamTestConfig{BaseURL: "https://upstream.example", APIFormat: "openai_responses", UpstreamModel: "model"}, false, "my exact prompt")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(req.Body)
	if !strings.Contains(string(b), "my exact prompt") || strings.Contains(string(b), "SeenOrAdd") {
		t.Fatal("custom prompt was not preserved")
	}
}

func TestDiagnosticRejectsOutputLimitTermination(t *testing.T) {
	for _, tc := range []struct{ format, body string }{
		{"openai_chat", `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`},
		{"anthropic_messages", `{"content":[{"type":"text","text":"partial"}],"stop_reason":"max_tokens"}`},
		{"anthropic_messages", "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"partial\"}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"},
	} {
		var result upstreamTestResult
		parseUpstreamTestChatResponse(&result, tc.format, []byte(tc.body))
		if result.OK || result.Error == "" {
			t.Fatalf("%s: %+v", tc.format, result)
		}
	}
}

func TestCompletedToolCallIsValidDiagnosticOutputWithoutExecution(t *testing.T) {
	body := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"function_call\",\"name\":\"lookup\",\"arguments\":\"{}\"}]}}\n\n"
	var result upstreamTestResult
	parseUpstreamTestChatResponse(&result, "openai_responses", []byte(body))
	if !result.OK || !strings.Contains(result.ReplyText, "lookup") || !strings.Contains(result.ReplyText, "未执行") {
		t.Fatalf("result=%+v", result)
	}
}

func TestRedactUpstreamDiagnosticSecretsRedactsCredentialHeaders(t *testing.T) {
	got := redactUpstreamDiagnosticSecrets(
		"provider echoed sk-main and Bearer header-secret; trace-public is safe",
		"sk-main",
		[]byte(`{"Authorization":"Bearer header-secret","X-Trace":"trace-public"}`),
	)
	if strings.Contains(got, "sk-main") || strings.Contains(got, "header-secret") {
		t.Fatalf("diagnostic error leaked credentials: %q", got)
	}
	if !strings.Contains(got, "trace-public") {
		t.Fatalf("non-sensitive diagnostic context was removed: %q", got)
	}
}

package formats

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/textproto"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

func TestRewriteModelNoOpWhenAlreadyMatches(t *testing.T) {
	in := []byte(`{"model":"foo","messages":[{"role":"user","content":"hi"}]}`)
	out, err := RewriteModel(in, "foo", "application/json")
	if err != nil {
		t.Fatal(err)
	}
	// Strict passthrough — when nothing changes, return original slice.
	if &in[0] != &out[0] {
		t.Errorf("expected zero-copy when model matches, got new slice")
	}
}

func TestRewriteModelReplaces(t *testing.T) {
	in := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}],"cache_control":{"type":"ephemeral"}}`)
	out, err := RewriteModel(in, "gpt-4-turbo-2026-05-01", "application/json")
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["model"] != "gpt-4-turbo-2026-05-01" {
		t.Errorf("model = %v, want gpt-4-turbo-2026-05-01", parsed["model"])
	}
	// Critical: unknown fields like cache_control must survive the rewrite.
	if _, ok := parsed["cache_control"]; !ok {
		t.Errorf("cache_control was dropped during model rewrite — passthrough is broken")
	}
	if _, ok := parsed["messages"]; !ok {
		t.Errorf("messages was dropped during model rewrite")
	}
}

func TestApplyCodexRequestModifications(t *testing.T) {
	in := []byte(`{"model":"gpt-5","max_output_tokens":4096,"temperature":0.7,"top_p":0.9,"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}],"previous_response_id":"resp_abc"}`)
	out, err := ApplyCodexRequestModifications(in)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"max_output_tokens", "temperature", "top_p"} {
		if _, ok := got[banned]; ok {
			t.Errorf("%s should have been stripped", banned)
		}
	}
	if got["store"] != false {
		t.Errorf("store = %v, want false", got["store"])
	}
	if got["instructions"] != "You are ChatGPT." {
		t.Errorf("instructions = %v, want default", got["instructions"])
	}
	// previous_response_id must be preserved — it's the multi-turn chain key.
	if got["previous_response_id"] != "resp_abc" {
		t.Errorf("previous_response_id was dropped — Codex multi-turn would break")
	}
}

func TestParseRequestMetaMultipart(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("model", "gpt-image-2"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("size", "1024x1024"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	meta, err := ParseRequestMeta(buf.Bytes(), w.FormDataContentType())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Model != "gpt-image-2" || meta.Stream {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestRewriteModelMultipart(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("model", "old-model"); err != nil {
		t.Fatal(err)
	}
	part, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="image"; filename="a.png"`},
		"Content-Type":        {"image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("fakepng")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := RewriteModel(buf.Bytes(), "gpt-image-2", w.FormDataContentType())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseRequestMeta(out, w.FormDataContentType())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Model != "gpt-image-2" {
		t.Fatalf("model not rewritten: %+v", meta)
	}
}

func TestExtractSyncUsage_OpenAIChat(t *testing.T) {
	body := []byte(`{"id":"x","usage":{"prompt_tokens":100,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":12}}}`)
	u := ExtractSyncUsage(body, domain.ProtocolOpenAIChat)
	if u.PromptTokens != 100 || u.CompletionTokens != 50 || u.CacheReadTokens != 40 || u.ReasoningTokens != 12 {
		t.Errorf("got %+v", u)
	}
}

func TestExtractSyncUsage_Anthropic(t *testing.T) {
	body := []byte(`{"id":"msg","usage":{"input_tokens":80,"output_tokens":25,"cache_creation_input_tokens":200,"cache_read_input_tokens":500}}`)
	u := ExtractSyncUsage(body, domain.ProtocolAnthropicMessages)
	if u.PromptTokens != 80 || u.CompletionTokens != 25 || u.CacheWriteTokens != 200 || u.CacheReadTokens != 500 {
		t.Errorf("got %+v — Anthropic prompt-caching billing depends on cache_creation/cache_read", u)
	}
}

func TestExtractSyncUsage_OpenAIResponses(t *testing.T) {
	body := []byte(`{"id":"resp","usage":{"input_tokens":120,"output_tokens":40,"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":8}}}`)
	u := ExtractSyncUsage(body, domain.ProtocolOpenAIResponses)
	if u.PromptTokens != 120 || u.CompletionTokens != 40 || u.CacheReadTokens != 30 || u.ReasoningTokens != 8 {
		t.Errorf("got %+v", u)
	}
}

func TestExtractSyncUsage_Gemini(t *testing.T) {
	body := []byte(`{"usageMetadata":{"promptTokenCount":60,"candidatesTokenCount":15,"cachedContentTokenCount":20,"thoughtsTokenCount":5}}`)
	u := ExtractSyncUsage(body, domain.ProtocolGeminiGenerate)
	if u.PromptTokens != 60 || u.CompletionTokens != 15 || u.CacheReadTokens != 20 || u.ReasoningTokens != 5 {
		t.Errorf("got %+v", u)
	}
}

func TestExtractStreamUsage_AnthropicSplit(t *testing.T) {
	// message_start carries input + cache counters and a placeholder output_tokens.
	start := []byte(`{"type":"message_start","message":{"usage":{"input_tokens":42,"output_tokens":1,"cache_read_input_tokens":300}}}`)
	u, ok := ExtractStreamUsage(domain.TokenUsage{}, start, "message_start", domain.ProtocolAnthropicMessages)
	if !ok {
		t.Fatal("expected message_start to report usage")
	}
	if u.PromptTokens != 42 || u.CacheReadTokens != 300 {
		t.Errorf("after message_start: %+v", u)
	}
	// message_delta later updates only output_tokens (cumulative).
	delta := []byte(`{"type":"message_delta","usage":{"output_tokens":17}}`)
	u, ok = ExtractStreamUsage(u, delta, "message_delta", domain.ProtocolAnthropicMessages)
	if !ok {
		t.Fatal("expected message_delta to report usage")
	}
	if u.PromptTokens != 42 || u.CacheReadTokens != 300 {
		t.Errorf("message_delta clobbered input counters: %+v", u)
	}
	if u.CompletionTokens != 17 {
		t.Errorf("CompletionTokens = %d, want 17 (cumulative output)", u.CompletionTokens)
	}
}

func TestExtractStreamUsage_OpenAIResponsesNested(t *testing.T) {
	// /v1/responses streaming wraps the terminal payload in {"response": {...}}.
	data := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":11,"output_tokens":22,"input_tokens_details":{"cached_tokens":3}}}}`)
	u, ok := ExtractStreamUsage(domain.TokenUsage{}, data, "response.completed", domain.ProtocolOpenAIResponses)
	if !ok {
		t.Fatal("expected response.completed to report usage")
	}
	if u.PromptTokens != 11 || u.CompletionTokens != 22 || u.CacheReadTokens != 3 {
		t.Errorf("got %+v", u)
	}
}

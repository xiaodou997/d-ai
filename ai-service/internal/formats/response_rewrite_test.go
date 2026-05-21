package formats

import (
	"bytes"
	"encoding/json"
	"testing"

	"xiaodou/uni-ai-api/internal/domain"
)

// ---- RewriteSyncResponseModel -----------------------------------------------

func TestRewriteSyncResponseModel_OpenAI(t *testing.T) {
	body := mustJSON(map[string]any{
		"id":    "chatcmpl-abc",
		"model": "gpt-4-turbo-preview",
		"choices": []any{
			map[string]any{"message": map[string]any{"role": "assistant", "content": "hi"}},
		},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
	})

	got := RewriteSyncResponseModel(body, "gpt-4", domain.ProtocolOpenAIChat)

	var result map[string]any
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if result["model"] != "gpt-4" {
		t.Errorf("model = %q, want %q", result["model"], "gpt-4")
	}
	// Other fields must survive.
	if result["id"] != "chatcmpl-abc" {
		t.Errorf("id field lost after rewrite")
	}
}

func TestSanitizePublicModelJSON_OpenAIResponsesNested(t *testing.T) {
	body := mustJSON(map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":     "resp_abc",
			"object": "response",
			"model":  "gpt-5.4-mini-2026-03-17",
			"status": "in_progress",
		},
		"sequence_number": 0,
	})

	got := SanitizePublicModelJSON(body, "gpt-5.4-mini", "gpt-5.4-mini-2026-03-17", domain.ProtocolOpenAIResponses)

	var result map[string]any
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	resp := result["response"].(map[string]any)
	if resp["model"] != "gpt-5.4-mini" {
		t.Errorf("response.model = %q, want %q", resp["model"], "gpt-5.4-mini")
	}
}

func TestSanitizePublicModelJSON_ExactUpstreamOnly(t *testing.T) {
	body := mustJSON(map[string]any{
		"model": "unrelated-model",
		"nested": map[string]any{
			"model": "upstream-internal",
		},
	})

	got := SanitizePublicModelJSON(body, "public", "upstream-internal", domain.ProtocolOpenAIChat)

	var result map[string]any
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatal(err)
	}
	if result["model"] != "unrelated-model" {
		t.Errorf("top-level unrelated model was changed: %q", result["model"])
	}
	nested := result["nested"].(map[string]any)
	if nested["model"] != "public" {
		t.Errorf("nested model = %q, want public", nested["model"])
	}
}

func TestRewriteSyncResponseModel_Anthropic(t *testing.T) {
	body := mustJSON(map[string]any{
		"id":    "msg_01",
		"type":  "message",
		"model": "claude-opus-4-5-20251101",
		"role":  "assistant",
	})

	got := RewriteSyncResponseModel(body, "claude-opus", domain.ProtocolAnthropicMessages)

	var result map[string]any
	json.Unmarshal(got, &result) //nolint:errcheck
	if result["model"] != "claude-opus" {
		t.Errorf("model = %q, want %q", result["model"], "claude-opus")
	}
}

func TestRewriteSyncResponseModel_Gemini(t *testing.T) {
	body := mustJSON(map[string]any{
		"candidates":   []any{},
		"modelVersion": "gemini-2.0-flash-001",
	})

	got := RewriteSyncResponseModel(body, "gemini-flash", domain.ProtocolGeminiGenerate)

	var result map[string]any
	json.Unmarshal(got, &result) //nolint:errcheck
	if result["modelVersion"] != "gemini-flash" {
		t.Errorf("modelVersion = %q, want %q", result["modelVersion"], "gemini-flash")
	}
	// The "model" key must not be introduced.
	if _, ok := result["model"]; ok {
		t.Errorf("unexpected \"model\" key introduced for Gemini response")
	}
}

func TestRewriteSyncResponseModel_AlreadyPublic(t *testing.T) {
	body := mustJSON(map[string]any{"model": "gpt-4", "id": "x"})
	got := RewriteSyncResponseModel(body, "gpt-4", domain.ProtocolOpenAIChat)
	// Must return original slice unchanged.
	if &got[0] != &body[0] {
		t.Error("expected original slice to be returned unchanged when model already matches")
	}
}

func TestRewriteSyncResponseModel_EmptyPublicModel(t *testing.T) {
	body := mustJSON(map[string]any{"model": "gpt-4-turbo"})
	got := RewriteSyncResponseModel(body, "", domain.ProtocolOpenAIChat)
	if &got[0] != &body[0] {
		t.Error("expected original slice when publicModel is empty")
	}
}

func TestRewriteSyncResponseModel_NotJSON(t *testing.T) {
	body := []byte("not json")
	got := RewriteSyncResponseModel(body, "gpt-4", domain.ProtocolOpenAIChat)
	if string(got) != "not json" {
		t.Error("non-JSON body should be returned unchanged")
	}
}

// ---- PublicModelSanitizer ---------------------------------------------------

func TestPublicModelSanitizer_OpenAI(t *testing.T) {
	rw := NewPublicModelSanitizer("gpt-4", "gpt-4-turbo-preview", domain.ProtocolOpenAIChat)

	frame1 := mustJSON(map[string]any{"id": "chatcmpl-1", "model": "gpt-4-turbo-preview", "choices": []any{}})
	frame2 := mustJSON(map[string]any{"id": "chatcmpl-1", "model": "gpt-4-turbo-preview", "choices": []any{}})
	frame3 := mustJSON(map[string]any{"id": "chatcmpl-1", "model": "gpt-4-turbo-preview", "choices": []any{}})

	r1 := rw.Sanitize(frame1)
	r2 := rw.Sanitize(frame2)
	r3 := rw.Sanitize(frame3)

	for i, r := range [][]byte{r1, r2, r3} {
		var obj map[string]any
		if err := json.Unmarshal(r, &obj); err != nil {
			t.Fatalf("frame %d: invalid JSON: %v", i+1, err)
		}
		if obj["model"] != "gpt-4" {
			t.Errorf("frame %d: model = %q, want %q", i+1, obj["model"], "gpt-4")
		}
	}
}

func TestPublicModelSanitizer_AlreadyPublic(t *testing.T) {
	rw := NewPublicModelSanitizer("gpt-4", "gpt-4", domain.ProtocolOpenAIChat)
	frame := mustJSON(map[string]any{"model": "gpt-4"})
	out := rw.Sanitize(frame)
	// Must return original slice.
	if len(out) != len(frame) || (len(out) > 0 && &out[0] != &frame[0]) {
		t.Error("expected original slice returned when model already matches")
	}
}

func TestPublicModelSanitizer_Anthropic_MessageStart(t *testing.T) {
	// Anthropic message_start event: model is nested under "message".
	rw := NewPublicModelSanitizer("claude-my", "claude-opus-4-5-20251101", domain.ProtocolAnthropicMessages)

	frame := mustJSON(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":    "msg_01",
			"model": "claude-opus-4-5-20251101",
			"role":  "assistant",
		},
	})

	out := rw.Sanitize(frame)

	// The raw bytes should contain the new model name.
	var obj map[string]any
	json.Unmarshal(out, &obj) //nolint:errcheck
	msg := obj["message"].(map[string]any)
	if msg["model"] != "claude-my" {
		t.Errorf("nested model = %q, want %q", msg["model"], "claude-my")
	}
}

func TestPublicModelSanitizer_Anthropic_ContentDelta(t *testing.T) {
	// content_block_delta frames have no model field — rewriter must be noop.
	rw := NewPublicModelSanitizer("claude-my", "claude-opus-4-5-20251101", domain.ProtocolAnthropicMessages)

	frame := mustJSON(map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{"type": "text_delta", "text": "hello"},
	})

	out := rw.Sanitize(frame)
	if !bytesEqual(out, frame) {
		t.Error("content_block_delta frame should be returned unchanged")
	}
}

func TestPublicModelSanitizer_Gemini(t *testing.T) {
	rw := NewPublicModelSanitizer("gemini-flash", "gemini-2.0-flash-001", domain.ProtocolGeminiGenerate)

	frame := mustJSON(map[string]any{
		"candidates":   []any{},
		"modelVersion": "gemini-2.0-flash-001",
	})

	out := rw.Sanitize(frame)

	var obj map[string]any
	json.Unmarshal(out, &obj) //nolint:errcheck
	if obj["modelVersion"] != "gemini-flash" {
		t.Errorf("modelVersion = %q, want %q", obj["modelVersion"], "gemini-flash")
	}
	if _, ok := obj["model"]; ok {
		t.Error("unexpected \"model\" key for Gemini stream frame")
	}
}

func TestPublicModelSanitizer_DoesNotNoopAfterFirstFrameWithoutModel(t *testing.T) {
	rw := NewPublicModelSanitizer("public", "upstream-internal", domain.ProtocolOpenAIResponses)

	first := mustJSON(map[string]any{"type": "response.output_text.delta", "delta": "hello"})
	if out := rw.Sanitize(first); !bytesEqual(out, first) {
		t.Errorf("first frame without upstream model should be unchanged: %s", out)
	}

	second := mustJSON(map[string]any{
		"type": "response.in_progress",
		"response": map[string]any{
			"id":    "resp_1",
			"model": "upstream-internal",
		},
	})
	out := rw.Sanitize(second)
	if !bytes.Contains(out, []byte(`"model":"public"`)) && !bytes.Contains(out, []byte(`"model": "public"`)) {
		t.Errorf("nested response.model was not sanitized: %s", out)
	}
}

// ---- helpers ----------------------------------------------------------------

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

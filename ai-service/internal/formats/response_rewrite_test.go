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

// ---- StreamModelRewriter ----------------------------------------------------

func TestStreamModelRewriter_OpenAI(t *testing.T) {
	rw := NewStreamModelRewriter("gpt-4", domain.ProtocolOpenAIChat)

	frame1 := mustJSON(map[string]any{"id": "chatcmpl-1", "model": "gpt-4-turbo-preview", "choices": []any{}})
	frame2 := mustJSON(map[string]any{"id": "chatcmpl-1", "model": "gpt-4-turbo-preview", "choices": []any{}})
	frame3 := mustJSON(map[string]any{"id": "chatcmpl-1", "model": "gpt-4-turbo-preview", "choices": []any{}})

	r1 := rw.Rewrite(frame1)
	r2 := rw.Rewrite(frame2)
	r3 := rw.Rewrite(frame3)

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

func TestStreamModelRewriter_AlreadyPublic(t *testing.T) {
	rw := NewStreamModelRewriter("gpt-4", domain.ProtocolOpenAIChat)
	frame := mustJSON(map[string]any{"model": "gpt-4"})
	out := rw.Rewrite(frame)
	// Must return original slice.
	if len(out) != len(frame) || (len(out) > 0 && &out[0] != &frame[0]) {
		t.Error("expected original slice returned when model already matches")
	}
}

func TestStreamModelRewriter_Anthropic_MessageStart(t *testing.T) {
	// Anthropic message_start event: model is nested under "message".
	rw := NewStreamModelRewriter("claude-my", domain.ProtocolAnthropicMessages)

	frame := mustJSON(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":    "msg_01",
			"model": "claude-opus-4-5-20251101",
			"role":  "assistant",
		},
	})

	out := rw.Rewrite(frame)

	// The raw bytes should contain the new model name.
	var obj map[string]any
	json.Unmarshal(out, &obj) //nolint:errcheck
	msg := obj["message"].(map[string]any)
	if msg["model"] != "claude-my" {
		t.Errorf("nested model = %q, want %q", msg["model"], "claude-my")
	}
}

func TestStreamModelRewriter_Anthropic_ContentDelta(t *testing.T) {
	// content_block_delta frames have no model field — rewriter must be noop.
	rw := NewStreamModelRewriter("claude-my", domain.ProtocolAnthropicMessages)

	frame := mustJSON(map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{"type": "text_delta", "text": "hello"},
	})

	out := rw.Rewrite(frame)
	if !bytesEqual(out, frame) {
		t.Error("content_block_delta frame should be returned unchanged")
	}
}

func TestStreamModelRewriter_Gemini(t *testing.T) {
	rw := NewStreamModelRewriter("gemini-flash", domain.ProtocolGeminiGenerate)

	frame := mustJSON(map[string]any{
		"candidates":   []any{},
		"modelVersion": "gemini-2.0-flash-001",
	})

	out := rw.Rewrite(frame)

	var obj map[string]any
	json.Unmarshal(out, &obj) //nolint:errcheck
	if obj["modelVersion"] != "gemini-flash" {
		t.Errorf("modelVersion = %q, want %q", obj["modelVersion"], "gemini-flash")
	}
	if _, ok := obj["model"]; ok {
		t.Error("unexpected \"model\" key for Gemini stream frame")
	}
}

func TestStreamModelRewriter_ByteReplacement_SubsequentFrames(t *testing.T) {
	// Verify that after the first frame the rewriter uses byte replacement
	// (not JSON parse) for subsequent frames. We inject a frame that is not
	// valid JSON to confirm it still gets the expected textual substitution.
	rw := NewStreamModelRewriter("public", domain.ProtocolOpenAIChat)

	// First frame — valid JSON to seed the rewriter.
	seed := mustJSON(map[string]any{"model": "upstream-internal", "id": "x"})
	rw.Rewrite(seed)

	// Subsequent frames: textual replacement must work even without full JSON parse.
	raw := []byte(`{"id":"x","model":"upstream-internal","choices":[]}`)
	out := rw.Rewrite(raw)
	if !bytes.Contains(out, []byte(`"model":"public"`)) && !bytes.Contains(out, []byte(`"model": "public"`)) {
		t.Errorf("byte replacement did not produce expected output: %s", out)
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

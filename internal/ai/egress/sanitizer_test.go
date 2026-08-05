package egress

import (
	"bytes"
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestSanitizeJSON_OpenAIResponsesNested(t *testing.T) {
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

	got := SanitizeJSON(body, Policy{
		PublicModel:        "gpt-5.4-mini",
		UpstreamModel:      "gpt-5.4-mini",
		Protocol:           domain.ProtocolOpenAIResponses,
		AllowVersionSuffix: true,
	})

	resp := decodeMap(t, got)["response"].(map[string]any)
	if resp["model"] != "gpt-5.4-mini" {
		t.Errorf("response.model = %q, want gpt-5.4-mini", resp["model"])
	}
}

func TestSanitizeJSON_ExactUpstreamOnly(t *testing.T) {
	body := mustJSON(map[string]any{
		"model": "unrelated-model",
		"nested": map[string]any{
			"model": "upstream-internal",
		},
	})

	got := SanitizeJSON(body, Policy{
		PublicModel:   "public",
		UpstreamModel: "upstream-internal",
		Protocol:      domain.ProtocolOpenAIChat,
	})

	result := decodeMap(t, got)
	if result["model"] != "unrelated-model" {
		t.Errorf("top-level unrelated model was changed: %q", result["model"])
	}
	nested := result["nested"].(map[string]any)
	if nested["model"] != "public" {
		t.Errorf("nested model = %q, want public", nested["model"])
	}
}

func TestSanitizeJSON_DoesNotRewriteDifferentModelWithSharedPrefix(t *testing.T) {
	body := mustJSON(map[string]any{
		"model": "gpt-4o",
		"nested": map[string]any{
			"model": "gpt-4-turbo",
		},
	})

	got := SanitizeJSON(body, Policy{
		PublicModel:        "gpt-4",
		UpstreamModel:      "gpt-4",
		Protocol:           domain.ProtocolOpenAIChat,
		AllowVersionSuffix: true,
	})

	result := decodeMap(t, got)
	if result["model"] != "gpt-4o" {
		t.Errorf("shared-prefix model was changed: %q", result["model"])
	}
	nested := result["nested"].(map[string]any)
	if nested["model"] != "gpt-4-turbo" {
		t.Errorf("non-version model variant was changed: %q", nested["model"])
	}
}

func TestSanitizeJSON_DoesNotRewriteUserText(t *testing.T) {
	body := mustJSON(map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": "The upstream model gpt-5.4-mini-2026-03-17 is mentioned as plain text.",
				},
			},
		},
	})

	got := SanitizeJSON(body, Policy{
		PublicModel:        "gpt-5.4-mini",
		UpstreamModel:      "gpt-5.4-mini",
		Protocol:           domain.ProtocolOpenAIChat,
		AllowVersionSuffix: true,
	})

	if !bytes.Equal(got, body) {
		t.Fatalf("plain text mention should not be rewritten: %s", got)
	}
}

func TestSanitizeJSON_ErrorBodyModelField(t *testing.T) {
	body := mustJSON(map[string]any{
		"error": map[string]any{
			"message": "upstream rejected the request",
			"model":   "gpt-5.4-mini-2026-03-17",
		},
	})

	got := SanitizeJSON(body, Policy{
		PublicModel:        "gpt-5.4-mini",
		UpstreamModel:      "gpt-5.4-mini",
		Protocol:           domain.ProtocolOpenAIResponses,
		AllowVersionSuffix: true,
	})

	errObj := decodeMap(t, got)["error"].(map[string]any)
	if errObj["model"] != "gpt-5.4-mini" {
		t.Fatalf("error.model = %q, want gpt-5.4-mini", errObj["model"])
	}
}

func TestSanitizeJSON_GeminiModelVersion(t *testing.T) {
	body := mustJSON(map[string]any{
		"candidates":   []any{},
		"modelVersion": "gemini-2.0-flash-001",
	})

	got := SanitizeJSON(body, Policy{
		PublicModel:        "gemini-2.0-flash",
		UpstreamModel:      "gemini-2.0-flash",
		Protocol:           domain.ProtocolGeminiGenerate,
		AllowVersionSuffix: true,
	})

	result := decodeMap(t, got)
	if result["modelVersion"] != "gemini-2.0-flash" {
		t.Errorf("modelVersion = %q, want gemini-2.0-flash", result["modelVersion"])
	}
	if _, ok := result["model"]; ok {
		t.Error("unexpected model key introduced")
	}
}

func TestSanitizer_DoesNotNoopAfterFirstSSEFrameWithoutModel(t *testing.T) {
	s := NewSanitizer(Policy{
		PublicModel:   "public",
		UpstreamModel: "upstream-internal",
		Protocol:      domain.ProtocolOpenAIResponses,
	})

	first := mustJSON(map[string]any{"type": "response.output_text.delta", "delta": "hello"})
	if out := s.SanitizeSSEData(first); !bytes.Equal(out, first) {
		t.Errorf("first frame without upstream model should be unchanged: %s", out)
	}

	second := mustJSON(map[string]any{
		"type": "response.in_progress",
		"response": map[string]any{
			"id":    "resp_1",
			"model": "upstream-internal",
		},
	})
	out := s.SanitizeSSEData(second)
	if !bytes.Contains(out, []byte(`"model":"public"`)) && !bytes.Contains(out, []byte(`"model": "public"`)) {
		t.Errorf("nested response.model was not sanitized: %s", out)
	}
}

func TestSanitizeJSON_StripsRevisedPromptWhenConfigured(t *testing.T) {
	body := mustJSON(map[string]any{
		"data": []any{
			map[string]any{
				"b64_json":       "aGVsbG8=",
				"revised_prompt": "hidden system prompt",
			},
		},
	})

	got := SanitizeJSON(body, Policy{HideRevisedPrompt: true})
	item := decodeMap(t, got)["data"].([]any)[0].(map[string]any)
	if _, ok := item["revised_prompt"]; ok {
		t.Fatalf("revised_prompt should be stripped: %+v", item)
	}
	if item["b64_json"] != "aGVsbG8=" {
		t.Fatalf("b64_json = %q, want preserved", item["b64_json"])
	}
}

func TestSanitizer_SanitizeSSEDataStripsRevisedPromptWhenConfigured(t *testing.T) {
	s := NewSanitizer(Policy{HideRevisedPrompt: true})
	frame := mustJSON(map[string]any{
		"data": []any{
			map[string]any{
				"url":            "https://example.test/cat.png",
				"revised_prompt": "hidden system prompt",
			},
		},
	})

	out := s.SanitizeSSEData(frame)
	item := decodeMap(t, out)["data"].([]any)[0].(map[string]any)
	if _, ok := item["revised_prompt"]; ok {
		t.Fatalf("revised_prompt should be stripped from SSE data: %+v", item)
	}
}

func TestSanitizeText_RewritesErrorIdentity(t *testing.T) {
	got := SanitizeText(
		`upstream https://example.invalid/v1 returned model gpt-5.4-mini-2026-03-17`,
		Policy{
			PublicModel:        "gpt-5.4-mini",
			UpstreamModel:      "gpt-5.4-mini",
			EndpointBaseURL:    "https://example.invalid/v1",
			AllowVersionSuffix: true,
		},
	)
	if got != `upstream gpt-5.4-mini returned model gpt-5.4-mini` {
		t.Fatalf("SanitizeText = %q", got)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func decodeMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return result
}

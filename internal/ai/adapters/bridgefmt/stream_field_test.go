package bridgefmt

import (
	"encoding/json"
	"testing"
)

func TestRewriteJSONStreamField(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		stream bool
		want   *bool // nil = expect unchanged / non-object
	}{
		{"set false when absent", `{"model":"gpt-image-2","prompt":"x"}`, false, boolPtr(false)},
		{"set true when absent", `{"model":"gpt-image-2","prompt":"x"}`, true, boolPtr(true)},
		{"override true to false", `{"stream":true,"prompt":"x"}`, false, boolPtr(false)},
		{"override false to true", `{"stream":false,"prompt":"x"}`, true, boolPtr(true)},
		{"keep matching false", `{"stream":false}`, false, boolPtr(false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := rewriteOpenAIImageJSONFields([]byte(tc.in), tc.stream, "")
			var doc map[string]any
			if err := json.Unmarshal(out, &doc); err != nil {
				t.Fatalf("unmarshal output: %v", err)
			}
			got, ok := doc["stream"].(bool)
			if !ok {
				t.Fatalf("stream field missing in output %s", out)
			}
			if got != *tc.want {
				t.Fatalf("stream = %v, want %v", got, *tc.want)
			}
		})
	}
}

func TestRewriteJSONStreamFieldNonObject(t *testing.T) {
	// Non-object / unparseable bodies are returned as-is (e.g. multipart is not
	// routed here, but defensively a bare array must not panic).
	in := []byte(`[1,2,3]`)
	if out := rewriteOpenAIImageJSONFields(in, true, ""); string(out) != string(in) {
		t.Fatalf("non-object body was modified: %s", out)
	}
	if out := rewriteOpenAIImageJSONFields(nil, true, ""); out != nil {
		t.Fatalf("nil body was modified: %s", out)
	}
}

func TestRewriteJSONStreamFieldRemovesClientResponseFormat(t *testing.T) {
	out := rewriteOpenAIImageJSONFields([]byte(`{"model":"gpt-image-2","response_format":"url"}`), false, "")
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if _, exists := doc["response_format"]; exists {
		t.Fatalf("response_format must be removed: %s", out)
	}
}

func TestRewriteOpenAIImageJSONFieldsReplacesClientResponseFormat(t *testing.T) {
	out := rewriteOpenAIImageJSONFields(
		[]byte(`{"model":"gpt-image-2","response_format":"url"}`),
		false,
		"b64_json",
	)
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got := doc["response_format"]; got != "b64_json" {
		t.Fatalf("response_format = %#v, want b64_json", got)
	}
}

func boolPtr(b bool) *bool { return &b }

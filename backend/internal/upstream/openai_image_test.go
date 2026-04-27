package upstream

import (
	"encoding/json"
	"testing"
)

func TestBuildOpenAIImageBodyReplacesModelAndMergesDefaults(t *testing.T) {
	raw := map[string]json.RawMessage{
		"model":  json.RawMessage(`"public-image"`),
		"prompt": json.RawMessage(`"a cat"`),
		"size":   json.RawMessage(`"1024x1024"`),
	}
	defaults := []byte(`{"size":"512x512","quality":"high"}`)

	encoded, err := buildOpenAIImageBody(raw, "upstream-image", defaults)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got["model"] != "upstream-image" {
		t.Fatalf("model = %v, want upstream-image", got["model"])
	}
	if got["size"] != "1024x1024" {
		t.Fatalf("size = %v, want caller value", got["size"])
	}
	if got["quality"] != "high" {
		t.Fatalf("quality = %v, want high", got["quality"])
	}
}

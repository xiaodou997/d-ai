package upstream

import (
	"encoding/json"
	"testing"
)

func TestBuildOpenAIResponsesBodyReplacesModelAndMergesDefaults(t *testing.T) {
	raw := map[string]json.RawMessage{
		"model":       json.RawMessage(`"public-model"`),
		"input":       json.RawMessage(`"hello"`),
		"temperature": json.RawMessage(`0.1`),
	}
	defaults := []byte(`{"temperature":0.7,"reasoning":{"effort":"low"}}`)

	encoded, err := buildOpenAIResponsesBody(raw, "upstream-model", defaults)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got["model"] != "upstream-model" {
		t.Fatalf("model = %v, want upstream-model", got["model"])
	}
	if got["temperature"].(float64) != 0.1 {
		t.Fatalf("temperature = %v, want caller value", got["temperature"])
	}
	if got["reasoning"] == nil {
		t.Fatalf("reasoning default missing")
	}
}

func TestBuildOpenAIEmbeddingsBodyReplacesModelAndMergesDefaults(t *testing.T) {
	raw := map[string]json.RawMessage{
		"model": json.RawMessage(`"public-embedding"`),
		"input": json.RawMessage(`["a","b"]`),
	}
	defaults := []byte(`{"encoding_format":"float"}`)

	encoded, err := buildOpenAIEmbeddingsBody(raw, "upstream-embedding", defaults)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got["model"] != "upstream-embedding" {
		t.Fatalf("model = %v, want upstream-embedding", got["model"])
	}
	if got["encoding_format"] != "float" {
		t.Fatalf("encoding_format = %v, want float", got["encoding_format"])
	}
}

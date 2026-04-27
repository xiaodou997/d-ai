package upstream

import (
	"encoding/json"
	"testing"
)

func TestBuildOpenAIChatBodyReplacesModelAndMergesDefaults(t *testing.T) {
	raw := map[string]json.RawMessage{
		"model":       json.RawMessage(`"public-model"`),
		"temperature": json.RawMessage(`0.2`),
		"messages":    json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}
	defaults := []byte(`{"temperature":0.8,"reasoning_effort":"high"}`)

	encoded, err := buildOpenAIChatBody(raw, "upstream-model", defaults)
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
	if got["temperature"].(float64) != 0.2 {
		t.Fatalf("temperature = %v, want caller value 0.2", got["temperature"])
	}
	if got["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %v, want high", got["reasoning_effort"])
	}
}

func TestBuildEndpointURL(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		customPath string
		want       string
	}{
		{
			name:    "deepseek chat default",
			baseURL: "https://api.deepseek.com",
			want:    "https://api.deepseek.com/chat/completions",
		},
		{
			name:    "openai v1 chat default",
			baseURL: "https://api.openai.com/v1",
			want:    "https://api.openai.com/v1/chat/completions",
		},
		{
			name:       "custom path",
			baseURL:    "https://example.com/base/",
			customPath: "/custom/chat",
			want:       "https://example.com/base/custom/chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildEndpointURL(tt.baseURL, tt.customPath, "/chat/completions")
			if err != nil {
				t.Fatalf("build url: %v", err)
			}
			if got != tt.want {
				t.Fatalf("url = %q, want %q", got, tt.want)
			}
		})
	}
}

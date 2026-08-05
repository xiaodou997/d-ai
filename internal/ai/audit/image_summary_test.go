package audit

import (
	"encoding/json"
	"testing"
)

func TestSummarizeImagesResponse(t *testing.T) {
	body := json.RawMessage(`{"created":1234,"data":[{"b64_json":"abc","revised_prompt":"a cat"},{"url":"https://x/y.png"}]}`)
	out := SummarizeImagesResponse(body)
	if len(out) == 0 {
		t.Fatal("want non-empty summary")
	}
	var parsed struct {
		ImageCount  int `json:"image_count"`
		InlineCount int `json:"inline_count"`
		URLCount    int `json:"url_count"`
		Items       []struct {
			HasB64JSON    bool   `json:"has_b64_json"`
			HasURL        bool   `json:"has_url"`
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if parsed.ImageCount != 2 || parsed.InlineCount != 1 || parsed.URLCount != 1 {
		t.Fatalf("unexpected counts: %+v", parsed)
	}
	if len(parsed.Items) != 2 || parsed.Items[0].RevisedPrompt != "a cat" {
		t.Fatalf("unexpected items: %+v", parsed.Items)
	}
	if parsed.Items[1].URL != "https://x/y.png" {
		t.Fatalf("want summarized URL to be preserved, got %+v", parsed.Items[1])
	}
}

func TestSummarizeImagesResponseTreatsInlineURLPayloadAsInlineImage(t *testing.T) {
	body := json.RawMessage(`{"created":1234,"data":[{"url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO2lmw0AAAAASUVORK5CYII="}]}`)
	out := SummarizeImagesResponse(body)
	if len(out) == 0 {
		t.Fatal("want non-empty summary")
	}
	var parsed struct {
		InlineCount int `json:"inline_count"`
		URLCount    int `json:"url_count"`
		Items       []struct {
			HasB64JSON bool   `json:"has_b64_json"`
			HasURL     bool   `json:"has_url"`
			URL        string `json:"url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if parsed.InlineCount != 1 || parsed.URLCount != 0 {
		t.Fatalf("unexpected counts: %+v", parsed)
	}
	if len(parsed.Items) != 1 || !parsed.Items[0].HasB64JSON || parsed.Items[0].HasURL || parsed.Items[0].URL != "" {
		t.Fatalf("unexpected items: %+v", parsed.Items)
	}
}

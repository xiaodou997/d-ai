package console

import (
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/imageassets"
)

func TestBuildConsoleImageTaskResultPayloadKeepsUpstreamURLAlongsideStoredAssets(t *testing.T) {
	t.Parallel()

	rawResponse := []byte(`{"created":1234,"data":[{"url":"https://upstream.example/cat.png","revised_prompt":"a cat"}]}`)
	stored := []imageassets.StoredAsset{
		{
			Index:       0,
			PreviewURL:  "/runtime/v1/images/tasks/task-1/assets/0/preview?key=preview",
			DisplayURL:  "/runtime/v1/images/tasks/task-1/assets/0/preview?key=preview",
			OriginalURL: "/runtime/v1/images/tasks/task-1/assets/0/original?key=original",
		},
	}

	payload := buildConsoleImageTaskResultPayload(rawResponse, stored, "auto", true)
	if len(payload) == 0 {
		t.Fatal("want non-empty payload")
	}

	var parsed struct {
		URLCount int `json:"url_count"`
		Items    []struct {
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"items"`
		Assets []struct {
			PreviewURL  string `json:"preview_url"`
			OriginalURL string `json:"original_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if parsed.URLCount != 1 {
		t.Fatalf("url_count = %d, want 1", parsed.URLCount)
	}
	if len(parsed.Items) != 1 || parsed.Items[0].URL != "https://upstream.example/cat.png" {
		t.Fatalf("unexpected summarized items: %+v", parsed.Items)
	}
	if len(parsed.Assets) != 1 || parsed.Assets[0].PreviewURL == "" || parsed.Assets[0].OriginalURL == "" {
		t.Fatalf("unexpected stored assets: %+v", parsed.Assets)
	}
}

func TestBuildConsoleImageTaskResultPayloadFallsBackToResponseURLsWithoutStoredAssets(t *testing.T) {
	t.Parallel()

	rawResponse := []byte(`{"created":1234,"data":[{"url":"https://upstream.example/cat.png"}]}`)

	payload := buildConsoleImageTaskResultPayload(rawResponse, nil, "auto", false)
	if len(payload) == 0 {
		t.Fatal("want non-empty payload")
	}

	var parsed struct {
		Assets []struct {
			PreviewURL string `json:"preview_url"`
			DisplayURL string `json:"display_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(parsed.Assets) != 1 || parsed.Assets[0].PreviewURL != "https://upstream.example/cat.png" {
		t.Fatalf("unexpected fallback assets: %+v", parsed.Assets)
	}
}

func TestExtractConsoleImageTaskAssetsKeepsPlatformAssetReference(t *testing.T) {
	t.Parallel()

	assets := extractConsoleImageTaskAssets([]byte(`{
		"data":[{
			"asset_ref":"media://b3b1d698-1ee0-4c91-b7aa-f15ad4a856b3",
			"url":"https://api.example.test/v1/files/content/capability",
			"expires_at":"2026-07-16T14:00:00Z"
		}]
	}`))
	if len(assets) != 1 {
		t.Fatalf("assets len = %d, want 1", len(assets))
	}
	if assets[0].ID != "media://b3b1d698-1ee0-4c91-b7aa-f15ad4a856b3" {
		t.Fatalf("asset ID = %q", assets[0].ID)
	}
	if assets[0].ExpiresAt == 0 {
		t.Fatal("platform URL expiry was not retained")
	}
}

func TestExtractConsoleImageTaskAssetsDropsInlineURLPayloads(t *testing.T) {
	t.Parallel()

	rawResponse := []byte(`{"assets":[{"preview_url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO2lmw0AAAAASUVORK5CYII=","display_url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO2lmw0AAAAASUVORK5CYII=","original_url":"https://upstream.example/cat.png"}]}`)

	assets := extractConsoleImageTaskAssets(rawResponse)
	if len(assets) != 1 {
		t.Fatalf("assets len = %d, want 1", len(assets))
	}
	if assets[0].PreviewURL != "https://upstream.example/cat.png" || assets[0].DisplayURL != "https://upstream.example/cat.png" {
		t.Fatalf("unexpected sanitized asset: %+v", assets[0])
	}
}

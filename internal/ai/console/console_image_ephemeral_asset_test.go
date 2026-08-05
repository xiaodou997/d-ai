package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/ai/imageassets"
)

const ephemeralAssetTestPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/lAAAAABJRU5ErkJggg=="

func TestConsoleImageEphemeralAssetRouteServesMatchingCapabilityURL(t *testing.T) {
	svc := imageassets.New(imageassets.Config{StorageDir: t.TempDir()}, nil)
	normalized, err := svc.NormalizeOpenAIResponse(
		context.Background(),
		[]byte(`{"data":[{"b64_json":"`+ephemeralAssetTestPNGBase64+`"}]}`),
		"url",
	)
	if err != nil {
		t.Fatalf("NormalizeOpenAIResponse: %v", err)
	}
	var response struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(normalized, &response); err != nil || len(response.Data) != 1 {
		t.Fatalf("normalized response = %s, err = %v", normalized, err)
	}
	assetURL, err := url.Parse(response.Data[0].URL)
	if err != nil {
		t.Fatalf("parse normalized URL: %v", err)
	}

	console := &Console{imageAssets: svc}
	router := chi.NewRouter()
	console.Routes(router)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, assetURL.String(), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("content type = %q, want image/png", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("asset body is empty")
	}

	wrongKeyURL := *assetURL
	query := wrongKeyURL.Query()
	query.Set("key", "wrong")
	wrongKeyURL.RawQuery = query.Encode()
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, wrongKeyURL.String(), nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("wrong key status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

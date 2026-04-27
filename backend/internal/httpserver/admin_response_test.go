package httpserver

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNormalizeAdminJSONConvertsJSONBBytes(t *testing.T) {
	type nestedResponse struct {
		AllowedModels []byte `json:"allowed_models"`
	}
	type sampleResponse struct {
		Config []byte           `json:"config"`
		Nested nestedResponse   `json:"nested"`
		Items  []nestedResponse `json:"items"`
	}

	normalized := normalizeAdminJSON(sampleResponse{
		Config: []byte(`{"temperature":0.2}`),
		Nested: nestedResponse{
			AllowedModels: []byte(`["gpt-5.4"]`),
		},
		Items: []nestedResponse{
			{AllowedModels: []byte(`["deepseek-v4-pro"]`)},
		},
	})

	body, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("marshal normalized response: %v", err)
	}

	var got struct {
		Config map[string]float64 `json:"config"`
		Nested struct {
			AllowedModels []string `json:"allowed_models"`
		} `json:"nested"`
		Items []struct {
			AllowedModels []string `json:"allowed_models"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal normalized body: %v; body=%s", err, body)
	}

	if got.Config["temperature"] != 0.2 {
		t.Fatalf("config was not emitted as JSON object: %s", body)
	}
	if got.Nested.AllowedModels[0] != "gpt-5.4" {
		t.Fatalf("nested JSONB was not emitted as JSON array: %s", body)
	}
	if got.Items[0].AllowedModels[0] != "deepseek-v4-pro" {
		t.Fatalf("slice JSONB was not emitted as JSON array: %s", body)
	}
}

func TestNormalizeAdminJSONConvertsTimestampsToUnixMillis(t *testing.T) {
	type sampleResponse struct {
		CreatedAt pgtype.Timestamptz `json:"created_at"`
	}

	ts := time.Date(2026, 4, 27, 8, 30, 0, 123000000, time.UTC)
	body, err := json.Marshal(normalizeAdminJSON(sampleResponse{
		CreatedAt: pgtype.Timestamptz{Time: ts, Valid: true},
	}))
	if err != nil {
		t.Fatalf("marshal normalized response: %v", err)
	}

	var got struct {
		CreatedAt int64 `json:"created_at"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if got.CreatedAt != ts.UnixMilli() {
		t.Fatalf("created_at = %d, want %d", got.CreatedAt, ts.UnixMilli())
	}
}

func TestNormalizeAdminJSONConvertsNonJSONBytesToString(t *testing.T) {
	type sampleResponse struct {
		Value []byte `json:"value"`
	}

	body, err := json.Marshal(normalizeAdminJSON(sampleResponse{Value: []byte("plain-text")}))
	if err != nil {
		t.Fatalf("marshal normalized response: %v", err)
	}

	var got struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if got.Value != "plain-text" {
		t.Fatalf("non-JSON bytes should be emitted as a string, got %q", got.Value)
	}
}

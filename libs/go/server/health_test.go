package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth_StrongTyped200(t *testing.T) {
	r, api := New(Options{Title: "demo", Version: "1.2.3"})
	Health(api, "demo-service", "1.2.3")
	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Status, Service, Version string
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Service != "demo-service" || body.Version != "1.2.3" {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestOpenAPI_GeneratesContractWithHealthAndProblem(t *testing.T) {
	_, api := New(Options{Title: "demo", Version: "1.2.3"})
	Health(api, "demo-service", "1.2.3")

	yamlBytes, err := OpenAPIYAML(api)
	if err != nil {
		t.Fatalf("export openapi yaml: %v", err)
	}
	if !bytes.Contains(yamlBytes, []byte("/healthz")) {
		t.Errorf("openapi yaml missing /healthz path")
	}

	// JSON 形态便于结构化断言：契约须为 3.1 且错误响应媒体类型为 problem+json。
	jsonBytes, err := api.OpenAPI().MarshalJSON()
	if err != nil {
		t.Fatalf("marshal openapi json: %v", err)
	}
	var doc struct {
		OpenAPI string `json:"openapi"`
	}
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI == "" || doc.OpenAPI[0] != '3' {
		t.Errorf("openapi version = %q, want 3.x", doc.OpenAPI)
	}
	if !bytes.Contains(jsonBytes, []byte("application/problem+json")) {
		t.Errorf("openapi missing application/problem+json error content type")
	}
}

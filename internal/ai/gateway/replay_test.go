package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	coreruntime "xiaodou/dai/internal/ai/core/runtime"
)

func TestAggregateImageStreamNormalizesGenerationResult(t *testing.T) {
	t.Parallel()

	body := []byte(
		"data: {\"object\":\"image.generation.chunk\",\"data\":[]}\n\n" +
			"data: {\"object\":\"image.generation.result\",\"created\":123,\"data\":[{\"url\":\"https://example.test/cat.png\"}]}\n\n" +
			"data: {\"object\":\"image.generation.chunk\",\"upstream_event_type\":\"message_stream_complete\",\"data\":[]}\n\n",
	)
	aggregated, ok, err := AggregateImageStreamResponse(body, "text/event-stream", true)
	if err != nil {
		t.Fatalf("aggregateConsoleImageStreamResponse: %v", err)
	}
	if !ok {
		t.Fatalf("stream was not aggregated")
	}
	var parsed struct {
		Object string `json:"object"`
		Data   []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(aggregated, &parsed); err != nil {
		t.Fatalf("unmarshal aggregated response: %v", err)
	}
	if parsed.Object != "" {
		t.Fatalf("object = %q, want canonical image response without upstream wrapper", parsed.Object)
	}
	if len(parsed.Data) != 1 || parsed.Data[0].URL != "https://example.test/cat.png" {
		t.Fatalf("unexpected image data: %+v", parsed.Data)
	}
}

func TestAggregateImageStreamAcceptsCompletedEventWithImageData(t *testing.T) {
	t.Parallel()

	body := []byte("event: image_generation.completed\ndata: {\"created\":123,\"data\":[{\"url\":\"https://example.test/cat.png\"}]}\n\n")
	aggregated, ok, err := AggregateImageStreamResponse(body, "text/event-stream", true)
	if err != nil {
		t.Fatalf("aggregateConsoleImageStreamResponse: %v", err)
	}
	if !ok {
		t.Fatalf("stream was not aggregated")
	}
	var parsed struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(aggregated, &parsed); err != nil {
		t.Fatalf("unmarshal aggregated response: %v", err)
	}
	if len(parsed.Data) != 1 || parsed.Data[0].URL != "https://example.test/cat.png" {
		t.Fatalf("unexpected image data: %+v", parsed.Data)
	}
}

func TestAggregateImageStreamNormalizesTopLevelCompletedImageData(t *testing.T) {
	t.Parallel()

	body := []byte("event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"created\":123,\"b64_json\":\"aGVsbG8=\",\"usage\":{\"total_tokens\":10}}\n\n")
	aggregated, ok, err := AggregateImageStreamResponse(body, "text/event-stream", true)
	if err != nil {
		t.Fatalf("aggregateConsoleImageStreamResponse: %v", err)
	}
	if !ok {
		t.Fatalf("stream was not aggregated")
	}
	var parsed struct {
		Created int `json:"created"`
		Data    []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(aggregated, &parsed); err != nil {
		t.Fatalf("unmarshal aggregated response: %v", err)
	}
	if parsed.Created != 123 {
		t.Fatalf("created = %d, want 123", parsed.Created)
	}
	if len(parsed.Data) != 1 || parsed.Data[0].B64JSON != "aGVsbG8=" {
		t.Fatalf("unexpected image data: %+v", parsed.Data)
	}
	if parsed.Usage["total_tokens"] != float64(10) {
		t.Fatalf("unexpected usage: %+v", parsed.Usage)
	}
}

func TestAggregateImageStreamNormalizesNestedCompletedImageData(t *testing.T) {
	t.Parallel()

	body := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"created\":123,\"response\":{\"output\":[{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"output_format\":\"png\"}],\"usage\":{\"total_tokens\":10}}}\n\n")
	aggregated, ok, err := AggregateImageStreamResponse(body, "text/event-stream", true)
	if err != nil {
		t.Fatalf("aggregateConsoleImageStreamResponse: %v", err)
	}
	if !ok {
		t.Fatalf("stream was not aggregated")
	}
	var parsed struct {
		Data []struct {
			B64JSON      string `json:"b64_json"`
			OutputFormat string `json:"output_format"`
		} `json:"data"`
	}
	if err := json.Unmarshal(aggregated, &parsed); err != nil {
		t.Fatalf("unmarshal aggregated response: %v", err)
	}
	if len(parsed.Data) != 1 || parsed.Data[0].B64JSON != "aGVsbG8=" || parsed.Data[0].OutputFormat != "png" {
		t.Fatalf("unexpected image data: %+v", parsed.Data)
	}
}

func TestReplayCallerChargeMicroUsesOwnerCost(t *testing.T) {
	t.Parallel()

	tenantReq := &coreruntime.Result{CallerChargeMicro: 120000}
	if got := ReplayCallerChargeMicro(tenantReq); got != 120000 {
		t.Fatalf("tenant caller charge = %d, want 120000", got)
	}

	userReq := &coreruntime.Result{CallerChargeMicro: 180000}
	if got := ReplayCallerChargeMicro(userReq); got != 180000 {
		t.Fatalf("user caller charge = %d, want 180000", got)
	}

	apiQuotaReq := &coreruntime.Result{CallerChargeMicro: 150000}
	if got := ReplayCallerChargeMicro(apiQuotaReq); got != 150000 {
		t.Fatalf("api key caller charge = %d, want 150000", got)
	}
}

func TestBuildReplayRequestHeaderPrecedence(t *testing.T) {
	t.Parallel()

	req, err := buildReplayRequest(context.Background(), ReplayInput{
		ClientPath:  "/v1/images/generations",
		Body:        []byte(`{"prompt":"a cat"}`),
		ContentType: "application/json",
		RequestID:   "atsk_chosen_1",
		// A forwarding caller's inbound headers must not override the body's
		// real content type, nor the request id we allocated for reconciliation.
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
			"X-Request-Id": []string{"client-supplied"},
			"X-Custom":     []string{"kept"},
		},
	})
	if err != nil {
		t.Fatalf("buildReplayRequest: %v", err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want the replay's own application/json", got)
	}
	if got := req.Header.Get("X-Request-Id"); got != "atsk_chosen_1" {
		t.Fatalf("X-Request-Id = %q, want the pre-allocated atsk_chosen_1", got)
	}
	if got := req.Header.Get("X-Custom"); got != "kept" {
		t.Fatalf("X-Custom = %q, want forwarded headers to survive", got)
	}
	if req.ContentLength != int64(len(`{"prompt":"a cat"}`)) {
		t.Fatalf("ContentLength = %d", req.ContentLength)
	}
	if req.URL.Path != "/v1/images/generations" {
		// ClientPath is semantic: image normalization tells generation from edit by it.
		t.Fatalf("path = %q", req.URL.Path)
	}
}

func TestBuildReplayRequestDefaultsAndOmissions(t *testing.T) {
	t.Parallel()

	req, err := buildReplayRequest(context.Background(), ReplayInput{
		ClientPath: "/v1/images/edits",
		Body:       []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("buildReplayRequest: %v", err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want the application/json default", got)
	}
	// A queued task with no pre-allocated id must not send an empty header;
	// newRequestID would then take "" and generate one, but only if absent.
	if _, ok := req.Header["X-Request-Id"]; ok {
		t.Fatal("X-Request-Id was set despite no RequestID being supplied")
	}
}

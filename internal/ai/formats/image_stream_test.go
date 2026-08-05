package formats

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestEncodeOpenAIImageCompletedSSESelectsSupportedImageResult(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "inline base64",
			body: `{"data":[{"b64_json":"aGVsbG8=","url":"https://example.test/ignored.png"}]}`,
			want: `"b64_json":"aGVsbG8="`,
		},
		{
			name: "http url fallback",
			body: `{"data":[{"url":"https://example.test/generated.png"}]}`,
			want: `"url":"https://example.test/generated.png"`,
		},
		{
			name: "inline base64 in url field",
			body: `{"data":[{"url":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Jf5kAAAAASUVORK5CYII="}]}`,
			want: `"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Jf5kAAAAASUVORK5CYII="`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream, err := EncodeOpenAIImageCompletedSSE([]byte(tc.body), "image_generation.completed")
			if err != nil {
				t.Fatalf("EncodeOpenAIImageCompletedSSE err = %v", err)
			}
			if !bytes.Contains(stream, []byte(tc.want)) {
				t.Fatalf("stream = %q, want %q", stream, tc.want)
			}
		})
	}
}

func TestDecodeOpenAIImageResponseRejectsMissingImage(t *testing.T) {
	_, err := DecodeOpenAIImageResponse([]byte(`{"data":[{"revised_prompt":"no image"}]}`))
	if !errors.Is(err, ErrOpenAIImageResponseNoImage) {
		t.Fatalf("error = %v, want ErrOpenAIImageResponseNoImage", err)
	}
}

func TestEncodeOpenAIImageCompletedSSEEmitsEveryImage(t *testing.T) {
	stream, err := EncodeOpenAIImageCompletedSSE(
		[]byte(`{"data":[{"b64_json":"Zmlyc3Q="},{"url":"https://example.test/second.png"}]}`),
		"image_generation.completed",
	)
	if err != nil {
		t.Fatalf("EncodeOpenAIImageCompletedSSE: %v", err)
	}
	if bytes.Count(stream, []byte("event: image_generation.completed")) != 2 {
		t.Fatalf("stream = %q", stream)
	}
	for _, want := range []string{`"output_index":0`, `"output_index":1`, `"b64_json":"Zmlyc3Q="`, `"url":"https://example.test/second.png"`} {
		if !bytes.Contains(stream, []byte(want)) {
			t.Fatalf("stream missing %s: %q", want, stream)
		}
	}
}

func TestAggregateOpenAIImageSSEAcceptsImageGenerationResult(t *testing.T) {
	raw := []byte(
		"data: {\"object\":\"image.generation.chunk\",\"created\":123,\"model\":\"gpt-image-2\",\"data\":[]}\n\n" +
			":\n\n" +
			"data: {\"object\":\"image.generation.result\",\"created\":124,\"model\":\"gpt-image-2\",\"data\":[{\"b64_json\":\"aGVsbG8=\",\"revised_prompt\":\"A cute sea otter\"}],\"usage\":{\"input_tokens\":6,\"output_tokens\":1056,\"total_tokens\":1062}}\n\n" +
			"data: [DONE]\n\n",
	)

	aggregated, err := AggregateOpenAIImageSSE(raw)
	if err != nil {
		t.Fatalf("AggregateOpenAIImageSSE err = %v", err)
	}
	var result struct {
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Data    []struct {
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(aggregated, &result); err != nil {
		t.Fatalf("aggregated body is not JSON: %v", err)
	}
	if result.Created != 124 || result.Model != "gpt-image-2" {
		t.Fatalf("metadata = %#v/%q, want 124/gpt-image-2", result.Created, result.Model)
	}
	if len(result.Data) != 1 || result.Data[0].B64JSON != "aGVsbG8=" || result.Data[0].RevisedPrompt != "A cute sea otter" {
		t.Fatalf("image data = %+v", result.Data)
	}
	if result.Usage["total_tokens"] != float64(1062) {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestAggregateOpenAIImageSSEAcceptsCompletedTopLevelImage(t *testing.T) {
	raw := []byte("event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"created\":123,\"model\":\"gpt-image-2\",\"b64_json\":\"aGVsbG8=\",\"usage\":{\"total_tokens\":10}}\n\n")

	aggregated, err := AggregateOpenAIImageSSE(raw)
	if err != nil {
		t.Fatalf("AggregateOpenAIImageSSE err = %v", err)
	}
	var result struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(aggregated, &result); err != nil {
		t.Fatalf("aggregated body is not JSON: %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].B64JSON != "aGVsbG8=" {
		t.Fatalf("image data = %+v", result.Data)
	}
	if result.Usage["total_tokens"] != float64(10) {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestAggregateOpenAIImageSSEAcceptsCompletedEventWithImageData(t *testing.T) {
	raw := []byte("event: image_generation.completed\ndata: {\"created\":123,\"model\":\"gpt-image-2\",\"data\":[{\"b64_json\":\"aGVsbG8=\",\"revised_prompt\":\"A cute sea otter\"}],\"usage\":{\"total_tokens\":10}}\n\n")

	aggregated, err := AggregateOpenAIImageSSE(raw)
	if err != nil {
		t.Fatalf("AggregateOpenAIImageSSE err = %v", err)
	}
	var result struct {
		Data []struct {
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(aggregated, &result); err != nil {
		t.Fatalf("aggregated body is not JSON: %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].B64JSON != "aGVsbG8=" || result.Data[0].RevisedPrompt != "A cute sea otter" {
		t.Fatalf("image data = %+v", result.Data)
	}
}

func TestAggregateOpenAIImageSSEAcceptsResponsesImageEvents(t *testing.T) {
	raw := []byte(
		"event: response.output_item.done\n" +
			"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"output_format\":\"png\",\"revised_prompt\":\"A cute sea otter\"}}\n\n" +
			"event: response.completed\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":123,\"model\":\"gpt-image-2\",\"usage\":{\"input_tokens\":6,\"output_tokens\":1056,\"total_tokens\":1062}}}\n\n",
	)

	aggregated, err := AggregateOpenAIImageSSE(raw)
	if err != nil {
		t.Fatalf("AggregateOpenAIImageSSE err = %v", err)
	}
	var result struct {
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Data    []struct {
			B64JSON       string `json:"b64_json"`
			OutputFormat  string `json:"output_format"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(aggregated, &result); err != nil {
		t.Fatalf("aggregated body is not JSON: %v", err)
	}
	if result.Created != 123 || result.Model != "gpt-image-2" {
		t.Fatalf("metadata = %#v/%q, want 123/gpt-image-2", result.Created, result.Model)
	}
	if len(result.Data) != 1 || result.Data[0].B64JSON != "aGVsbG8=" || result.Data[0].OutputFormat != "png" || result.Data[0].RevisedPrompt != "A cute sea otter" {
		t.Fatalf("image data = %+v", result.Data)
	}
	if result.Usage["total_tokens"] != float64(1062) {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestAggregateOpenAIImageSSEPreservesOutputItemMetadata(t *testing.T) {
	raw := []byte("event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"created\":123,\"model\":\"gpt-image-2\",\"usage\":{\"total_tokens\":10},\"item\":{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\"}}\n\n")

	aggregated, err := AggregateOpenAIImageSSE(raw)
	if err != nil {
		t.Fatalf("AggregateOpenAIImageSSE err = %v", err)
	}
	var result struct {
		Created int64          `json:"created"`
		Model   string         `json:"model"`
		Usage   map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(aggregated, &result); err != nil {
		t.Fatalf("aggregated body is not JSON: %v", err)
	}
	if result.Created != 123 || result.Model != "gpt-image-2" {
		t.Fatalf("metadata = %d/%q, want 123/gpt-image-2", result.Created, result.Model)
	}
	if result.Usage["total_tokens"] != float64(10) {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestAggregateOpenAIImageSSEPreservesOuterResponsesMetadata(t *testing.T) {
	raw := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"created\":123,\"response\":{\"output\":[{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\"}],\"usage\":{\"total_tokens\":10}}}\n\n")

	aggregated, err := AggregateOpenAIImageSSE(raw)
	if err != nil {
		t.Fatalf("AggregateOpenAIImageSSE err = %v", err)
	}
	var result struct {
		Created int64 `json:"created"`
	}
	if err := json.Unmarshal(aggregated, &result); err != nil {
		t.Fatalf("aggregated body is not JSON: %v", err)
	}
	if result.Created != 123 {
		t.Fatalf("created = %d, want 123", result.Created)
	}
}

func TestAggregateOpenAIImageSSEReturnsExplicitUpstreamError(t *testing.T) {
	raw := []byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"image generation failed\"}}\n\n")

	_, err := AggregateOpenAIImageSSE(raw)
	if !errors.Is(err, ErrOpenAIImageStreamUpstream) {
		t.Fatalf("error = %v, want ErrOpenAIImageStreamUpstream", err)
	}
}

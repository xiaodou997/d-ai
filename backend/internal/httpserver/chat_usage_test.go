package httpserver

import (
	"encoding/json"
	"testing"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/upstream"
)

func TestTokenCost(t *testing.T) {
	tests := []struct {
		name       string
		tokens     int32
		pricePer1M int64
		want       int64
	}{
		{name: "zero tokens", tokens: 0, pricePer1M: 1000, want: 0},
		{name: "zero price", tokens: 1000, pricePer1M: 0, want: 0},
		{name: "rounds up small usage", tokens: 1, pricePer1M: 1000, want: 1},
		{name: "exact million", tokens: 1_000_000, pricePer1M: 1000, want: 1000},
		{name: "rounds up fractional", tokens: 1_500_000, pricePer1M: 1000, want: 1500},
		{name: "rounds up remainder", tokens: 1_500_001, pricePer1M: 1000, want: 1501},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenCost(tt.tokens, tt.pricePer1M)
			if got != tt.want {
				t.Fatalf("tokenCost(%d, %d) = %d, want %d", tt.tokens, tt.pricePer1M, got, tt.want)
			}
		})
	}
}

func TestImageCost(t *testing.T) {
	tests := []struct {
		name  string
		count int32
		unit  int64
		want  int64
	}{
		{name: "zero images", count: 0, unit: 10, want: 0},
		{name: "zero price", count: 3, unit: 0, want: 0},
		{name: "multiplies image count by unit price", count: 4, unit: 7, want: 28},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imageCost(tt.count, tt.unit)
			if got != tt.want {
				t.Fatalf("imageCost(%d, %d) = %d, want %d", tt.count, tt.unit, got, tt.want)
			}
		})
	}
}

func TestValidatePriceCreditsRejectsNegativeValues(t *testing.T) {
	if got := validateModelPriceCredits(createModelPriceRequest{TenantImagePrice: -1}); got == "" {
		t.Fatal("validateModelPriceCredits accepted a negative credit value")
	}
	if got := validateProviderModelPriceCredits(createProviderModelPriceRequest{ImageCost: -1}); got == "" {
		t.Fatal("validateProviderModelPriceCredits accepted a negative credit value")
	}
}

func TestRequestedOutputTokens(t *testing.T) {
	raw := map[string]json.RawMessage{
		"max_tokens": json.RawMessage(`123`),
	}
	if got := requestedOutputTokens(raw, 4096); got != 123 {
		t.Fatalf("requestedOutputTokens = %d, want 123", got)
	}

	raw = map[string]json.RawMessage{}
	if got := requestedOutputTokens(raw, 4096); got != 4096 {
		t.Fatalf("requestedOutputTokens default = %d, want 4096", got)
	}
}

func TestEstimateChatQuotaCost(t *testing.T) {
	raw := map[string]json.RawMessage{
		"max_completion_tokens": json.RawMessage(`1000`),
	}
	price := dbgen.GetActiveModelPriceRow{
		TenantOutputPricePer1m: 2000,
	}
	if got := estimateChatQuotaCost(raw, 4096, price); got != 2 {
		t.Fatalf("estimateChatQuotaCost = %d, want 2", got)
	}
}

func TestEstimateNonStreamChatUsage(t *testing.T) {
	raw := map[string]json.RawMessage{
		"messages": json.RawMessage(`[{"role":"user","content":"hello there"}]`),
	}
	resp := &upstream.Response{
		StatusCode: 200,
		Body:       []byte(`{"choices":[{"message":{"content":"general kenobi"}}]}`),
	}

	usage := estimateNonStreamChatUsage(raw, resp)
	if usage.PromptTokens <= 0 {
		t.Fatalf("PromptTokens = %d, want positive", usage.PromptTokens)
	}
	if usage.CompletionTokens <= 0 {
		t.Fatalf("CompletionTokens = %d, want positive", usage.CompletionTokens)
	}
	if usage.TotalTokens != usage.PromptTokens+usage.CompletionTokens {
		t.Fatalf("TotalTokens = %d, want %d", usage.TotalTokens, usage.PromptTokens+usage.CompletionTokens)
	}
}

func TestParseOpenAIResponsesUsage(t *testing.T) {
	resp := &upstream.Response{
		StatusCode: 200,
		Body:       []byte(`{"usage":{"input_tokens":12,"output_tokens":8,"total_tokens":20}}`),
	}
	usage := parseOpenAIResponsesUsage(resp)
	if usage.PromptTokens != 12 || usage.CompletionTokens != 8 || usage.TotalTokens != 20 {
		t.Fatalf("usage = %+v, want 12/8/20", usage)
	}
}

func TestResponsesSSEUsageParser(t *testing.T) {
	parser := newResponsesSSEUsageParser()
	parser.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hel\"}\n\n"))
	parser.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n"))
	parser.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n"))
	parser.Close()
	usage := parser.Usage()
	if parser.OutputText() != "hello" {
		t.Fatalf("output = %q, want hello", parser.OutputText())
	}
	if usage.PromptTokens != 3 || usage.CompletionTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v, want 3/2/5", usage)
	}
}

func TestParseOpenAIEmbeddingsUsage(t *testing.T) {
	resp := &upstream.Response{
		StatusCode: 200,
		Body:       []byte(`{"usage":{"prompt_tokens":9,"total_tokens":9}}`),
	}
	usage := parseOpenAIEmbeddingsUsage(resp)
	if usage.PromptTokens != 9 || usage.TotalTokens != 9 {
		t.Fatalf("usage = %+v, want prompt/total 9", usage)
	}
}

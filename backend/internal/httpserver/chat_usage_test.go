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

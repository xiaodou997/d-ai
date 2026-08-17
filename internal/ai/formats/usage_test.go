package formats

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

// The bill is unit price × token counts. The price side is covered in
// adapters/postgres; this file covers the counts, which come from parsing four
// different upstream shapes. A decoder that is wrong here mis-bills every
// request served through that protocol, silently and systematically.

// requireBillable asserts the invariant the pricing math depends on:
// PromptTokens is the whole input context, so the cached parts are a subset of
// it. priceBreakdown computes nonCachedIn = Prompt - CacheWrite - CacheRead and
// hard-errors when that is negative, which at settlement time means a request
// that cannot be priced at all.
func requireBillable(t *testing.T, u domain.TokenUsage) {
	t.Helper()
	if u.CacheReadTokens+u.CacheWriteTokens > u.PromptTokens {
		t.Fatalf("cache tokens exceed prompt tokens: %+v — this usage cannot be priced", u)
	}
	if u.ReasoningTokens > u.CompletionTokens {
		t.Fatalf("reasoning tokens exceed completion tokens: %+v", u)
	}
	if u.PromptTokens < 0 || u.CompletionTokens < 0 {
		t.Fatalf("negative token counts: %+v", u)
	}
}

func TestExtractSyncUsagePerProtocol(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol domain.UpstreamProtocol
		body     string
		want     domain.TokenUsage
	}{
		{
			name:     "openai chat folds cached into prompt",
			protocol: domain.ProtocolOpenAIChat,
			body: `{"usage":{"prompt_tokens":1000,"completion_tokens":200,
			        "prompt_tokens_details":{"cached_tokens":400},
			        "completion_tokens_details":{"reasoning_tokens":50}}}`,
			want: domain.TokenUsage{
				PromptTokens: 1000, CompletionTokens: 200,
				CacheReadTokens: 400, ReasoningTokens: 50,
			},
		},
		{
			name:     "openai responses uses input/output naming",
			protocol: domain.ProtocolOpenAIResponses,
			body: `{"usage":{"input_tokens":900,"output_tokens":150,
			        "input_tokens_details":{"cached_tokens":300},
			        "output_tokens_details":{"reasoning_tokens":40}}}`,
			want: domain.TokenUsage{
				PromptTokens: 900, CompletionTokens: 150,
				CacheReadTokens: 300, ReasoningTokens: 40,
			},
		},
		{
			// Anthropic reports uncached input separately, so prompt tokens
			// must be reassembled or the cached portion is billed twice.
			name:     "anthropic sums uncached and cached input",
			protocol: domain.ProtocolAnthropicMessages,
			body: `{"usage":{"input_tokens":100,"output_tokens":80,
			        "cache_creation_input_tokens":300,"cache_read_input_tokens":600}}`,
			want: domain.TokenUsage{
				PromptTokens: 1000, CompletionTokens: 80,
				CacheWriteTokens: 300, CacheReadTokens: 600,
			},
		},
		{
			// Gemini keeps thoughts outside candidates, unlike the others.
			name:     "gemini folds thoughts into completion",
			protocol: domain.ProtocolGeminiGenerate,
			body: `{"usageMetadata":{"promptTokenCount":500,"candidatesTokenCount":120,
			        "thoughtsTokenCount":30,"cachedContentTokenCount":200}}`,
			want: domain.TokenUsage{
				PromptTokens: 500, CompletionTokens: 150,
				CacheReadTokens: 200, ReasoningTokens: 30,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractSyncUsage([]byte(tc.body), tc.protocol)
			if got != tc.want {
				t.Fatalf("usage = %+v, want %+v", got, tc.want)
			}
			requireBillable(t, got)
		})
	}
}

// A response without usage bills nothing rather than guessing.
func TestExtractSyncUsageMissingOrMalformedIsZero(t *testing.T) {
	for _, protocol := range []domain.UpstreamProtocol{
		domain.ProtocolOpenAIChat,
		domain.ProtocolOpenAIResponses,
		domain.ProtocolAnthropicMessages,
		domain.ProtocolGeminiGenerate,
	} {
		for _, body := range []string{`{}`, `{"usage":null}`, `not json`, ``} {
			if got := ExtractSyncUsage([]byte(body), protocol); got != (domain.TokenUsage{}) {
				t.Fatalf("%s / %q usage = %+v, want zero", protocol, body, got)
			}
		}
	}
}

// Streaming providers publish running totals, so each chunk replaces the
// previous value. Adding them instead would multiply the bill by chunk count.
func TestStreamUsageIsCumulativeNotAdditive(t *testing.T) {
	for _, tc := range []struct {
		name      string
		protocol  domain.UpstreamProtocol
		eventType string
		chunks    []string
		want      domain.TokenUsage
	}{
		{
			name:     "openai chat terminal chunk wins",
			protocol: domain.ProtocolOpenAIChat,
			chunks: []string{
				`{"choices":[{"delta":{"content":"a"}}]}`,
				`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5}}`,
				`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":12}}`,
			},
			want: domain.TokenUsage{PromptTokens: 100, CompletionTokens: 12},
		},
		{
			name:     "gemini latest chunk wins",
			protocol: domain.ProtocolGeminiGenerate,
			chunks: []string{
				`{"usageMetadata":{"promptTokenCount":80,"candidatesTokenCount":4}}`,
				`{"usageMetadata":{"promptTokenCount":80,"candidatesTokenCount":9}}`,
				`{"usageMetadata":{"promptTokenCount":80,"candidatesTokenCount":21,"thoughtsTokenCount":6}}`,
			},
			want: domain.TokenUsage{PromptTokens: 80, CompletionTokens: 27, ReasoningTokens: 6},
		},
		{
			name:      "openai responses reads the nested completed object",
			protocol:  domain.ProtocolOpenAIResponses,
			eventType: "response.completed",
			chunks: []string{
				`{"type":"response.output_text.delta","delta":"hi"}`,
				`{"type":"response.completed","response":{"usage":{"input_tokens":70,"output_tokens":13}}}`,
			},
			want: domain.TokenUsage{PromptTokens: 70, CompletionTokens: 13},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var usage domain.TokenUsage
			sawUsage := false
			for _, chunk := range tc.chunks {
				var carried bool
				usage, carried = ExtractStreamUsage(usage, []byte(chunk), tc.eventType, tc.protocol)
				sawUsage = sawUsage || carried
			}
			if !sawUsage {
				t.Fatal("no chunk was reported as carrying usage")
			}
			if usage != tc.want {
				t.Fatalf("usage = %+v, want %+v", usage, tc.want)
			}
			requireBillable(t, usage)
		})
	}
}

// Anthropic splits usage across two events: cache counters arrive in
// message_start and the authoritative output count in message_delta. Losing
// either half changes the bill.
func TestAnthropicStreamUsageMergesAcrossEvents(t *testing.T) {
	usage, ok := ExtractStreamUsage(domain.TokenUsage{},
		[]byte(`{"message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":200,"cache_read_input_tokens":700}}}`),
		"message_start", domain.ProtocolAnthropicMessages)
	if !ok {
		t.Fatal("message_start carried no usage")
	}
	if usage.PromptTokens != 1000 || usage.CacheWriteTokens != 200 || usage.CacheReadTokens != 700 {
		t.Fatalf("after message_start: %+v", usage)
	}

	// The terminal delta omits cache counters; they must survive it.
	usage, ok = ExtractStreamUsage(usage,
		[]byte(`{"usage":{"output_tokens":42}}`),
		"message_delta", domain.ProtocolAnthropicMessages)
	if !ok {
		t.Fatal("message_delta carried no usage")
	}
	want := domain.TokenUsage{
		PromptTokens: 1000, CompletionTokens: 42,
		CacheWriteTokens: 200, CacheReadTokens: 700,
	}
	if usage != want {
		t.Fatalf("after message_delta: %+v, want %+v", usage, want)
	}
	requireBillable(t, usage)
}

// Some compatible upstreams send provisional zeros first and the real numbers
// in the terminal event; the terminal event has to win even when it reports a
// lower output count than a mid-stream chunk did.
func TestAnthropicTerminalEventOverridesProvisionalUsage(t *testing.T) {
	usage, _ := ExtractStreamUsage(domain.TokenUsage{},
		[]byte(`{"message":{"usage":{"input_tokens":0,"output_tokens":0}}}`),
		"message_start", domain.ProtocolAnthropicMessages)
	usage, _ = ExtractStreamUsage(usage,
		[]byte(`{"usage":{"output_tokens":99}}`),
		"message_delta", domain.ProtocolAnthropicMessages)
	usage, _ = ExtractStreamUsage(usage,
		[]byte(`{"usage":{"input_tokens":500,"output_tokens":30}}`),
		"message_delta", domain.ProtocolAnthropicMessages)

	if usage.PromptTokens != 500 || usage.CompletionTokens != 30 {
		t.Fatalf("terminal snapshot did not win: %+v", usage)
	}
	requireBillable(t, usage)
}

// A chunk with no usage must leave the running total untouched and say so, so
// the caller does not mistake it for a usage update.
func TestStreamUsageWithoutUsagePreservesPrevious(t *testing.T) {
	prev := domain.TokenUsage{PromptTokens: 10, CompletionTokens: 3}
	for _, protocol := range []domain.UpstreamProtocol{
		domain.ProtocolOpenAIChat,
		domain.ProtocolOpenAIResponses,
		domain.ProtocolAnthropicMessages,
		domain.ProtocolGeminiGenerate,
	} {
		got, carried := ExtractStreamUsage(prev, []byte(`{"choices":[{"delta":{}}]}`), "", protocol)
		if carried {
			t.Fatalf("%s reported usage on a chunk that has none", protocol)
		}
		if got != prev {
			t.Fatalf("%s mutated usage = %+v, want %+v", protocol, got, prev)
		}
	}
}

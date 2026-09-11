package formats

import (
	"testing"
	"xiaodou/dai/internal/ai/domain"
)

func TestReportedUsagePresenceAndProtocolSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, body                      string
		protocol                        domain.UpstreamProtocol
		reported                        bool
		input, output, cache, reasoning int
	}{
		{"absent", `{}`, domain.ProtocolOpenAIResponses, false, 0, 0, 0, 0},
		{"null", `{"usage":null}`, domain.ProtocolOpenAIResponses, false, 0, 0, 0, 0},
		{"empty", `{"usage":{}}`, domain.ProtocolOpenAIResponses, false, 0, 0, 0, 0},
		{"zero", `{"response":{"usage":{"input_tokens":0,"output_tokens":0}}}`, domain.ProtocolOpenAIResponses, true, 0, 0, 0, 0},
		{"partial", `{"usage":{"output_tokens":2}}`, domain.ProtocolOpenAIResponses, true, 0, 2, 0, 0},
		{"invalid", `{"usage":{"input_tokens":-1,"output_tokens":2.5}}`, domain.ProtocolOpenAIResponses, false, 0, 0, 0, 0},
		{"responses_failed", `{"response":{"status":"failed","usage":{"input_tokens":9,"output_tokens":2,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":1}}}}`, domain.ProtocolOpenAIResponses, true, 9, 2, 4, 1},
		{"chat", `{"usage":{"prompt_tokens":9,"completion_tokens":2}}`, domain.ProtocolOpenAIChat, true, 9, 2, 0, 0},
		{"embedding_total", `{"usage":{"total_tokens":9}}`, domain.ProtocolOpenAIEmbeddings, true, 9, 0, 0, 0},
		{"anthropic", `{"message":{"usage":{"input_tokens":9,"output_tokens":2,"cache_read_input_tokens":4,"cache_creation_input_tokens":3}}}`, domain.ProtocolAnthropicMessages, true, 16, 2, 4, 0},
		{"gemini", `{"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":2,"thoughtsTokenCount":3,"cachedContentTokenCount":4}}`, domain.ProtocolGeminiGenerate, true, 9, 5, 4, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := ObserveUsage(domain.UsageEvidence{}, []byte(tc.body), "json", tc.protocol)
			u := e.Tokens()
			if e.Reported() != tc.reported || u.PromptTokens != tc.input || u.CompletionTokens != tc.output || u.CacheReadTokens != tc.cache || u.ReasoningTokens != tc.reasoning {
				t.Fatalf("evidence=%+v tokens=%+v", e, u)
			}
		})
	}
}

func TestReportedUsageSnapshotsDoNotAccumulateOrInvent(t *testing.T) {
	p := domain.ProtocolOpenAIResponses
	e := ObserveUsage(domain.UsageEvidence{}, []byte(`{"usage":{"input_tokens":100,"output_tokens":10}}`), "response.in_progress", p)
	e = ObserveUsage(e, []byte(`{"response":{"usage":{"input_tokens":80,"output_tokens":5}}}`), "response.completed", p)
	e = ObserveUsage(e, []byte(`{"response":{"usage":{"input_tokens":80,"output_tokens":5}}}`), "response.completed", p)
	if got := e.Tokens(); got.PromptTokens != 80 || got.CompletionTokens != 5 {
		t.Fatalf("duplicate terminal: %+v", got)
	}
	e = ObserveUsage(e, []byte(`{"response":{"usage":{"input_tokens":0,"output_tokens":0}}}`), "response.failed", p)
	if !e.IgnoredZeroTerminal || e.Tokens().PromptTokens != 80 {
		t.Fatalf("zero terminal erased prior usage: %+v", e)
	}
	e = ObserveUsage(e, []byte(`{"response":{"usage":{"output_tokens":7}}}`), "response.failed", p)
	if got := e.Tokens(); got.PromptTokens != 0 || got.CompletionTokens != 7 {
		t.Fatalf("terminal must replace whole Responses snapshot: %+v", got)
	}
	claude := ObserveUsage(domain.UsageEvidence{}, []byte(`{"message":{"usage":{"input_tokens":5,"output_tokens":0,"cache_read_input_tokens":3}}}`), "message_start", domain.ProtocolAnthropicMessages)
	claude = ObserveUsage(claude, []byte(`{"usage":{"output_tokens":4}}`), "message_delta", domain.ProtocolAnthropicMessages)
	if got := claude.Tokens(); got.PromptTokens != 8 || got.CompletionTokens != 4 {
		t.Fatalf("split usage: %+v", got)
	}
}

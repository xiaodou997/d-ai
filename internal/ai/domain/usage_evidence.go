package domain

// UsageEvidence holds only counters explicitly present in an upstream payload.
// A present zero is different from an absent field. Counters are cumulative,
// never per-frame increments. Protocol preserves cache/reasoning semantics.
type UsageEvidence struct {
	Protocol            UpstreamProtocol `json:"protocol,omitempty"`
	Fields              map[string]int   `json:"reported_fields,omitempty"`
	Event               string           `json:"event,omitempty"`
	Terminal            bool             `json:"terminal_snapshot"`
	IgnoredZeroTerminal bool             `json:"ignored_zero_terminal,omitempty"`
	InvalidFields       []string         `json:"invalid_fields,omitempty"`
}

func (e UsageEvidence) Reported() bool { return len(e.Fields) > 0 }

func (e UsageEvidence) Tokens() TokenUsage {
	f := e.Fields
	u := TokenUsage{PromptTokens: f["input_tokens"], CompletionTokens: f["output_tokens"],
		CacheReadTokens: f["cache_read_tokens"], CacheWriteTokens: f["cache_write_tokens"], ReasoningTokens: f["reasoning_tokens"]}
	if e.Protocol == ProtocolAnthropicMessages {
		u.PromptTokens += u.CacheReadTokens + u.CacheWriteTokens
	}
	if e.Protocol == ProtocolGeminiGenerate || e.Protocol == ProtocolGeminiEmbeddings {
		u.CompletionTokens += u.ReasoningTokens
	}
	return u
}

func (e UsageEvidence) HasTokens() bool {
	for _, value := range e.Fields {
		if value > 0 {
			return true
		}
	}
	return false
}

// UsesReportedTokenBilling excludes media and per-asset settlement. Empty is
// the legacy chat capability used by internal callers.
func UsesReportedTokenBilling(capability CapabilityType) bool {
	return capability == "" || capability == CapabilityChat || capability == CapabilityEmbedding || capability == CapabilityRerank
}

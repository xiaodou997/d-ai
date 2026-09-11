package formats

import (
	"encoding/json"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

// ObserveUsage reads provider-format bytes, before any client conversion or
// synthesized finish frame. Missing/invalid counters never acquire a value.
func ObserveUsage(prev domain.UsageEvidence, data []byte, event string, protocol domain.UpstreamProtocol, capability ...domain.CapabilityType) domain.UsageEvidence {
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return prev
	}
	obj := func(raw json.RawMessage) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		_ = json.Unmarshal(raw, &m)
		return m
	}
	if event == "" {
		_ = json.Unmarshal(root["type"], &event)
	}
	if event == "" {
		event = "json"
	}
	terminal := event == "json" || event == "message_delta" || event == "message_stop" ||
		strings.HasPrefix(event, "response.completed") || event == "response.done" || event == "response.failed" ||
		event == "response.incomplete" || event == "response.cancelled" || event == "response.canceled"
	usage := obj(root["usage"])
	if usage == nil {
		usage = obj(obj(root["response"])["usage"])
	}
	if usage == nil && protocol == domain.ProtocolAnthropicMessages {
		usage = obj(obj(root["message"])["usage"])
	}
	if protocol == domain.ProtocolGeminiGenerate || protocol == domain.ProtocolGeminiEmbeddings {
		usage = obj(root["usageMetadata"])
		if usage == nil {
			usage = obj(obj(root["response"])["usageMetadata"])
		}
	}
	if usage == nil {
		return prev
	}
	fields := map[string]int{}
	invalid := append([]string(nil), prev.InvalidFields...)
	read := func(dst string, m map[string]json.RawMessage, keys ...string) {
		for _, key := range keys {
			raw, exists := m[key]
			if !exists || string(raw) == "null" {
				continue
			}
			var n int
			if json.Unmarshal(raw, &n) != nil || n < 0 || n > 1_000_000_000_000 {
				if len(invalid) < 16 {
					invalid = append(invalid, key)
				}
				return
			}
			fields[dst] = n
			return
		}
	}
	switch protocol {
	case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
		read("input_tokens", usage, "promptTokenCount")
		read("output_tokens", usage, "candidatesTokenCount")
		read("cache_read_tokens", usage, "cachedContentTokenCount")
		read("reasoning_tokens", usage, "thoughtsTokenCount")
	case domain.ProtocolAnthropicMessages:
		read("input_tokens", usage, "input_tokens")
		read("output_tokens", usage, "output_tokens")
		read("cache_read_tokens", usage, "cache_read_input_tokens")
		read("cache_write_tokens", usage, "cache_creation_input_tokens")
	default:
		read("input_tokens", usage, "input_tokens", "prompt_tokens")
		read("output_tokens", usage, "output_tokens", "completion_tokens")
		inputDetails := obj(usage["input_tokens_details"])
		if inputDetails == nil {
			inputDetails = obj(usage["prompt_tokens_details"])
		}
		outputDetails := obj(usage["output_tokens_details"])
		if outputDetails == nil {
			outputDetails = obj(usage["completion_tokens_details"])
		}
		read("cache_read_tokens", inputDetails, "cached_tokens")
		read("cache_write_tokens", inputDetails, "cache_write_tokens", "cache_creation_tokens")
		read("cache_read_tokens", usage, "cache_read_input_tokens")
		read("cache_write_tokens", usage, "cache_creation_input_tokens")
		read("reasoning_tokens", outputDetails, "reasoning_tokens")
		if _, present := fields["input_tokens"]; !present && (protocol == domain.ProtocolOpenAIEmbeddings || (len(capability) > 0 && capability[0] == domain.CapabilityRerank)) {
			// Input-only APIs may expose only total_tokens. This is reported
			// input usage, not a request-size estimate.
			read("input_tokens", usage, "total_tokens")
		}
	}
	prev.InvalidFields = invalid
	if len(fields) == 0 {
		return prev
	}
	next := domain.UsageEvidence{Protocol: protocol, Fields: fields, Event: event, Terminal: terminal, InvalidFields: invalid,
		IgnoredZeroTerminal: prev.IgnoredZeroTerminal}
	if protocol == domain.ProtocolOpenAIResponses && terminal {
		if !next.HasTokens() && prev.HasTokens() {
			prev.IgnoredZeroTerminal = true
			return prev
		}
		return next
	}
	merged := make(map[string]int, len(prev.Fields)+len(fields))
	for k, v := range prev.Fields {
		merged[k] = v
	}
	for k, v := range fields {
		// sub2api keeps progressive non-zero Responses reports; a terminal
		// snapshot is authoritative and handled above.
		if protocol != domain.ProtocolOpenAIResponses || v > 0 || merged[k] == 0 {
			merged[k] = v
		}
	}
	next.Fields = merged
	return next
}

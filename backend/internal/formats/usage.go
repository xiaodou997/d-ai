package formats

import (
	"encoding/json"
	"strings"

	"uni-ai-api/backend/internal/domain"
)

// ExtractSyncUsage parses an upstream non-streaming response body and returns
// the usage counters in our domain shape. Each protocol uses a different JSON
// layout, so we have one decoder per protocol family. Unknown fields are
// ignored; absent fields default to 0. The caller can rely on the result being
// non-nil — a zero-value TokenUsage means "upstream did not report usage".
//
// Strict 1:1 routing guarantees that protocol matches the client's request,
// so each branch only fires for upstream bodies in its own native shape.
func ExtractSyncUsage(body []byte, protocol domain.UpstreamProtocol) domain.TokenUsage {
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		return extractAnthropicUsage(body)
	case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
		return extractGeminiUsage(body)
	case domain.ProtocolOpenAIResponses:
		return extractOpenAIResponsesUsage(body)
	default: // openai_chat, openai_completions, openai_embeddings, openai_images
		return extractOpenAIChatUsage(body)
	}
}

// ExtractStreamUsage inspects a single SSE `data: ...` payload (eventType is
// the value of the preceding `event:` line, or "" if absent) and merges any
// usage info into prev. Returns the (possibly updated) usage plus a bool that
// reports whether this line carried any usage data — callers use it to decide
// whether to reset their byte-length estimator.
//
// For protocols that emit usage in multiple chunks (Anthropic: input_tokens in
// message_start, output_tokens in message_delta), prev is merged with the
// latest values. Output / completion counts are taken as *cumulative*, not
// additive — providers already accumulate per chunk.
func ExtractStreamUsage(prev domain.TokenUsage, data []byte, eventType string, protocol domain.UpstreamProtocol) (domain.TokenUsage, bool) {
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		return mergeAnthropicStreamUsage(prev, data, eventType)
	case domain.ProtocolGeminiGenerate:
		return mergeGeminiStreamUsage(prev, data)
	case domain.ProtocolOpenAIResponses:
		return mergeOpenAIResponsesStreamUsage(prev, data, eventType)
	default: // openai_chat / openai_completions
		return mergeOpenAIChatStreamUsage(prev, data)
	}
}

// ============================================================================
// OpenAI Chat Completions (and OpenAI-compatible openai_chat upstreams)
// ============================================================================

type openaiChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// Newer OpenAI fields — surfaced for prompt caching / reasoning billing.
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details,omitempty"`
}

func (u openaiChatUsage) toDomain() domain.TokenUsage {
	out := domain.TokenUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
	}
	if u.PromptTokensDetails != nil {
		out.CacheReadTokens = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		out.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return out
}

func extractOpenAIChatUsage(body []byte) domain.TokenUsage {
	var env struct {
		Usage *openaiChatUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Usage == nil {
		return domain.TokenUsage{}
	}
	return env.Usage.toDomain()
}

func mergeOpenAIChatStreamUsage(prev domain.TokenUsage, data []byte) (domain.TokenUsage, bool) {
	// Chat streaming emits usage only when stream_options.include_usage=true,
	// and only on the terminal chunk (which has empty choices[]). We don't
	// gate on choices; any chunk with a non-nil usage object is accepted.
	var env struct {
		Usage *openaiChatUsage `json:"usage"`
	}
	if err := json.Unmarshal(data, &env); err != nil || env.Usage == nil {
		return prev, false
	}
	return env.Usage.toDomain(), true
}

// ============================================================================
// OpenAI Responses API (/v1/responses + Codex OAuth)
// ============================================================================

type openaiResponsesUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	TotalTokens         int `json:"total_tokens"`
	InputTokensDetails  *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details,omitempty"`
}

func (u openaiResponsesUsage) toDomain() domain.TokenUsage {
	out := domain.TokenUsage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
	}
	if u.InputTokensDetails != nil {
		out.CacheReadTokens = u.InputTokensDetails.CachedTokens
	}
	if u.OutputTokensDetails != nil {
		out.ReasoningTokens = u.OutputTokensDetails.ReasoningTokens
	}
	return out
}

func extractOpenAIResponsesUsage(body []byte) domain.TokenUsage {
	var env struct {
		Usage *openaiResponsesUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Usage == nil {
		return domain.TokenUsage{}
	}
	return env.Usage.toDomain()
}

func mergeOpenAIResponsesStreamUsage(prev domain.TokenUsage, data []byte, eventType string) (domain.TokenUsage, bool) {
	// Responses streaming wraps the terminal response object inside
	// {"type":"response.completed","response":{... ,"usage":{...}}}.
	// Other event types may also carry usage; we accept any payload that has
	// a "usage" or "response.usage" object.
	if eventType != "" && !strings.HasPrefix(eventType, "response.") {
		// Non-response events (e.g. ping) won't have usage.
	}
	var env struct {
		Usage    *openaiResponsesUsage `json:"usage"`
		Response *struct {
			Usage *openaiResponsesUsage `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return prev, false
	}
	switch {
	case env.Usage != nil:
		return env.Usage.toDomain(), true
	case env.Response != nil && env.Response.Usage != nil:
		return env.Response.Usage.toDomain(), true
	}
	return prev, false
}

// ============================================================================
// Anthropic Messages API
// ============================================================================

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u anthropicUsage) toDomain() domain.TokenUsage {
	return domain.TokenUsage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
	}
}

func extractAnthropicUsage(body []byte) domain.TokenUsage {
	var env struct {
		Usage *anthropicUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Usage == nil {
		return domain.TokenUsage{}
	}
	return env.Usage.toDomain()
}

// mergeAnthropicStreamUsage handles the split-stream model: message_start
// carries input/cache tokens (and an initial output_tokens=1 placeholder),
// message_delta carries cumulative output_tokens. We merge incrementally so
// the running state is always correct after every event.
func mergeAnthropicStreamUsage(prev domain.TokenUsage, data []byte, eventType string) (domain.TokenUsage, bool) {
	switch eventType {
	case "message_start":
		var env struct {
			Message struct {
				Usage *anthropicUsage `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal(data, &env); err != nil || env.Message.Usage == nil {
			return prev, false
		}
		u := env.Message.Usage.toDomain()
		// Carry forward whatever output_tokens we'd already accumulated
		// (defensive — message_start should be the first event in practice).
		if prev.CompletionTokens > u.CompletionTokens {
			u.CompletionTokens = prev.CompletionTokens
		}
		return u, true

	case "message_delta":
		var env struct {
			Usage *anthropicUsage `json:"usage"`
		}
		if err := json.Unmarshal(data, &env); err != nil || env.Usage == nil {
			return prev, false
		}
		// Anthropic message_delta usage is cumulative output_tokens; the input
		// counters are immutable post-message_start, so don't overwrite them.
		out := prev
		if env.Usage.OutputTokens > 0 {
			out.CompletionTokens = env.Usage.OutputTokens
		}
		return out, true
	}
	return prev, false
}

// ============================================================================
// Gemini GenerateContent (and CodeAssist / Antigravity wrappers; the caller is
// responsible for stripping the {"response": {...}} envelope before invoking
// these helpers).
// ============================================================================

type geminiUsage struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
}

func (u geminiUsage) toDomain() domain.TokenUsage {
	return domain.TokenUsage{
		PromptTokens:     u.PromptTokenCount,
		CompletionTokens: u.CandidatesTokenCount,
		CacheReadTokens:  u.CachedContentTokenCount,
		ReasoningTokens:  u.ThoughtsTokenCount,
	}
}

func extractGeminiUsage(body []byte) domain.TokenUsage {
	var env struct {
		UsageMetadata *geminiUsage `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.UsageMetadata == nil {
		return domain.TokenUsage{}
	}
	return env.UsageMetadata.toDomain()
}

func mergeGeminiStreamUsage(prev domain.TokenUsage, data []byte) (domain.TokenUsage, bool) {
	// Gemini streams emit usageMetadata on most/all chunks; the final chunk
	// carries the authoritative totals. Take whatever the latest chunk has.
	var env struct {
		UsageMetadata *geminiUsage `json:"usageMetadata"`
	}
	if err := json.Unmarshal(data, &env); err != nil || env.UsageMetadata == nil {
		return prev, false
	}
	return env.UsageMetadata.toDomain(), true
}

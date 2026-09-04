package serving

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestStreamChunkStartsTokenSkipsPreambleAndUsage(t *testing.T) {
	if streamChunkStartsToken(`{"type":"response.created"}`, "", domain.ProtocolOpenAIResponses) {
		t.Fatal("preamble counted as token")
	}
	if streamChunkStartsToken(`{"type":"response.completed","usage":{}}`, "", domain.ProtocolOpenAIResponses) {
		t.Fatal("usage/terminal counted as token")
	}
	if !streamChunkStartsToken(`{"type":"response.output_text.delta","delta":"hi"}`, "", domain.ProtocolOpenAIResponses) {
		t.Fatal("text delta not counted")
	}
}

func TestStreamChunkStartsTokenOpenAIChat(t *testing.T) {
	if streamChunkStartsToken(`{"choices":[],"usage":{"total_tokens":2}}`, "", domain.ProtocolOpenAIChat) {
		t.Fatal("usage-only chat chunk counted")
	}
	if !streamChunkStartsToken(`{"choices":[{"delta":{"content":"hi"}}]}`, "", domain.ProtocolOpenAIChat) {
		t.Fatal("chat content not counted")
	}
}

package serving

import (
	"testing"
	"time"

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

func TestFirstOutputIncludesPriorAttemptsWithoutInflatingUpstreamLatency(t *testing.T) {
	start := time.Now()
	attempt := start.Add(55 * time.Second)
	req := &Request{StartedAt: start, Attempts: []AttemptRecord{{TransportStartedAt: start}, {TransportStartedAt: attempt}}}
	req.MarkFirstOutput(start.Add(66*time.Second), attempt)
	if req.FirstTokenMs != 66000 || req.Attempts[1].FirstOutputMs != 11000 || req.Attempts[0].FirstOutputMs != 0 {
		t.Fatalf("request=%d attempts=%+v", req.FirstTokenMs, req.Attempts)
	}
	req.MarkFirstOutput(start.Add(70*time.Second), attempt)
	if req.FirstTokenMs != 66000 || req.Attempts[1].FirstOutputMs != 11000 {
		t.Fatal("first output overwritten")
	}
}

func TestEmptyStreamEventsAreNotFirstOutput(t *testing.T) {
	for _, tc := range []struct {
		protocol domain.UpstreamProtocol
		body     string
	}{
		{domain.ProtocolOpenAIResponses, `{"type":"response.output_text.delta","delta":""}`},
		{domain.ProtocolAnthropicMessages, `{"type":"content_block_start","content_block":{"type":"text","text":""}}`},
		{domain.ProtocolAnthropicMessages, `{"type":"content_block_delta","delta":{"type":"text_delta","text":""}}`},
	} {
		if streamChunkStartsToken(tc.body, "", tc.protocol) {
			t.Fatalf("empty event counted: %s", tc.body)
		}
	}
}

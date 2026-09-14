package serving

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

func (s *ExecuteStep) sealCompletion(req *Request) error {
	if s.CompletionRecorder == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req.MarkCompleted(time.Now())
	if err := s.CompletionRecorder.Log(ctx, req); err != nil {
		req.ErrorCode = "completion_storage_unavailable"
		req.ErrorMessage = "request completion could not be recorded"
		req.FailedStep = "completion"
		req.HTTPStatus = 503
		return apiError(503, req.ErrorCode, req.ErrorMessage)
	}
	return nil
}

// streamChoiceTracker tracks the whole response, not just the choices present
// in the latest event. Providers may finish candidates in separate SSE frames.
type streamChoiceTracker struct {
	expected int
	finished map[int]bool
}

func newStreamChoiceTracker(req *Request) *streamChoiceTracker {
	t := &streamChoiceTracker{expected: 1, finished: make(map[int]bool)}
	if req.Envelope != nil {
		var body struct {
			N                int `json:"n"`
			GenerationConfig struct {
				Count      int `json:"candidateCount"`
				SnakeCount int `json:"candidate_count"`
			} `json:"generationConfig"`
		}
		if json.Unmarshal(req.Envelope.ClientBody, &body) == nil {
			t.expected = max(1, body.N, body.GenerationConfig.Count, body.GenerationConfig.SnakeCount)
		}
	}
	return t
}
func (t *streamChoiceTracker) observe(data []byte, protocol domain.UpstreamProtocol) {
	var body struct {
		Choices []struct {
			Index  int    `json:"index"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Candidates []struct {
			Index  int    `json:"index"`
			Finish string `json:"finishReason"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &body) != nil {
		return
	}
	if protocol == domain.ProtocolOpenAIChat {
		for _, c := range body.Choices {
			t.finished[c.Index] = t.finished[c.Index] || c.Finish != ""
		}
	}
	if protocol == domain.ProtocolGeminiGenerate {
		for _, c := range body.Candidates {
			t.finished[c.Index] = t.finished[c.Index] || c.Finish != ""
		}
	}
}
func (t *streamChoiceTracker) complete() bool {
	if len(t.finished) < t.expected {
		return false
	}
	for _, done := range t.finished {
		if !done {
			return false
		}
	}
	return true
}
func (t *streamChoiceTracker) incomplete(protocol domain.UpstreamProtocol) bool {
	return (protocol == domain.ProtocolOpenAIChat || protocol == domain.ProtocolGeminiGenerate) &&
		(t.expected > 1 || len(t.finished) > 1) && !t.complete()
}
func providerFinalEvent(protocol domain.UpstreamProtocol, data []byte, event string) bool {
	o := formats.InspectStreamOutcome(data, event)
	if protocol == domain.ProtocolOpenAIChat {
		return bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]"))
	}
	if protocol == domain.ProtocolOpenAIResponses {
		return o.Event != "[DONE]" && o.State == domain.ProviderTerminalCompleted
	}
	if protocol == domain.ProtocolAnthropicMessages {
		return o.Event == "message_stop"
	}
	if protocol == domain.ProtocolGeminiGenerate {
		var body struct {
			Candidates []struct {
				Finish string `json:"finishReason"`
			} `json:"candidates"`
		}
		if json.Unmarshal(data, &body) != nil || len(body.Candidates) == 0 {
			return false
		}
		for _, c := range body.Candidates {
			if c.Finish == "" {
				return false
			}
		}
		return true
	}
	return o.State == domain.ProviderTerminalCompleted
}

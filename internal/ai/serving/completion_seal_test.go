package serving

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xiaodou/dai/internal/ai/domain"
)

type sealRecorderFunc func(context.Context, *Request) error

func (f sealRecorderFunc) Log(ctx context.Context, r *Request) error { return f(ctx, r) }
func TestStreamTerminalPersistsBeforeForwardAndDoesNotWaitForEOF(t *testing.T) {
	req := newStreamReq()
	req.Candidate.Protocol = domain.ProtocolOpenAIResponses
	req.ClientProtocol = domain.ProtocolOpenAIResponses
	dc := genTestDC()
	defer dc.stop()
	w := httptest.NewRecorder()
	step := newExecuteStepForTests()
	called := false
	step.CompletionRecorder = sealRecorderFunc(func(_ context.Context, r *Request) error {
		called = true
		if strings.Contains(w.Body.String(), "response.completed") {
			t.Error("terminal forwarded before durable seal")
		}
		if r.TokenUsage.PromptTokens != 10 {
			t.Errorf("sealed input=%d", r.TokenUsage.PromptTokens)
		}
		return nil
	})
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	resp := sseResp("")
	resp.Body = reader
	go func() {
		_, _ = writer.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"))
	}()
	done := make(chan error, 1)
	go func() { done <- step.executeReportedStream(dc, req, resp, w, time.Now(), false) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("waited for connection close after protocol terminal")
	}
	if !called || !strings.Contains(w.Body.String(), "response.completed") {
		t.Fatal("terminal was not sealed and forwarded")
	}
}
func TestStreamDoesNotForwardTerminalWhenSealFails(t *testing.T) {
	req := newStreamReq()
	req.Candidate.Protocol = domain.ProtocolOpenAIResponses
	req.ClientProtocol = domain.ProtocolOpenAIResponses
	dc := genTestDC()
	defer dc.stop()
	w := httptest.NewRecorder()
	step := newExecuteStepForTests()
	step.CompletionRecorder = sealRecorderFunc(func(context.Context, *Request) error { return errors.New("injected persistence failure") })
	err := step.executeReportedStream(dc, req, sseResp("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":10}}}\n\n"), w, time.Now(), false)
	if err == nil || strings.Contains(w.Body.String(), "response.completed") {
		t.Fatal("completion forwarded without evidence persistence")
	}
	if DecideCompletion(req).Billable {
		t.Fatal("unpersisted completion remained chargeable")
	}
}

func TestConvertedTerminalWaitsForSeal(t *testing.T) {
	req := newStreamReq()
	req.Candidate.Protocol = domain.ProtocolAnthropicMessages
	req.ClientProtocol = domain.ProtocolOpenAIResponses
	dc := genTestDC()
	defer dc.stop()
	w := httptest.NewRecorder()
	step := newExecuteStepForTests()
	step.CompletionRecorder = sealRecorderFunc(func(context.Context, *Request) error {
		if strings.Contains(w.Body.String(), "response.completed") {
			t.Error("BUG: client completion was already emitted before seal")
		}
		return errors.New("injected storage outage")
	})
	body := "data: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"usage\":{\"input_tokens\":10}}}\n\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	_ = step.executeReportedStream(dc, req, sseResp(body), w, time.Now(), true)
	if strings.Contains(w.Body.String(), "response.completed") {
		t.Error("BUG: storage failure still leaves successful client terminal")
	}
}
func TestPartialChoiceEOFWaivesUsage(t *testing.T) {
	req := newStreamReq()
	req.Candidate.Protocol = domain.ProtocolOpenAIChat
	req.ClientProtocol = domain.ProtocolOpenAIChat
	dc := genTestDC()
	defer dc.stop()
	w := httptest.NewRecorder()
	step := newExecuteStepForTests()
	body := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"a\"}},{\"index\":1,\"delta\":{\"content\":\"b\"}}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"finish_reason\":\"stop\",\"delta\":{}}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2}}\n\n"
	_ = step.executeReportedStream(dc, req, sseResp(body), w, time.Now(), false)
	if DecideCompletion(req).Billable {
		t.Errorf("BUG: choice 1 truncated at EOF but billed: terminal=%s", req.ProviderTerminalState)
	}
}

func TestChatChoiceCompletionAcrossFrames(t *testing.T) {
	for _, tc := range []struct {
		name, tail string
		complete   bool
	}{
		{"missing_requested_choice", "", false},
		{"done_with_missing_choice", "data: [DONE]\n\n", false},
		{"both_finished_at_eof", "data: {\"choices\":[{\"index\":1,\"finish_reason\":\"stop\"}]}\n\n", true},
		{"both_finished_then_done", "data: {\"choices\":[{\"index\":1,\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := newStreamReq()
			req.Candidate.Protocol = domain.ProtocolOpenAIChat
			req.ClientProtocol = domain.ProtocolOpenAIChat
			req.Envelope = &RequestEnvelope{ClientBody: []byte(`{"n":2,"stream":true}`)}
			dc := genTestDC()
			defer dc.stop()
			w := httptest.NewRecorder()
			step := newExecuteStepForTests()
			body := "data: {\"choices\":[{\"index\":0,\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2}}\n\n" + tc.tail
			_ = step.executeReportedStream(dc, req, sseResp(body), w, time.Now(), false)
			if DecideCompletion(req).Billable != tc.complete {
				t.Fatalf("wrong completion decision: %+v", DecideCompletion(req))
			}
		})
	}
}

func TestConvertedCompletionReleasedOnlyAfterSeal(t *testing.T) {
	req := newStreamReq()
	req.Candidate.Protocol = domain.ProtocolAnthropicMessages
	req.ClientProtocol = domain.ProtocolOpenAIResponses
	dc := genTestDC()
	defer dc.stop()
	w := httptest.NewRecorder()
	step := newExecuteStepForTests()
	seals := 0
	step.CompletionRecorder = sealRecorderFunc(func(context.Context, *Request) error {
		seals++
		if strings.Contains(w.Body.String(), "response.completed") {
			t.Fatal("terminal preceded seal")
		}
		return nil
	})
	body := "data: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"usage\":{\"input_tokens\":10}}}\n\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	if err := step.executeReportedStream(dc, req, sseResp(body), w, time.Now(), true); err != nil {
		t.Fatal(err)
	}
	if seals != 1 || !strings.Contains(w.Body.String(), "response.completed") {
		t.Fatalf("seals=%d output=%s", seals, w.Body.String())
	}
}

func TestGeminiWaitsForAllRequestedCandidates(t *testing.T) {
	for _, complete := range []bool{false, true} {
		req := newStreamReq()
		req.Candidate.Protocol = domain.ProtocolGeminiGenerate
		req.ClientProtocol = domain.ProtocolGeminiGenerate
		req.Envelope = &RequestEnvelope{ClientBody: []byte(`{"generationConfig":{"candidateCount":2}}`)}
		dc := genTestDC()
		w := httptest.NewRecorder()
		step := newExecuteStepForTests()
		body := "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\",\"content\":{\"parts\":[{\"text\":\"first\"}]}}],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":1}}\n\n"
		if complete {
			body += "data: {\"candidates\":[{\"index\":1,\"finishReason\":\"STOP\",\"content\":{\"parts\":[{\"text\":\"second\"}]}}],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":2}}\n\n"
		}
		_ = step.executeReportedStream(dc, req, sseResp(body), w, time.Now(), false)
		dc.stop()
		if DecideCompletion(req).Billable != complete {
			t.Fatalf("complete=%v decision=%+v", complete, DecideCompletion(req))
		}
		if complete && (!strings.Contains(w.Body.String(), "second") || req.TokenUsage.CompletionTokens != 2) {
			t.Fatalf("stopped after first candidate: output=%s usage=%+v", w.Body.String(), req.TokenUsage)
		}
	}
}

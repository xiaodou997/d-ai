package serving

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xiaodou/dai/internal/ai/domain"
)

func TestReportedUsageControlsRetryAndResetsAttemptState(t *testing.T) {
	for _, metered := range []bool{false, true} {
		failed := `{"type":"response.failed","response":{"status":"failed","error":{"message":"unavailable"}}}`
		if metered {
			failed = `{"type":"response.failed","response":{"status":"failed","error":{"message":"failed"},"usage":{"input_tokens":6,"output_tokens":0}}}`
		}
		transport := &sequenceTransport{responses: []*UpstreamResponse{
			sseResp("event: response.created\ndata: {\"type\":\"response.created\"}\n\nevent: response.failed\ndata: " + failed + "\n\n"),
			sseResp("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n"),
		}}
		w := httptest.NewRecorder()
		req := executeTestRequest(w, []*domain.RouteCandidate{
			{RouteID: "first", EndpointID: "first", ModelCode: "public-model", Protocol: domain.ProtocolOpenAIResponses, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
			{RouteID: "second", EndpointID: "second", ModelCode: "public-model", Protocol: domain.ProtocolOpenAIResponses, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		})
		req.IsStream = true
		req.ClientProtocol = domain.ProtocolOpenAIResponses
		req.Envelope.ClientBody = []byte(`{"model":"public-model","input":"hello","stream":true}`)
		step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, Budget: RetryBudget{MaxAttempts: 2}}
		if err := step.Execute(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if metered {
			if transport.calls != 1 || req.TokenUsage.PromptTokens != 6 || req.RequestStatus != domain.RequestFailed {
				t.Fatalf("metered attempt replayed: calls=%d usage=%+v status=%s", transport.calls, req.TokenUsage, req.RequestStatus)
			}
		} else if transport.calls != 2 || req.TokenUsage.PromptTokens != 3 || req.RequestStatus != domain.RequestSuccess || req.UpstreamErrorCode != "" || req.InternalErrorDetail != "" {
			t.Fatalf("retry state leaked: calls=%d usage=%+v status=%s error=%s", transport.calls, req.TokenUsage, req.RequestStatus, req.InternalErrorDetail)
		}
	}
}

func TestReportedStreamSub2APIFailures(t *testing.T) {
	for _, client := range []domain.UpstreamProtocol{domain.ProtocolOpenAIResponses, domain.ProtocolOpenAIChat, domain.ProtocolAnthropicMessages} {
		for _, tc := range []struct {
			name, body    string
			input, output int
			precommit     bool
		}{
			{"incident", `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"upstream_timeout","message":"timed out"}}}` + "\n\n", 0, 0, true},
			{"top_level_usage", `data: {"type":"response.failed","error":{"code":"content_policy","message":"blocked"},"usage":{"input_tokens":6,"output_tokens":0}}` + "\n\n", 6, 0, false},
			{"error_pair", "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"failed\"}}\n\n" + `event: response.failed` + "\n" + `data: {"type":"response.failed","response":{"status":"failed","error":{"message":"failed"},"usage":{"input_tokens":9,"output_tokens":2}}}` + "\n\ndata: [DONE]\n\n", 9, 2, false},
			{"error_metadata_failure", "event: error\ndata: {\"error\":{\"message\":\"failed\"}}\n\n" + "event: response.in_progress\ndata: {\"response\":{\"status\":\"in_progress\"}}\n\n" + "event: response.failed\ndata: {\"response\":{\"status\":\"failed\",\"usage\":{\"input_tokens\":9,\"output_tokens\":2}}}\n\n", 9, 2, false},
			{"error_then_success", "event: error\ndata: {\"error\":{\"message\":\"failed\"},\"usage\":{\"input_tokens\":9,\"output_tokens\":2}}\n\n" + "event: response.completed\ndata: {\"response\":{\"status\":\"completed\"}}\n\ndata: [DONE]\n\n", 9, 2, false},
			{"metadata_then_failure", `data: {"type":"response.created","response":{"model":"upstream-model"}}` + "\n\n" + `data: {"type":"response.failed","response":{"status":"failed","error":{"message":"failed"}}}` + "\n\n", 0, 0, true},
		} {
			t.Run(string(client)+"/"+tc.name, func(t *testing.T) {
				req := newStreamReq()
				req.CapabilityType = domain.CapabilityChat
				req.Candidate.Protocol = domain.ProtocolOpenAIResponses
				req.ClientProtocol = client
				req.UpstreamBodySize = 5_625_609
				w := httptest.NewRecorder()
				dc := genTestDC()
				defer dc.stop()
				err := newExecuteStepForTests().executeReportedStream(dc, req, sseResp(tc.body), w, time.Now(), client != domain.ProtocolOpenAIResponses)
				var pre *precommitError
				if errors.As(err, &pre) != tc.precommit {
					t.Fatalf("err=%v precommit=%v", err, tc.precommit)
				}
				if req.TokenUsage.PromptTokens != tc.input || req.TokenUsage.CompletionTokens != tc.output {
					t.Fatalf("tokens=%+v", req.TokenUsage)
				}
				if req.RequestStatus != domain.RequestFailed || req.ProviderTerminalState != domain.ProviderTerminalFailed {
					t.Fatalf("status=%s/%s", req.RequestStatus, req.ProviderTerminalState)
				}
				if tc.precommit && (req.ResponseCommitted || w.Body.Len() != 0) {
					t.Fatalf("preamble/error committed: %s", w.Body.String())
				}
				if strings.Contains(w.Body.String(), "response.completed") || strings.Contains(w.Body.String(), "[DONE]") || strings.Contains(w.Body.String(), "message_stop") {
					t.Fatalf("false success: %s", w.Body.String())
				}
				if req.FirstTokenMs != 0 {
					t.Fatalf("error counted as first token: %d", req.FirstTokenMs)
				}
				if ShouldVoidBilling(req) != tc.precommit {
					t.Fatalf("wrong chargeability: %+v", req.UsageEvidence)
				}
			})
		}
	}
}

func TestReportedStreamEOFAndInterruptedUsage(t *testing.T) {
	for _, convert := range []bool{false, true} {
		req := newStreamReq()
		req.Candidate.Protocol = domain.ProtocolOpenAIResponses
		if !convert {
			req.ClientProtocol = domain.ProtocolOpenAIResponses
		}
		body := "event: response.output_text.delta\r\ndata:{\"type\":\"response.output_text.delta\",\"delta\":\"hello\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}\r\n\r\ndata: [DONE]\n\n"
		w := httptest.NewRecorder()
		dc := genTestDC()
		err := newExecuteStepForTests().executeReportedStream(dc, req, sseResp(body), w, time.Now(), convert)
		dc.stop()
		if err != nil {
			t.Fatal(err)
		}
		if req.RequestStatus != domain.RequestFailed || req.ProviderTerminalState != domain.ProviderTerminalIncomplete || ShouldVoidBilling(req) {
			t.Fatalf("EOF treated as success/void: %+v", req)
		}
		if req.TokenUsage.PromptTokens != 10 || req.TokenUsage.CompletionTokens != 2 {
			t.Fatalf("lost progressive usage: %+v", req.TokenUsage)
		}
		if strings.Contains(w.Body.String(), "[DONE]") || strings.Contains(w.Body.String(), "response.completed") {
			t.Fatalf("EOF emitted false success: %s", w.Body.String())
		}
	}
}

func TestTextEstimatesNeverReachSettlement(t *testing.T) {
	for _, capability := range []domain.CapabilityType{domain.CapabilityChat, domain.CapabilityEmbedding, domain.CapabilityRerank} {
		req := &Request{CapabilityType: capability, UpstreamBodySize: 5_625_609, TokenUsage: domain.TokenUsage{PromptTokens: 1_875_203, CompletionTokens: 99}}
		fillEstimatedUsage(req, 297)
		if req.TokenUsage.TotalTokens() != 0 || req.TokenCountSource != domain.TokenUsageSourceMissing || !ShouldVoidBilling(req) {
			t.Fatalf("estimate survived: %+v", req)
		}
		req.UsageEvidence = domain.UsageEvidence{Fields: map[string]int{"input_tokens": 0, "output_tokens": 0}}
		ApplyReportedUsage(req)
		if ShouldVoidBilling(req) || req.TokenCountSource != domain.TokenUsageSourceUpstream {
			t.Fatal("explicit zero is not missing")
		}
		req.UsageEvidence.Fields["output_tokens"] = 2
		markClientCancellation(req)
		ApplyReportedUsage(req)
		if ShouldVoidBilling(req) || req.TokenUsage.CompletionTokens != 2 {
			t.Fatal("client cancellation erased reported usage")
		}
	}
}

func TestReportedStreamConvertsEventOnlyTypes(t *testing.T) {
	for _, client := range []domain.UpstreamProtocol{domain.ProtocolOpenAIChat, domain.ProtocolAnthropicMessages} {
		t.Run(string(client), func(t *testing.T) {
			req := newStreamReq()
			req.Candidate.Protocol = domain.ProtocolOpenAIResponses
			req.ClientProtocol = client
			body := "event: response.output_text.delta\ndata: {\"delta\":\"event-only answer\"}\n\nevent: response.completed\ndata: {\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"
			w := httptest.NewRecorder()
			dc := genTestDC()
			defer dc.stop()
			if err := newExecuteStepForTests().executeReportedStream(dc, req, sseResp(body), w, time.Now(), true); err != nil {
				t.Fatal(err)
			}
			if req.RequestStatus != domain.RequestSuccess || !strings.Contains(w.Body.String(), "event-only answer") || req.TokenUsage.PromptTokens != 3 || req.TokenUsage.CompletionTokens != 2 {
				t.Fatalf("event-only conversion lost content or usage: status=%s usage=%+v body=%s", req.RequestStatus, req.TokenUsage, w.Body.String())
			}
		})
	}
}

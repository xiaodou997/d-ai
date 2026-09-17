package serving

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/upstreamcompat"
)

func probeResponse() *UpstreamResponse {
	h := http.Header{}
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Sub2api-Probe-Blocked", "true")
	return &UpstreamResponse{StatusCode: 200, Headers: h, Body: io.NopCloser(strings.NewReader("Hi! What can I help you with?"))}
}

func TestProbeRejectionsDoNotCoolHealthyAccount(t *testing.T) {
	a, _ := servingAvailability(t)
	tr := &sequenceTransport{}
	for i := 0; i < 6; i++ {
		tr.responses = append(tr.responses, probeResponse())
	}
	step := &ExecuteStep{Availability: a, Transport: tr, Bridge: testProtocolBridge{}}
	c := runtimeCandidate("endpoint")
	for i := 0; i < 6; i++ {
		w := httptest.NewRecorder()
		req := executeTestRequest(w, []*domain.RouteCandidate{c})
		req.IsStream = i%2 == 0
		err := step.Execute(context.Background(), req)
		var api *APIError
		if !errors.As(err, &api) || api.Status != 502 || api.Code != upstreamcompat.ProbeBlockedCode {
			t.Fatalf("error=%v", err)
		}
		if req.ResponseCommitted || w.Body.Len() != 0 || req.UsageEvidence.HasTokens() {
			t.Fatalf("refusal was relayed or metered")
		}
		if len(req.Attempts) != 1 || req.Attempts[0].Outcome != ResultPolicyRejected || req.Attempts[0].UpstreamErrorCode != upstreamcompat.ProbeBlockedCode {
			t.Fatalf("attempts=%+v", req.Attempts)
		}
	}
	if tr.calls != 6 {
		t.Fatalf("calls=%d", tr.calls)
	}
	states, err := a.List(context.Background(), "direct_upstream", "account")
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Phase != routing.Available {
			t.Fatalf("state=%+v", state)
		}
	}
}

func TestProbeRejectionFailsOverToUsableEndpoint(t *testing.T) {
	first, second := runtimeCandidate("one"), runtimeCandidate("two")
	tr := &sequenceTransport{responses: []*UpstreamResponse{probeResponse(), jsonResp(`{"choices":[{"message":{"content":"ok"}}]}`)}}
	w := httptest.NewRecorder()
	req := executeTestRequest(w, []*domain.RouteCandidate{first, second})
	if err := (&ExecuteStep{Transport: tr, Bridge: testProtocolBridge{}}).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if tr.calls != 2 || req.RequestStatus != domain.RequestSuccess || strings.Contains(w.Body.String(), "Hi!") {
		t.Fatalf("calls=%d status=%s body=%s", tr.calls, req.RequestStatus, w.Body.String())
	}
}

func TestProbeClassificationRequiresExplicitSuccessfulResponseSignal(t *testing.T) {
	for _, tc := range []struct {
		status int
		flag   string
		want   ResultStatus
	}{{200, "true", ResultPolicyRejected}, {200, "false", ResultSuccess}, {200, "", ResultSuccess}, {401, "true", ResultUnauthorized}, {503, "true", ResultServerError}} {
		h := http.Header{}
		h.Set("X-Sub2api-Probe-Blocked", tc.flag)
		out := ClassifyResponse(tc.status, h, "Hi! What can I help you with?", nil)
		if out.Status != tc.want {
			t.Fatalf("%+v => %+v", tc, out)
		}
		if out.Status == ResultPolicyRejected && (out.CountsAsHealthFailure() || out.Decision(false) != DecisionRetry) {
			t.Fatalf("outcome=%+v", out)
		}
	}
}

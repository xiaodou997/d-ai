package serving

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

type stalledImageBody struct {
	ctx  context.Context
	sent bool
}

func (r *stalledImageBody) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, "data: {\"type\":\"image_generation.partial_image\"}\n\n"), nil
	}
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (r *stalledImageBody) Close() error { return nil }

func imageRelayReq(clientStream bool) *Request {
	return &Request{
		CapabilityType: domain.CapabilityImage,
		ClientProtocol: domain.ProtocolOpenAIImages,
		IsStream:       clientStream,
		ModelCode:      "img-model",
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolOpenAIImages,
			RouteID:       "r1",
			UpstreamModel: "img-model",
		},
		Attempts: []AttemptRecord{{RouteID: "r1"}},
	}
}

// P1-A 客户端侧解耦：上游返回同步 JSON，但客户端要流 → 网关合成单帧 SSE 回吐。
func TestImageRelaySyncUpstreamToStreamClient(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := imageRelayReq(true) // 客户端要流

	err := newExecuteStepForTests().executeImageRelay(dc, req, jsonResp(`{"data":[{"b64_json":"aGk="}]}`), w, time.Now())
	if err != nil {
		t.Fatalf("executeImageRelay err = %v", err)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream (client wants stream)", ct)
	}
	if !strings.HasPrefix(w.Body.String(), "data: ") {
		t.Fatalf("client output is not SSE: %q", w.Body.String())
	}
	if req.RequestStatus != domain.RequestSuccess {
		t.Fatalf("RequestStatus = %v, want success", req.RequestStatus)
	}
}

// 上游返回同步 JSON，客户端要同步 → 直接回 JSON。
func TestImageRelaySyncUpstreamToSyncClient(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := imageRelayReq(false)

	err := newExecuteStepForTests().executeImageRelay(dc, req, jsonResp(`{"data":[{"b64_json":"aGk="}]}`), w, time.Now())
	if err != nil {
		t.Fatalf("executeImageRelay err = %v", err)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(w.Body.String(), "b64_json") {
		t.Fatalf("client JSON not forwarded: %q", w.Body.String())
	}
}

// 上游以 SSE 流式返回，但客户端要同步 → 网关聚合为 JSON 回吐。
func TestImageRelayStreamUpstreamToSyncClient(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := imageRelayReq(false) // 客户端要同步

	sse := "data: {\"data\":[{\"b64_json\":\"aGk=\"}]}\n\n"
	err := newExecuteStepForTests().executeImageRelay(dc, req, jsonResp(sse), w, time.Now())
	if err != nil {
		t.Fatalf("executeImageRelay err = %v", err)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json (client wants sync)", ct)
	}
}

func TestImageRelayResetsAndEnforcesUpstreamIdleTimeout(t *testing.T) {
	w := httptest.NewRecorder()
	dc := newDeadlineController(context.Background(), domain.RouteTimeouts{
		ResponseHeader: time.Hour,
		FirstByte:      time.Hour,
		Idle:           20 * time.Millisecond,
		MaxDuration:    time.Hour,
	})
	defer dc.stop()
	dc.headersReceived()
	req := imageRelayReq(false)
	resp := &UpstreamResponse{StatusCode: 200, Body: &stalledImageBody{ctx: dc.ctx}}

	err := newExecuteStepForTests().executeImageRelay(dc, req, resp, w, time.Now())
	if !isPrecommitError(err) || dc.cause() != ErrIdleTimeout {
		t.Fatalf("stalled image relay error = %v, cause = %v; want idle timeout", err, dc.cause())
	}
}

// 200 且 body 是错误对象 → precommit（可 failover），不提交客户端。
func TestImageRelayErrorBodyPrecommit(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := imageRelayReq(false)

	err := newExecuteStepForTests().executeImageRelay(dc, req, jsonResp(`{"error":{"message":"boom"}}`), w, time.Now())
	if !isPrecommitError(err) {
		t.Fatalf("200+error body: err = %v, want *precommitError", err)
	}
	if req.HTTPStatus != 0 {
		t.Fatalf("HTTPStatus must stay 0 (uncommitted), got %d", req.HTTPStatus)
	}
}

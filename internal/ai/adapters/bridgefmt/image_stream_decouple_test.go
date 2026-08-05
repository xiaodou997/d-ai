package bridgefmt

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

// imageReq 构造一个生图 serving.Request。clientStream 是客户端意愿，
// streamMode 是 binding 的上游侧策略。
func imageReq(clientProto, providerProto domain.UpstreamProtocol, clientStream bool, streamMode string) *serving.Request {
	httpReq := httptest.NewRequest("POST", "/v1/images/generations", nil)
	httpReq.Header.Set("Content-Type", "application/json")
	return &serving.Request{
		CapabilityType: domain.CapabilityImage,
		ClientProtocol: clientProto,
		IsStream:       clientStream,
		ClientPath:     "/v1/images/generations",
		Candidate: &domain.RouteCandidate{
			BaseURL:          "https://upstream.test",
			Protocol:         providerProto,
			UpstreamModel:    "img-model",
			APIKeyCiphertext: "sk-abc",
			ImageStreamMode:  streamMode,
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}
}

// P1-A：binding 的 force_sync/force_stream 决定"我们如何请求上游"，
// 与客户端 stream 意愿解耦。

func TestForceSyncMakesOpenAIImageUpstreamSyncDespiteClientStream(t *testing.T) {
	rt := NewRuntime()
	// 客户端要流，但 binding force_sync → 上游 body 必须 stream=false。
	req := imageReq(domain.ProtocolOpenAIImages, domain.ProtocolOpenAIImages, true, domain.ImageStreamModeForceSync)
	prepared, err := rt.PrepareRequest(req, []byte(`{"model":"img-model","prompt":"a cat","stream":true}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(prepared.Body, &doc); err != nil {
		t.Fatalf("unmarshal prepared body: %v", err)
	}
	if stream, _ := doc["stream"].(bool); stream {
		t.Fatalf("force_sync: upstream body stream = true, want false (body=%s)", prepared.Body)
	}
}

func TestForceStreamMakesOpenAIImageUpstreamStreamDespiteClientSync(t *testing.T) {
	rt := NewRuntime()
	// 客户端要同步，但 binding force_stream → 上游 body 必须 stream=true。
	req := imageReq(domain.ProtocolOpenAIImages, domain.ProtocolOpenAIImages, false, domain.ImageStreamModeForceStream)
	prepared, err := rt.PrepareRequest(req, []byte(`{"model":"img-model","prompt":"a cat","stream":false}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(prepared.Body, &doc); err != nil {
		t.Fatalf("unmarshal prepared body: %v", err)
	}
	if stream, _ := doc["stream"].(bool); !stream {
		t.Fatalf("force_stream: upstream body stream = false, want true (body=%s)", prepared.Body)
	}
}

func TestForceSyncMakesGeminiImageUpstreamSyncDespiteClientStream(t *testing.T) {
	rt := NewRuntime()
	// 客户端 openai_images 要流，provider 是 gemini，binding force_sync →
	// Gemini 走同步 generateContent：Accept 头不应是 event-stream。
	req := imageReq(domain.ProtocolOpenAIImages, domain.ProtocolGeminiGenerate, true, domain.ImageStreamModeForceSync)
	prepared, err := rt.PrepareRequest(req, []byte(`{"model":"img-model","prompt":"a cat"}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	upReq, err := rt.BuildUpstreamRequest(req, prepared)
	if err != nil {
		t.Fatalf("BuildUpstreamRequest err = %v", err)
	}
	if upReq.Headers["Accept"] == "text/event-stream" {
		t.Fatalf("force_sync gemini: Accept = event-stream, want sync")
	}
}

func TestForceStreamMakesGeminiImageUpstreamStreamDespiteClientSync(t *testing.T) {
	rt := NewRuntime()
	req := imageReq(domain.ProtocolOpenAIImages, domain.ProtocolGeminiGenerate, false, domain.ImageStreamModeForceStream)
	prepared, err := rt.PrepareRequest(req, []byte(`{"model":"img-model","prompt":"a cat"}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	upReq, err := rt.BuildUpstreamRequest(req, prepared)
	if err != nil {
		t.Fatalf("BuildUpstreamRequest err = %v", err)
	}
	if upReq.Headers["Accept"] != "text/event-stream" {
		t.Fatalf("force_stream gemini: Accept = %q, want text/event-stream", upReq.Headers["Accept"])
	}
}

// auto 模式仍跟随客户端。
func TestAutoStreamFollowsClientForOpenAIImage(t *testing.T) {
	rt := NewRuntime()
	req := imageReq(domain.ProtocolOpenAIImages, domain.ProtocolOpenAIImages, false, domain.ImageStreamModeAuto)
	prepared, err := rt.PrepareRequest(req, []byte(`{"model":"img-model","prompt":"a cat","stream":true}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	var doc map[string]any
	_ = json.Unmarshal(prepared.Body, &doc)
	if stream, _ := doc["stream"].(bool); stream {
		t.Fatalf("auto+client sync: upstream stream = true, want false")
	}
}

func TestClientImageResponseFormatIsNotSentToUpstream(t *testing.T) {
	rt := NewRuntime()
	req := imageReq(domain.ProtocolOpenAIImages, domain.ProtocolOpenAIImages, false, domain.ImageStreamModeAuto)
	prepared, err := rt.PrepareRequest(req, []byte(`{"model":"img-model","prompt":"a cat","response_format":"url"}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	var doc map[string]any
	_ = json.Unmarshal(prepared.Body, &doc)
	if _, exists := doc["response_format"]; exists {
		t.Fatalf("response_format must not be sent upstream (body=%s)", prepared.Body)
	}
	if req.ImageClientResponseFormat != domain.ImageResponseFormatURL {
		t.Fatalf("client response format = %q, want url", req.ImageClientResponseFormat)
	}
}

var _ = corebridge.PreparedRequest{}

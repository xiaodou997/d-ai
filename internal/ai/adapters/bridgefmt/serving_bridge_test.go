package bridgefmt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

func TestBuildUpstreamRequest(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		IsStream:   true,
		ClientPath: "/v1/chat/completions",
		Candidate: &domain.RouteCandidate{
			BaseURL:          "https://api.openai.com",
			Protocol:         domain.ProtocolOpenAIChat,
			APIKeyCiphertext: "sk-abc",
		},
	}

	upReq, err := runtime.BuildUpstreamRequest(req, corebridge.PreparedRequest{Body: []byte(`{"model":"gpt-5"}`)})
	if err != nil {
		t.Fatalf("BuildUpstreamRequest err = %v", err)
	}
	if upReq.Method != "POST" {
		t.Fatalf("BuildUpstreamRequest method = %q, want POST", upReq.Method)
	}
	if upReq.URL != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("BuildUpstreamRequest url = %q", upReq.URL)
	}
	if upReq.Headers["Authorization"] != "Bearer sk-abc" {
		t.Fatalf("Authorization = %q", upReq.Headers["Authorization"])
	}
	if upReq.Headers["Accept"] != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", upReq.Headers["Accept"])
	}
}

func TestBuildUpstreamRequestForwardsAPIUserAgent(t *testing.T) {
	runtime := NewRuntime()
	httpReq := httptest.NewRequest("POST", "/v1/responses", nil)
	httpReq.Header.Set("User-Agent", "curl/8.7.1")
	req := &serving.Request{
		ClientPath: "/v1/responses",
		Candidate: &domain.RouteCandidate{
			BaseURL:          "https://api.openai.com",
			Protocol:         domain.ProtocolOpenAIResponses,
			APIKeyCiphertext: "sk-abc",
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	upReq, err := runtime.BuildUpstreamRequest(req, corebridge.PreparedRequest{Body: []byte(`{"model":"gpt-5"}`)})
	if err != nil {
		t.Fatalf("BuildUpstreamRequest err = %v", err)
	}
	if got := upReq.Headers["user-agent"]; got != "curl/8.7.1" {
		t.Fatalf("user-agent = %q, want curl/8.7.1", got)
	}
}

func TestBuildUpstreamRequestUsesWebUserAgentForWebRuntimeSubject(t *testing.T) {
	runtime := NewRuntime()
	httpReq := httptest.NewRequest("POST", "/runtime/v1/images/tasks", nil)
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")
	req := &serving.Request{
		ClientPath: "/v1/images/generations",
		Subject: &coreidentity.Subject{
			RequestSource: coreidentity.RequestSourceWebImage,
		},
		Candidate: &domain.RouteCandidate{
			BaseURL:          "https://api.openai.com",
			Protocol:         domain.ProtocolOpenAIImages,
			APIKeyCiphertext: "sk-abc",
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	upReq, err := runtime.BuildUpstreamRequest(req, corebridge.PreparedRequest{Body: []byte(`{"model":"gpt-image-1"}`)})
	if err != nil {
		t.Fatalf("BuildUpstreamRequest err = %v", err)
	}
	if got := upReq.Headers["user-agent"]; got != webOutboundUserAgent {
		t.Fatalf("user-agent = %q, want %q", got, webOutboundUserAgent)
	}
}

func TestBuildUpstreamRequestUsesAppUserAgentForAppPreview(t *testing.T) {
	runtime := NewRuntime()
	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")
	req := &serving.Request{
		ClientPath: "/v1/chat/completions",
		Subject: &coreidentity.Subject{
			RequestSource: coreidentity.RequestSourceAppPreview,
		},
		Candidate: &domain.RouteCandidate{
			BaseURL:          "https://api.openai.com",
			Protocol:         domain.ProtocolOpenAIChat,
			APIKeyCiphertext: "sk-abc",
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	upReq, err := runtime.BuildUpstreamRequest(req, corebridge.PreparedRequest{Body: []byte(`{"model":"gpt-5"}`)})
	if err != nil {
		t.Fatalf("BuildUpstreamRequest err = %v", err)
	}
	if got := upReq.Headers["user-agent"]; got != appOutboundUserAgent {
		t.Fatalf("user-agent = %q, want %q", got, appOutboundUserAgent)
	}
}

func TestPrepareRequestConvertsOpenAIImageEditJSONToMultipart(t *testing.T) {
	runtime := NewRuntime()
	body := []byte(`{"model":"public-image","prompt":"touch up","images":[{"image_url":"data:image/png;base64,` + bridgeTestImagePNGBase64 + `"}],"stream":true,"response_format":"url"}`)
	httpReq := httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		ClientPath:     "/v1/images/edits",
		Envelope: &serving.RequestEnvelope{
			R:          httpReq,
			ClientBody: body,
		},
		Candidate: &domain.RouteCandidate{
			Protocol:           domain.ProtocolOpenAIImages,
			UpstreamModel:      "upstream-image",
			ImageEditTransport: domain.ImageEditTransportMultipart,
			ImageStreamMode:    domain.ImageStreamModeForceStream,
		},
	}

	prepared, err := runtime.PrepareRequest(req, body)
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(prepared.ContentType)
	if err != nil {
		t.Fatalf("parse content type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, want multipart/form-data", mediaType)
	}
	fields, err := multipartScalarFieldsForTest(prepared.Body, params["boundary"])
	if err != nil {
		t.Fatalf("read multipart fields: %v", err)
	}
	if fields["model"] != "upstream-image" {
		t.Fatalf("model = %q, want upstream-image", fields["model"])
	}
	if _, exists := fields["image_url"]; exists {
		t.Fatalf("multipart body must not retain image_url: %#v", fields)
	}
	files, err := multipartFilesForTest(prepared.Body, params["boundary"])
	if err != nil {
		t.Fatalf("read multipart files: %v", err)
	}
	if !bytes.Equal(files["image[]"], mustBridgeTestImagePNG(t)) {
		t.Fatalf("image file does not match input PNG")
	}
	if fields["stream"] != "true" {
		t.Fatalf("stream = %q, want true", fields["stream"])
	}
	if req.ImageClientResponseFormat != domain.ImageResponseFormatURL {
		t.Fatalf("client response format = %q, want url", req.ImageClientResponseFormat)
	}
	if _, exists := fields["response_format"]; exists {
		t.Fatalf("response_format must not be sent upstream: %#v", fields)
	}
}

func TestPrepareRequestKeepsOfficialOpenAIImageEditJSONShape(t *testing.T) {
	runtime := NewRuntime()
	body := []byte(`{"model":"public-image","prompt":"touch up","images":[{"image_url":"https://images.example.test/input.png"}]}`)
	httpReq := httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		ClientPath:     "/v1/images/edits",
		Envelope: &serving.RequestEnvelope{
			R:          httpReq,
			ClientBody: body,
		},
		Candidate: &domain.RouteCandidate{
			Protocol:           domain.ProtocolOpenAIImages,
			UpstreamModel:      "upstream-image",
			ImageEditTransport: domain.ImageEditTransportJSON,
		},
	}

	prepared, err := runtime.PrepareRequest(req, body)
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	if prepared.ContentType != "application/json" {
		t.Fatalf("content type override = %q, want application/json", prepared.ContentType)
	}
	var payload map[string]any
	if err := json.Unmarshal(prepared.Body, &payload); err != nil {
		t.Fatalf("unmarshal prepared body: %v", err)
	}
	if payload["model"] != "upstream-image" {
		t.Fatalf("model = %#v, want upstream-image", payload["model"])
	}
	images, ok := payload["images"].([]any)
	if !ok || len(images) != 1 || images[0].(map[string]any)["image_url"] != "https://images.example.test/input.png" {
		t.Fatalf("images = %#v", payload["images"])
	}
}

func TestPrepareRequestReplacesClientImageResponseFormatWithBindingValue(t *testing.T) {
	runtime := NewRuntime()
	body := []byte(`{"model":"public-image","prompt":"a cat","response_format":"url"}`)
	httpReq := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		ClientPath:     "/v1/images/generations",
		Envelope:       &serving.RequestEnvelope{R: httpReq, ClientBody: body},
		Candidate: &domain.RouteCandidate{
			Protocol:                    domain.ProtocolOpenAIImages,
			UpstreamModel:               "upstream-image",
			ImageUpstreamResponseFormat: domain.ImageResponseFormatB64,
		},
	}

	prepared, err := runtime.PrepareRequest(req, body)
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	if req.ImageClientResponseFormat != domain.ImageResponseFormatURL {
		t.Fatalf("client response format = %q, want url", req.ImageClientResponseFormat)
	}
	var payload map[string]any
	if err := json.Unmarshal(prepared.Body, &payload); err != nil {
		t.Fatalf("unmarshal prepared body: %v", err)
	}
	if got := payload["response_format"]; got != domain.ImageResponseFormatB64 {
		t.Fatalf("response_format = %#v, want binding value %q", got, domain.ImageResponseFormatB64)
	}
}

func TestPrepareRequestReplacesMultipartClientResponseFormatWithBindingValue(t *testing.T) {
	runtime := NewRuntime()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "public-image"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("prompt", "touch up"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("response_format", "url"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image[]", "image.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(mustBridgeTestImagePNG(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	httpReq := httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body.Bytes()))
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		ClientPath:     "/v1/images/edits",
		Envelope:       &serving.RequestEnvelope{R: httpReq, ClientBody: body.Bytes()},
		Candidate: &domain.RouteCandidate{
			Protocol:                    domain.ProtocolOpenAIImages,
			UpstreamModel:               "upstream-image",
			ImageEditTransport:          domain.ImageEditTransportMultipart,
			ImageStreamMode:             domain.ImageStreamModeForceSync,
			ImageUpstreamResponseFormat: domain.ImageResponseFormatB64,
		},
	}

	prepared, err := runtime.PrepareRequest(req, body.Bytes())
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	if req.ImageClientResponseFormat != domain.ImageResponseFormatURL {
		t.Fatalf("client response format = %q, want url", req.ImageClientResponseFormat)
	}
	_, params, err := mime.ParseMediaType(prepared.ContentType)
	if err != nil {
		t.Fatalf("parse content type: %v", err)
	}
	fields, err := multipartScalarFieldsForTest(prepared.Body, params["boundary"])
	if err != nil {
		t.Fatalf("read multipart fields: %v", err)
	}
	if got := fields["response_format"]; got != domain.ImageResponseFormatB64 {
		t.Fatalf("response_format = %q, want binding value %q", got, domain.ImageResponseFormatB64)
	}
	if fields["stream"] != "false" {
		t.Fatalf("upstream stream = %q, want false", fields["stream"])
	}
}

func TestPrepareRequestConvertsMultipartImageEditToOfficialJSON(t *testing.T) {
	runtime := NewRuntime()
	body, contentType := mustMultipartImageEditBody(t, "touch up")
	httpReq := httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", contentType)
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		ClientPath:     "/v1/images/edits",
		Envelope:       &serving.RequestEnvelope{R: httpReq, ClientBody: body},
		Candidate: &domain.RouteCandidate{
			Protocol:           domain.ProtocolOpenAIImages,
			UpstreamModel:      "upstream-image",
			ImageEditTransport: domain.ImageEditTransportJSON,
			ImageStreamMode:    domain.ImageStreamModeForceSync,
		},
	}

	prepared, err := runtime.PrepareRequest(req, body)
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	if prepared.ContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", prepared.ContentType)
	}
	var payload map[string]any
	if err := json.Unmarshal(prepared.Body, &payload); err != nil {
		t.Fatalf("unmarshal prepared body: %v", err)
	}
	images, ok := payload["images"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("images = %#v", payload["images"])
	}
	wantImageURL := "data:image/png;base64," + bridgeTestImagePNGBase64
	if got := images[0].(map[string]any)["image_url"]; got != wantImageURL {
		t.Fatalf("image_url = %#v, want fixed PNG data URL", got)
	}
	if _, exists := payload["response_format"]; exists {
		t.Fatalf("response_format must not be sent upstream: %#v", payload)
	}
	if got := payload["stream"]; got != false {
		t.Fatalf("stream = %#v, want false", got)
	}
}

func TestNormalizeResponseBodyGeminiCLI(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		Candidate: &domain.RouteCandidate{
			FixedProviderType: domain.FixedProviderGeminiCLI,
		},
	}
	body := []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}}`)

	got := runtime.NormalizeResponseBody(req, body)
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("NormalizeResponseBody returned invalid json: %v", err)
	}
	if _, ok := decoded["response"]; ok {
		t.Fatalf("NormalizeResponseBody should unwrap CodeAssist envelope, got %s", string(got))
	}
}

func TestPrepareRequestLeavesGeminiCLIEnvelopeToClientRuntime(t *testing.T) {
	runtime := NewRuntime()
	httpReq := httptest.NewRequest("POST", "/v1beta/models/gemini-2.0-flash:generateContent", nil)
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolGeminiGenerate,
		Candidate: &domain.RouteCandidate{
			Protocol:          domain.ProtocolGeminiGenerate,
			FixedProviderType: domain.FixedProviderGeminiCLI,
			UpstreamModel:     "gemini-2.0-flash",
		},
		SelectedCredential: &domain.OAuthCredential{
			AuthMetadata: map[string]any{"project_id": "proj-1"},
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	plan, err := runtime.PrepareRequest(req, []byte(`{"model":"logical-model","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}

	var semantic map[string]any
	if err := json.Unmarshal(plan.Body, &semantic); err != nil {
		t.Fatalf("PrepareRequest returned invalid json: %v", err)
	}
	if semantic["model"] != "gemini-2.0-flash" {
		t.Fatalf("semantic model = %#v", semantic["model"])
	}
	if _, wrapped := semantic["request"]; wrapped {
		t.Fatalf("PrepareRequest must not apply a provider envelope: %s", string(plan.Body))
	}

}

func TestPrepareRequestBridgesOpenAIImagesToGemini(t *testing.T) {
	runtime := NewRuntime()
	body, contentType := mustMultipartImageEditBody(t, "draw a kite")
	httpReq := httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", contentType)
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		ClientPath:     "/v1/images/edits",
		CapabilityType: domain.CapabilityImage,
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiGenerate,
			UpstreamModel: "imagen-4",
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	plan, err := runtime.PrepareRequest(req, body)
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	if plan.ContentType != "application/json" {
		t.Fatalf("ContentType = %q", plan.ContentType)
	}
	if plan.RequestPath != "" {
		t.Fatalf("RequestPath = %q", plan.RequestPath)
	}
	var geminiReq map[string]any
	if err := json.Unmarshal(plan.Body, &geminiReq); err != nil {
		t.Fatalf("rewritten body invalid json: %v", err)
	}
	if geminiReq["model"] != "imagen-4" {
		t.Fatalf("model = %#v", geminiReq["model"])
	}
	if _, ok := geminiReq["generationConfig"].(map[string]any); !ok {
		t.Fatalf("missing generationConfig: %s", string(plan.Body))
	}
}

func TestPrepareRequestBridgesOpenAIImageCountToGemini(t *testing.T) {
	runtime := NewRuntime()
	body := []byte(`{"model":"gpt-image-1","prompt":"draw a kite","n":3}`)
	httpReq := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		ClientPath:     "/v1/images/generations",
		CapabilityType: domain.CapabilityImage,
		Candidate:      &domain.RouteCandidate{Protocol: domain.ProtocolGeminiGenerate, UpstreamModel: "imagen-4"},
		Envelope:       &serving.RequestEnvelope{R: httpReq},
	}
	plan, err := runtime.PrepareRequest(req, body)
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	var payload struct {
		GenerationConfig struct {
			CandidateCount int `json:"candidateCount"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(plan.Body, &payload); err != nil || payload.GenerationConfig.CandidateCount != 3 {
		t.Fatalf("Gemini candidateCount = %d, err = %v, body = %s", payload.GenerationConfig.CandidateCount, err, plan.Body)
	}
}

func TestPrepareRequestBridgesGeminiImagesToOpenAI(t *testing.T) {
	runtime := NewRuntime()
	body := []byte(`{"generationConfig":{"responseModalities":["TEXT","IMAGE"],"candidateCount":3},"contents":[{"role":"user","parts":[{"text":"draw a cat"},{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}]}`)
	httpReq := httptest.NewRequest("POST", "/v1beta/models/imagen-4:generateContent", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolGeminiGenerate,
		ClientPath:     "/v1beta/models/imagen-4:generateContent",
		CapabilityType: domain.CapabilityImage,
		Candidate: &domain.RouteCandidate{
			Protocol:           domain.ProtocolOpenAIImages,
			UpstreamModel:      "gpt-image-1",
			BaseURL:            "https://api.openai.com",
			ImageEditTransport: domain.ImageEditTransportMultipart,
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	plan, err := runtime.PrepareRequest(req, body)
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	if plan.ContentType == "" || plan.ContentType == "application/json" {
		t.Fatalf("ContentType = %q, want multipart", plan.ContentType)
	}
	if plan.RequestPath != "/v1/images/edits" {
		t.Fatalf("RequestPath = %q", plan.RequestPath)
	}
	mediaType, params, err := mime.ParseMediaType(plan.ContentType)
	if err != nil {
		t.Fatalf("parse multipart content type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("mediaType = %q", mediaType)
	}
	reader := multipart.NewReader(bytes.NewReader(plan.Body), params["boundary"])
	foundPrompt := false
	foundImage := false
	foundCount := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		payload, _ := io.ReadAll(part)
		switch part.FormName() {
		case "prompt":
			foundPrompt = string(payload) == "draw a cat"
		case "image[]":
			foundImage = len(payload) > 0
		case "n":
			foundCount = string(payload) == "3"
		}
	}
	if !foundPrompt || !foundImage || !foundCount {
		t.Fatalf("multipart fields missing: prompt=%v image=%v n=%v", foundPrompt, foundImage, foundCount)
	}

	upReq, err := runtime.BuildUpstreamRequest(req, plan)
	if err != nil {
		t.Fatalf("BuildUpstreamRequest err = %v", err)
	}
	if upReq.URL != "https://api.openai.com/v1/images/edits" {
		t.Fatalf("upstream url = %q", upReq.URL)
	}
	if upReq.Headers["Content-Type"] != plan.ContentType {
		t.Fatalf("Content-Type = %q, want %q", upReq.Headers["Content-Type"], plan.ContentType)
	}
}

func TestPrepareRequestBridgesGeminiImageEditUsingJSONTransport(t *testing.T) {
	runtime := NewRuntime()
	body := []byte(`{"generationConfig":{"responseModalities":["TEXT","IMAGE"]},"contents":[{"role":"user","parts":[{"text":"draw a cat"},{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}]}`)
	httpReq := httptest.NewRequest("POST", "/v1beta/models/imagen-4:generateContent", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolGeminiGenerate,
		ClientPath:     "/v1beta/models/imagen-4:generateContent",
		CapabilityType: domain.CapabilityImage,
		Candidate: &domain.RouteCandidate{
			Protocol:           domain.ProtocolOpenAIImages,
			UpstreamModel:      "gpt-image-1",
			ImageEditTransport: domain.ImageEditTransportJSON,
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	plan, err := runtime.PrepareRequest(req, body)
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	if plan.ContentType != "application/json" {
		t.Fatalf("ContentType = %q, want application/json", plan.ContentType)
	}
	if plan.RequestPath != "/v1/images/edits" {
		t.Fatalf("RequestPath = %q", plan.RequestPath)
	}
	var payload struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Images []struct {
			ImageURL string `json:"image_url"`
		} `json:"images"`
	}
	if err := json.Unmarshal(plan.Body, &payload); err != nil {
		t.Fatalf("unmarshal prepared body: %v", err)
	}
	if payload.Model != "gpt-image-1" || payload.Prompt != "draw a cat" {
		t.Fatalf("model/prompt = %q/%q", payload.Model, payload.Prompt)
	}
	if len(payload.Images) != 1 || payload.Images[0].ImageURL != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("images = %#v", payload.Images)
	}
}

func TestBridgeResponseConvertsGeminiImagesToOpenAI(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiGenerate,
			UpstreamModel: "imagen-4",
		},
		Envelope: &serving.RequestEnvelope{},
	}
	resp := []byte(`{"candidates":[{"content":{"parts":[{"text":"revised"},{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}`)
	got, err := runtime.BridgeResponse(req, resp)
	if err != nil {
		t.Fatalf("BridgeResponse err = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("BridgeResponse invalid json: %v", err)
	}
	data, ok := out["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("data = %#v", out["data"])
	}
	item := data[0].(map[string]any)
	if item["b64_json"] != "aGVsbG8=" {
		t.Fatalf("b64_json = %#v", item["b64_json"])
	}
	if item["revised_prompt"] != "revised" {
		t.Fatalf("revised_prompt = %#v", item["revised_prompt"])
	}
}

func TestBridgeResponseConvertsOpenAIImagesToGemini(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		ClientProtocol: domain.ProtocolGeminiGenerate,
		CapabilityType: domain.CapabilityImage,
		RequestedModel: "gpt-image-1",
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolOpenAIImages,
			UpstreamModel: "gpt-image-1",
		},
	}
	resp := []byte(`{"created":1234,"data":[{"b64_json":"aGVsbG8=","output_format":"png","revised_prompt":"revised"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
	got, err := runtime.BridgeResponse(req, resp)
	if err != nil {
		t.Fatalf("BridgeResponse err = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("BridgeResponse invalid json: %v", err)
	}
	if out["modelVersion"] != "gpt-image-1" {
		t.Fatalf("modelVersion = %#v", out["modelVersion"])
	}
	candidates, ok := out["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("candidates = %#v", out["candidates"])
	}
}

func TestPrepareRequestBridgesOpenAIEmbeddingsToGemini(t *testing.T) {
	runtime := NewRuntime()
	httpReq := httptest.NewRequest("POST", "/v1/embeddings", nil)
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIEmbeddings,
		ClientPath:     "/v1/embeddings",
		CapabilityType: domain.CapabilityEmbedding,
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiEmbeddings,
			UpstreamModel: "text-embedding-004",
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	plan, err := runtime.PrepareRequest(req, []byte(`{"model":"text-embedding-3-large","input":["alpha","beta"],"dimensions":256}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plan.Body, &out); err != nil {
		t.Fatalf("PrepareRequest returned invalid json: %v", err)
	}
	if out["model"] != "text-embedding-004" {
		t.Fatalf("model = %#v", out["model"])
	}
	if out["outputDimensionality"] != float64(256) {
		t.Fatalf("outputDimensionality = %#v", out["outputDimensionality"])
	}
	requests, ok := out["requests"].([]any)
	if !ok || len(requests) != 2 {
		t.Fatalf("requests = %#v", out["requests"])
	}
}

func TestPrepareRequestRejectsOpenAIEmbeddingTokenArrayForGemini(t *testing.T) {
	runtime := NewRuntime()
	httpReq := httptest.NewRequest("POST", "/v1/embeddings", nil)
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIEmbeddings,
		ClientPath:     "/v1/embeddings",
		CapabilityType: domain.CapabilityEmbedding,
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiEmbeddings,
			UpstreamModel: "text-embedding-004",
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	_, err := runtime.PrepareRequest(req, []byte(`{"model":"text-embedding-3-large","input":[1,2,3]}`))
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("PrepareRequest err = %T, want *serving.APIError", err)
	}
	if apiErr.Status != 400 {
		t.Fatalf("status = %d, want 400", apiErr.Status)
	}
}

func TestPrepareRequestBridgesGeminiEmbeddingsToOpenAI(t *testing.T) {
	runtime := NewRuntime()
	httpReq := httptest.NewRequest("POST", "/v1beta/models/text-embedding-004:embedContent", nil)
	httpReq.Header.Set("Content-Type", "application/json")
	req := &serving.Request{
		ClientProtocol: domain.ProtocolGeminiEmbeddings,
		ClientPath:     "/v1beta/models/text-embedding-004:embedContent",
		CapabilityType: domain.CapabilityEmbedding,
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolOpenAIEmbeddings,
			UpstreamModel: "text-embedding-3-large",
		},
		Envelope: &serving.RequestEnvelope{R: httpReq},
	}

	plan, err := runtime.PrepareRequest(req, []byte(`{"model":"text-embedding-004","requests":[{"content":{"parts":[{"text":"alpha"}]}},{"content":{"parts":[{"text":"beta"}]}}],"outputDimensionality":128}`))
	if err != nil {
		t.Fatalf("PrepareRequest err = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plan.Body, &out); err != nil {
		t.Fatalf("PrepareRequest returned invalid json: %v", err)
	}
	if out["model"] != "text-embedding-3-large" {
		t.Fatalf("model = %#v", out["model"])
	}
	if out["dimensions"] != float64(128) {
		t.Fatalf("dimensions = %#v", out["dimensions"])
	}
	inputs, ok := out["input"].([]any)
	if !ok || len(inputs) != 2 {
		t.Fatalf("input = %#v", out["input"])
	}
}

func TestBridgeResponseConvertsGeminiEmbeddingsToOpenAI(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIEmbeddings,
		CapabilityType: domain.CapabilityEmbedding,
		RequestedModel: "text-embedding-3-large",
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiEmbeddings,
			UpstreamModel: "text-embedding-004",
		},
	}
	resp := []byte(`{"embeddings":[{"values":[0.1,0.2]},{"values":[0.3,0.4]}],"usageMetadata":{"promptTokenCount":2,"totalTokenCount":2}}`)
	got, err := runtime.BridgeResponse(req, resp)
	if err != nil {
		t.Fatalf("BridgeResponse err = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("BridgeResponse invalid json: %v", err)
	}
	if out["model"] != "text-embedding-3-large" {
		t.Fatalf("model = %#v", out["model"])
	}
	data, ok := out["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("data = %#v", out["data"])
	}
	usage, ok := out["usage"].(map[string]any)
	if !ok || usage["input_tokens"] != float64(2) {
		t.Fatalf("usage = %#v", out["usage"])
	}
}

func TestBridgeResponseConvertsOpenAIEmbeddingsToGemini(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		ClientProtocol: domain.ProtocolGeminiEmbeddings,
		CapabilityType: domain.CapabilityEmbedding,
		RequestedModel: "text-embedding-004",
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolOpenAIEmbeddings,
			UpstreamModel: "text-embedding-3-large",
		},
	}
	resp := []byte(`{"model":"text-embedding-3-large","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`)
	got, err := runtime.BridgeResponse(req, resp)
	if err != nil {
		t.Fatalf("BridgeResponse err = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("BridgeResponse invalid json: %v", err)
	}
	if out["modelVersion"] != "text-embedding-004" {
		t.Fatalf("modelVersion = %#v", out["modelVersion"])
	}
	embedding, ok := out["embedding"].(map[string]any)
	if !ok {
		t.Fatalf("embedding = %#v", out["embedding"])
	}
	values, ok := embedding["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("embedding values = %#v", embedding["values"])
	}
}

func TestPrepareRequestRejectsPartialImages(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiGenerate,
			UpstreamModel: "imagen-4",
		},
	}

	_, err := runtime.PrepareRequest(req, []byte(`{"model":"gpt-image-1","prompt":"poster","partial_images":1}`))
	if err == nil {
		t.Fatal("expected partial_images to be rejected")
	}
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != 400 || apiErr.Code != "invalid_request_error" {
		t.Fatalf("apiErr = %+v", apiErr)
	}
	if apiErr.Message != imagePartialImagesUnsupportedMessage {
		t.Fatalf("message = %q", apiErr.Message)
	}
}

func TestBridgeImageStreamConvertsGeminiToOpenAI(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		ClientPath:     "/v1/images/generations",
		CapabilityType: domain.CapabilityImage,
		RequestedModel: "gpt-image-1",
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiGenerate,
			UpstreamModel: "imagen-4",
		},
	}

	raw := []byte("data: {\"responseId\":\"resp_gem_stream_123\",\"modelVersion\":\"imagen-4\",\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"inlineData\":{\"mimeType\":\"image/png\",\"data\":\"aGVsbG8=\"}}]}}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":3,\"totalTokenCount\":5}}\n\n")
	result, err := runtime.BridgeImageStream(req, raw)
	if err != nil {
		t.Fatalf("BridgeImageStream err = %v", err)
	}
	if !bytes.Contains(result.ClientStream, []byte("event: image_generation.completed")) {
		t.Fatalf("client stream = %q", string(result.ClientStream))
	}
	if !bytes.Contains(result.ClientStream, []byte("\"b64_json\":\"aGVsbG8=\"")) {
		t.Fatalf("client stream missing image payload: %q", string(result.ClientStream))
	}
	var provider map[string]any
	if err := json.Unmarshal(result.ProviderBody, &provider); err != nil {
		t.Fatalf("provider body invalid json: %v", err)
	}
	if provider["modelVersion"] != "imagen-4" {
		t.Fatalf("provider modelVersion = %#v", provider["modelVersion"])
	}
}

func TestAggregateOpenAIImageStreamAcceptsCompletedTopLevelImageData(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		CapabilityType: domain.CapabilityImage,
		Candidate: &domain.RouteCandidate{
			Protocol: domain.ProtocolOpenAIImages,
		},
	}
	raw := []byte("event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":\"aGVsbG8=\",\"usage\":{\"total_tokens\":10}}\n\n")

	aggregated, err := runtime.AggregateImageProviderBody(req, raw)
	if err != nil {
		t.Fatalf("AggregateImageProviderBody err = %v", err)
	}
	var result struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(aggregated, &result); err != nil {
		t.Fatalf("aggregated body is not valid JSON: %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].B64JSON != "aGVsbG8=" {
		t.Fatalf("aggregated image data = %+v, want top-level completed image", result.Data)
	}
	if result.Usage["total_tokens"] != float64(10) {
		t.Fatalf("aggregated usage = %+v", result.Usage)
	}
}

func TestAggregateOpenAIImageStreamAcceptsGenerationResultObject(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		CapabilityType: domain.CapabilityImage,
		Candidate: &domain.RouteCandidate{
			Protocol: domain.ProtocolOpenAIImages,
		},
	}
	raw := []byte(
		"data: {\"object\":\"image.generation.chunk\",\"created\":123,\"model\":\"gpt-image-2\",\"data\":[]}\n\n" +
			"data: {\"object\":\"image.generation.result\",\"created\":124,\"model\":\"gpt-image-2\",\"data\":[{\"b64_json\":\"aGVsbG8=\",\"revised_prompt\":\"A cute sea otter\"}],\"usage\":{\"total_tokens\":1062}}\n\n" +
			"data: [DONE]\n\n",
	)

	aggregated, err := runtime.AggregateImageProviderBody(req, raw)
	if err != nil {
		t.Fatalf("AggregateImageProviderBody err = %v", err)
	}
	var result struct {
		Created int64 `json:"created"`
		Data    []struct {
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(aggregated, &result); err != nil {
		t.Fatalf("aggregated body is not JSON: %v", err)
	}
	if result.Created != 124 || len(result.Data) != 1 || result.Data[0].B64JSON != "aGVsbG8=" || result.Data[0].RevisedPrompt != "A cute sea otter" {
		t.Fatalf("aggregated result = %+v", result)
	}
	if result.Usage["total_tokens"] != float64(1062) {
		t.Fatalf("aggregated usage = %+v", result.Usage)
	}
}

func TestBuildOpenAIImageClientStreamPreservesHTTPImageURL(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		ClientPath:     "/v1/images/generations",
	}

	stream, err := runtime.BuildImageClientStream(req, []byte(`{"data":[{"url":"https://example.test/generated.png"}]}`))
	if err != nil {
		t.Fatalf("BuildImageClientStream err = %v", err)
	}
	var event struct {
		URL string `json:"url"`
	}
	parts := bytes.SplitN(stream, []byte("data: "), 2)
	if len(parts) != 2 {
		t.Fatalf("stream missing SSE data frame: %q", stream)
	}
	if err := json.Unmarshal(bytes.TrimSpace(parts[1]), &event); err != nil {
		t.Fatalf("unmarshal SSE payload: %v", err)
	}
	if event.URL != "https://example.test/generated.png" {
		t.Fatalf("url = %q", event.URL)
	}
}

func TestBridgeImageStreamConvertsOpenAIToGemini(t *testing.T) {
	runtime := NewRuntime()
	req := &serving.Request{
		ClientProtocol: domain.ProtocolGeminiGenerate,
		ClientPath:     "/v1beta/models/imagen-4:streamGenerateContent",
		CapabilityType: domain.CapabilityImage,
		RequestedModel: "imagen-4",
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolOpenAIImages,
			UpstreamModel: "gpt-image-1",
		},
	}

	raw := []byte(
		"event: response.output_item.done\n" +
			"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"image_generation_call\",\"output_format\":\"png\",\"result\":\"aGVsbG8=\"}}\n\n" +
			"event: response.completed\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_img_123\",\"model\":\"gpt-image-1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\n",
	)
	result, err := runtime.BridgeImageStream(req, raw)
	if err != nil {
		t.Fatalf("BridgeImageStream err = %v", err)
	}
	if !bytes.HasPrefix(result.ClientStream, []byte("data: {")) {
		t.Fatalf("client stream = %q", string(result.ClientStream))
	}
	if !bytes.Contains(result.ClientStream, []byte("\"mimeType\":\"image/png\"")) || !bytes.Contains(result.ClientStream, []byte("\"data\":\"aGVsbG8=\"")) {
		t.Fatalf("client stream missing gemini inlineData: %q", string(result.ClientStream))
	}
	var provider map[string]any
	if err := json.Unmarshal(result.ProviderBody, &provider); err != nil {
		t.Fatalf("provider body invalid json: %v", err)
	}
	if provider["model"] != "gpt-image-1" {
		t.Fatalf("provider model = %#v", provider["model"])
	}
}

func TestRequestEnvelopeUsesImageIRKind(t *testing.T) {
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiGenerate,
			UpstreamModel: "imagen-4",
		},
	}

	env := requestEnvelope(req)
	if env.Kind != corebridge.IRKindImage {
		t.Fatalf("request envelope kind = %q, want image", env.Kind)
	}
}

func TestRequestEnvelopeUsesEmbeddingIRKind(t *testing.T) {
	req := &serving.Request{
		ClientProtocol: domain.ProtocolOpenAIEmbeddings,
		CapabilityType: domain.CapabilityEmbedding,
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolGeminiEmbeddings,
			UpstreamModel: "text-embedding-004",
		},
	}

	env := requestEnvelope(req)
	if env.Kind != corebridge.IRKindEmbedding {
		t.Fatalf("request envelope kind = %q, want embedding", env.Kind)
	}
}

func TestRuntimeRegistersImageBridgeDefinitions(t *testing.T) {
	runtime := NewRuntime()
	if !runtime.registry.Supports(corebridge.IRKindImage, surface.OpenAIImages, surface.GeminiImages) {
		t.Fatal("expected runtime registry to support openai_images -> gemini_images")
	}
	if _, ok := runtime.lookupRequestHandler(corebridge.IRKindImage, surface.OpenAIImages, surface.GeminiImages); !ok {
		t.Fatal("expected image request handler to be registered")
	}
	if _, ok := runtime.lookupResponseHandler(corebridge.IRKindImage, surface.GeminiImages, surface.OpenAIImages); !ok {
		t.Fatal("expected image response handler to be registered")
	}
}

func TestRuntimeRegistersEmbeddingBridgeDefinitions(t *testing.T) {
	runtime := NewRuntime()
	if !runtime.registry.Supports(corebridge.IRKindEmbedding, surface.OpenAIEmbeddings, surface.GeminiEmbeddings) {
		t.Fatal("expected runtime registry to support openai_embeddings -> gemini_embeddings")
	}
	if !runtime.registry.Supports(corebridge.IRKindEmbedding, surface.GeminiEmbeddings, surface.OpenAIEmbeddings) {
		t.Fatal("expected runtime registry to support gemini_embeddings -> openai_embeddings")
	}
	if _, ok := runtime.lookupRequestHandler(corebridge.IRKindEmbedding, surface.OpenAIEmbeddings, surface.GeminiEmbeddings); !ok {
		t.Fatal("expected embedding request handler to be registered")
	}
	if _, ok := runtime.lookupResponseHandler(corebridge.IRKindEmbedding, surface.GeminiEmbeddings, surface.OpenAIEmbeddings); !ok {
		t.Fatal("expected embedding response handler to be registered")
	}
}

func TestRuntimeRegistersChatStreamHandlers(t *testing.T) {
	runtime := NewRuntime()
	if _, ok := runtime.lookupStreamProviderFactory(corebridge.IRKindChat, surface.OpenAIChat, surface.GeminiText); !ok {
		t.Fatal("expected chat stream provider factory to be registered")
	}
	if _, ok := runtime.lookupStreamEmitterFactory(corebridge.IRKindChat, surface.OpenAIChat, surface.GeminiText); !ok {
		t.Fatal("expected chat stream emitter factory to be registered")
	}
}

func mustMultipartImageEditBody(t *testing.T, prompt string) ([]byte, string) {
	t.Helper()
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	if err := writer.WriteField("model", "gpt-image-1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("prompt", prompt); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image[]", "image.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(mustBridgeTestImagePNG(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes(), writer.FormDataContentType()
}

const bridgeTestImagePNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/lAAAAABJRU5ErkJggg=="

func mustBridgeTestImagePNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(bridgeTestImagePNGBase64)
	if err != nil {
		t.Fatalf("decode test PNG: %v", err)
	}
	return data
}

func multipartScalarFieldsForTest(body []byte, boundary string) (map[string]string, error) {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	fields := map[string]string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if part.FileName() != "" {
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return nil, err
		}
		fields[part.FormName()] = string(data)
	}
	return fields, nil
}

func multipartFilesForTest(body []byte, boundary string) (map[string][]byte, error) {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	files := map[string][]byte{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if part.FileName() == "" {
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return nil, err
		}
		files[part.FormName()] = data
	}
	return files, nil
}

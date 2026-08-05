package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/appkey"
	"xiaodou/dai/internal/ai/application"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/imageedit"
)

func TestMarshalRunChatBodyPreservesDefaultsAndReservedFields(t *testing.T) {
	body := marshalRunChatBody(
		"gpt-4.1-mini",
		"system prompt",
		map[string]any{"temperature": 0.2, "top_p": 0.8, "model": "bad", "messages": []any{}, "stream": false},
		runRequest{
			Input:  "hello world",
			Stream: true,
		},
	)

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got := payload["model"]; got != "gpt-4.1-mini" {
		t.Fatalf("model = %#v, want gpt-4.1-mini", got)
	}
	if got := payload["stream"]; got != true {
		t.Fatalf("stream = %#v, want true", got)
	}
	if got := payload["temperature"]; got != 0.2 {
		t.Fatalf("temperature = %#v, want 0.2", got)
	}
	if got := payload["top_p"]; got != 0.8 {
		t.Fatalf("top_p = %#v, want 0.8", got)
	}

	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want 2 items", payload["messages"])
	}
	first, _ := messages[0].(map[string]any)
	second, _ := messages[1].(map[string]any)
	if first["role"] != "system" || first["content"] != "system prompt" {
		t.Fatalf("system message = %#v", first)
	}
	if second["role"] != "user" || second["content"] != "hello world" {
		t.Fatalf("user message = %#v", second)
	}
}

func TestRenderPromptTemplateReplacesSupportedPlaceholders(t *testing.T) {
	got := application.RenderTemplate(
		"Hello {{tenant_name}}, agent={{ agent_name }}, untouched={{missing}}",
		map[string]string{
			"tenant_name": "Acme",
			"agent_name":  "Writer",
		},
	)
	want := "Hello Acme, agent=Writer, untouched={{missing}}"
	if got != want {
		t.Fatalf("renderPromptTemplate = %q, want %q", got, want)
	}
}

func chatAppExpansion(chat map[string]any) coreruntime.InvokeExpansion {
	return coreruntime.InvokeExpansion{
		BoundModel: "gpt-5.4",
		App: &application.RuntimeApp{
			App: application.App{
				AppType:        application.AppTypeChatAgent,
				BoundModelID:   "gpt-5.4",
				Status:         application.StatusActive,
				PromptStrategy: application.PromptStrategyCallerVariables,
				DefaultOptions: map[string]any{"chat": chat},
			},
			PromptBindings: []application.RuntimePromptBinding{
				{Role: application.PromptBindingSystem, TemplateText: "hello {{name}}"},
			},
		},
	}
}

func TestBuildRunChatBodyFromExpansionAgent(t *testing.T) {
	body, err := buildRunChatBodyFromExpansion(
		chatAppExpansion(map[string]any{"creativity": "precise"}),
		runRequest{
			Input:     "say hi",
			Stream:    true,
			Variables: map[string]string{"name": "alice"},
		},
	)
	if err != nil {
		t.Fatalf("buildRunChatBodyFromExpansion: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["model"] != "gpt-5.4" {
		t.Fatalf("model = %#v, want gpt-5.4", payload["model"])
	}
	// creativity=precise maps to temperature 0.2.
	if payload["temperature"] != 0.2 {
		t.Fatalf("temperature = %#v, want 0.2", payload["temperature"])
	}
	if _, ok := payload["max_tokens"]; ok {
		t.Fatalf("max_tokens should never be sent for an app: %#v", payload["max_tokens"])
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want 2 items", payload["messages"])
	}
	system, _ := messages[0].(map[string]any)
	if system["content"] != "hello alice" {
		t.Fatalf("system content = %#v, want hello alice", system["content"])
	}
}

func TestBuildRunChatBodyRejectsCallerOverrides(t *testing.T) {
	temp := 0.9
	_, err := buildRunChatBodyFromExpansion(
		chatAppExpansion(map[string]any{"creativity": "balanced"}),
		runRequest{Input: "hi", Temperature: &temp},
	)
	if !errors.Is(err, errRunOptionOverrideDenied) {
		t.Fatalf("err = %v, want errRunOptionOverrideDenied", err)
	}
}

func TestBuildRunChatBodyAttachments(t *testing.T) {
	// Attachments rejected when the app disallows them.
	_, err := buildRunChatBodyFromExpansion(
		chatAppExpansion(map[string]any{"allow_attachments": false}),
		runRequest{Input: "look", Attachments: []runAttachment{{Type: "image", URL: "https://x/y.png"}}},
	)
	if !errors.Is(err, errRunAttachmentsNotAllowed) {
		t.Fatalf("err = %v, want errRunAttachmentsNotAllowed", err)
	}

	// Attachments accepted and converted to a multi-part content array.
	body, err := buildRunChatBodyFromExpansion(
		chatAppExpansion(map[string]any{"allow_attachments": true}),
		runRequest{Input: "look", Variables: map[string]string{"name": "alice"}, Attachments: []runAttachment{
			{Type: "image", URL: "https://x/y.png"},
			{URL: "https://x/doc.pdf", Name: "doc.pdf"},
		}},
	)
	if err != nil {
		t.Fatalf("buildRunChatBodyFromExpansion: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	messages := payload["messages"].([]any)
	user := messages[len(messages)-1].(map[string]any)
	parts, ok := user["content"].([]any)
	if !ok || len(parts) != 3 {
		t.Fatalf("content parts = %#v, want text + 2 attachments", user["content"])
	}
	img := parts[1].(map[string]any)
	if img["type"] != "image_url" {
		t.Fatalf("attachment[0] type = %#v, want image_url", img["type"])
	}
	file := parts[2].(map[string]any)
	if file["type"] != "file" {
		t.Fatalf("attachment[1] type = %#v, want file", file["type"])
	}
}

func TestBuildRunChatBodyInfersImageWithoutType(t *testing.T) {
	// No explicit type nor mime_type: a bare image URL must still go as image_url,
	// while a bare non-image URL falls back to file.
	body, err := buildRunChatBodyFromExpansion(
		chatAppExpansion(map[string]any{"allow_attachments": true}),
		runRequest{Input: "look", Variables: map[string]string{"name": "alice"}, Attachments: []runAttachment{
			{URL: "https://cdn.example.com/a/photo.JPG?sig=abc"},
			{URL: "https://cdn.example.com/a/notes.txt"},
		}},
	)
	if err != nil {
		t.Fatalf("buildRunChatBodyFromExpansion: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	parts := payload["messages"].([]any)[1].(map[string]any)["content"].([]any)
	if parts[1].(map[string]any)["type"] != "image_url" {
		t.Fatalf("bare image URL should infer image_url: %#v", parts[1])
	}
	if parts[2].(map[string]any)["type"] != "file" {
		t.Fatalf("bare non-image URL should be file: %#v", parts[2])
	}
}

func imageAppExpansion(appType application.AppType, image map[string]any) coreruntime.InvokeExpansion {
	return coreruntime.InvokeExpansion{
		BoundModel: "gpt-image-1",
		App: &application.RuntimeApp{
			App: application.App{
				AppType:        appType,
				BoundModelID:   "gpt-image-1",
				Status:         application.StatusActive,
				PromptStrategy: application.PromptStrategyCallerVariables,
				DefaultOptions: map[string]any{"image": image},
			},
			PromptBindings: []application.RuntimePromptBinding{
				{Role: application.PromptBindingInputTemplate, TemplateText: "poster {{topic}}"},
			},
		},
	}
}

func withImageVariables(req runImageGenerationRequest) runImageGenerationRequest {
	req.Variables = map[string]string{"topic": "coffee"}
	return req
}

func TestBuildRunImageGenerationBodyFromExpansionAgent(t *testing.T) {
	body, err := buildRunImageGenerationBodyFromExpansion(
		imageAppExpansion(application.AppTypeImageGenerationAgent, map[string]any{"resolution": "1k", "aspect_ratio": "2:3"}),
		runImageGenerationRequest{
			Input:     "with neon lights",
			Variables: map[string]string{"topic": "coffee"},
		},
	)
	if err != nil {
		t.Fatalf("buildRunImageGenerationBodyFromExpansion: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["model"] != "gpt-image-1" {
		t.Fatalf("model = %#v, want gpt-image-1", payload["model"])
	}
	if payload["size"] != "832x1248" {
		t.Fatalf("size = %#v, want 832x1248", payload["size"])
	}
	// App image runs default to b64_json unless the caller explicitly asks for url.
	if payload["response_format"] != "b64_json" {
		t.Fatalf("response_format = %#v, want b64_json", payload["response_format"])
	}
	if payload["prompt"] != "poster coffee\n\nwith neon lights" {
		t.Fatalf("prompt = %#v", payload["prompt"])
	}
}

func TestBuildRunImageGenerationRejectsSizeAllowsResponseFormat(t *testing.T) {
	// Caller may not override the app-fixed resolution.
	_, err := buildRunImageGenerationBodyFromExpansion(
		imageAppExpansion(application.AppTypeImageGenerationAgent, map[string]any{"resolution": "1024x1024"}),
		runImageGenerationRequest{Input: "x", Size: "2048x2048"},
	)
	if !errors.Is(err, errRunOptionOverrideDenied) {
		t.Fatalf("err = %v, want errRunOptionOverrideDenied", err)
	}

	// Caller may choose response_format.
	body, err := buildRunImageGenerationBodyFromExpansion(
		imageAppExpansion(application.AppTypeImageGenerationAgent, map[string]any{"resolution": "1024x1024"}),
		runImageGenerationRequest{Input: "x", Variables: map[string]string{"topic": "coffee"}, ResponseFormat: "b64_json"},
	)
	if err != nil {
		t.Fatalf("buildRunImageGenerationBodyFromExpansion: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["response_format"] != "b64_json" {
		t.Fatalf("response_format = %#v, want b64_json", payload["response_format"])
	}
	if payload["size"] != "1024x1024" {
		t.Fatalf("size = %#v, want 1024x1024", payload["size"])
	}
}

func TestDecodeRunImageGenerationIgnoresQualityAndStyle(t *testing.T) {
	req, err := decodeRunImageGenerationRequestBody([]byte(`{
		"input":"poster",
		"quality":"high",
		"style":"vivid"
	}`))
	if err != nil {
		t.Fatalf("decodeRunImageGenerationRequestBody: %v", err)
	}
	body, err := buildRunImageGenerationBodyFromExpansion(
		imageAppExpansion(application.AppTypeImageGenerationAgent, map[string]any{"resolution": "1024x1024"}),
		withImageVariables(req),
	)
	if err != nil {
		t.Fatalf("buildRunImageGenerationBodyFromExpansion: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	for _, field := range []string{"quality", "style"} {
		if _, ok := payload[field]; ok {
			t.Fatalf("application request must ignore %s: %#v", field, payload)
		}
	}
}

func TestBuildRunImageGenerationBodyPreservesExplicitStream(t *testing.T) {
	body, err := buildRunImageGenerationBodyFromExpansion(
		imageAppExpansion(application.AppTypeImageGenerationAgent, map[string]any{"resolution": "1024x1024"}),
		runImageGenerationRequest{
			Input: "poster", Variables: map[string]string{"topic": "coffee"},
			Stream: true,
		},
	)
	if err != nil {
		t.Fatalf("buildRunImageGenerationBodyFromExpansion: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["stream"] != true {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
}

func TestBuildRunImageGenerationBodyPreservesAutoSize(t *testing.T) {
	body, err := buildRunImageGenerationBodyFromExpansion(
		imageAppExpansion(application.AppTypeImageGenerationAgent, map[string]any{"resolution": "auto"}),
		runImageGenerationRequest{Input: "x", Variables: map[string]string{"topic": "coffee"}},
	)
	if err != nil {
		t.Fatalf("buildRunImageGenerationBodyFromExpansion: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["size"] != "auto" {
		t.Fatalf("size = %#v, want auto", payload["size"])
	}
}

func TestBuildRunChatBodyResolvesBoundPromptsByChineseName(t *testing.T) {
	expansion := chatAppExpansion(map[string]any{"creativity": "balanced"})
	expansion.App.App.PromptStrategy = application.PromptStrategyBoundExact
	expansion.App.PromptBindings = []application.RuntimePromptBinding{
		{PromptID: "prompt-a", PromptName: "客户背景", PromptRevision: 4, TemplateText: "客户是小豆科技"},
		{PromptID: "prompt-b", PromptName: "售前规范", PromptRevision: 2, TemplateText: "回答必须清晰"},
	}
	body, err := buildRunChatBodyFromExpansion(expansion, runRequest{
		Input: "请结合 {{客户背景}}，遵循 {{售前规范}}。",
	})
	if err != nil {
		t.Fatalf("build dynamic chat body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	messages := payload["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want only a user message", messages)
	}
	if got := messages[0].(map[string]any)["content"]; got != "请结合 客户是小豆科技，遵循 回答必须清晰。" {
		t.Fatalf("dynamic input = %#v", got)
	}
}

func TestBuildRunImageBodiesResolveBoundPromptsFromInput(t *testing.T) {
	expansion := imageAppExpansion(application.AppTypeImageGenerationAgent, map[string]any{"resolution": "1k", "aspect_ratio": "1:1"})
	expansion.App.App.PromptStrategy = application.PromptStrategyBoundExact
	expansion.App.PromptBindings = []application.RuntimePromptBinding{
		{PromptID: "prompt-a", PromptName: "品牌风格", PromptRevision: 3, TemplateText: "冷静、专业的蓝白视觉"},
	}
	body, err := buildRunImageGenerationBodyFromExpansion(expansion, runImageGenerationRequest{Input: "生成海报，采用 {{品牌风格}}。"})
	if err != nil {
		t.Fatalf("build dynamic image generation body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal generation: %v", err)
	}
	if got := payload["prompt"]; got != "生成海报，采用 冷静、专业的蓝白视觉。" {
		t.Fatalf("generation prompt = %#v", got)
	}

	expansion.App.App.AppType = application.AppTypeImageEditAgent
	body, err = buildRunImageEditBodyFromExpansion(expansion, imageedit.Request{
		Prompt: "保留主体，采用 {{品牌风格}}。",
		Images: []imageedit.Source{{URL: "https://example.com/source.png"}},
	}, nil)
	if err != nil {
		t.Fatalf("build dynamic image edit body: %v", err)
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal edit: %v", err)
	}
	if got := payload["prompt"]; got != "保留主体，采用 冷静、专业的蓝白视觉。" {
		t.Fatalf("edit prompt = %#v", got)
	}
}

func TestDecodeRunImageGenerationUsesUnifiedInput(t *testing.T) {
	req, err := decodeRunImageGenerationRequestBody([]byte(`{"input":"生成产品海报"}`))
	if err != nil {
		t.Fatalf("decode image input: %v", err)
	}
	if req.Input != "生成产品海报" {
		t.Fatalf("run input = %q", req.Input)
	}
}

func TestRunImageAppsRequireUnifiedInput(t *testing.T) {
	generation := imageAppExpansion(application.AppTypeImageGenerationAgent, map[string]any{"resolution": "1k", "aspect_ratio": "1:1"})
	if _, err := buildRunImageGenerationBodyFromExpansion(generation, runImageGenerationRequest{}); !errors.Is(err, errRunInputRequired) {
		t.Fatalf("generation error = %v, want input required", err)
	}

	edit := imageAppExpansion(application.AppTypeImageEditAgent, map[string]any{"resolution": "1k", "aspect_ratio": "1:1"})
	if _, err := buildRunImageEditBodyFromExpansion(edit, imageedit.Request{
		Images: []imageedit.Source{{URL: "https://example.com/source.png"}},
	}, nil); !errors.Is(err, errRunInputRequired) {
		t.Fatalf("edit error = %v, want input required", err)
	}
}

func TestDecodeRunChatRequestBodyRejectsPromptField(t *testing.T) {
	_, err := decodeRunChatRequestBody(strings.NewReader(`{"input":"hello","prompt":"legacy"}`))
	if err == nil || !strings.Contains(err.Error(), "prompt is not supported for app runs; use input") {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleRunRuntimeUsesRuntimeEngine(t *testing.T) {
	expander := &runInvokeExpanderStub{
		expansion: coreruntime.InvokeExpansion{
			Subject: coreidentity.Subject{
				AuthMethod:    coreidentity.AuthMethodInvokeKey,
				RequestSource: coreidentity.RequestSourceInvokeKey,
				Scope:         coreidentity.ScopeTenant,
				TenantID:      "tenant-a",
			},
			Request: coreruntime.Request{
				RequestedModel:  "gpt-5.4",
				ResolvedModelID: "gpt-5.4",
			},
			InvokeKey: application.InvokeKey{
				Status: application.StatusActive,
			},
			BoundModel: "gpt-5.4",
			App: &application.RuntimeApp{
				App: application.App{
					AppType:        application.AppTypeChatAgent,
					BoundModelID:   "gpt-5.4",
					Status:         application.StatusActive,
					PromptStrategy: application.PromptStrategyCallerVariables,
					DefaultOptions: map[string]any{
						"chat": map[string]any{
							"creativity": "balanced",
						},
					},
				},
				PromptBindings: []application.RuntimePromptBinding{
					{Role: application.PromptBindingSystem, TemplateText: "system {{name}}"},
				},
			},
		},
	}
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeInvokeExpander: expander,
		runtimeEngine:         executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/run", strings.NewReader(`{"input":"hello","stream":true,"variables":{"name":"alice"}}`))
	req.Header.Set("Authorization", "Bearer rk_demo")
	rec := httptest.NewRecorder()

	gw.handleRunRuntime(rec, req)

	if expander.keyHash != appkey.Hash("rk_demo") {
		t.Fatalf("key hash = %q", expander.keyHash)
	}
	if executor.input.Subject.TenantID != "tenant-a" {
		t.Fatalf("resolver subject tenant = %q", executor.input.Subject.TenantID)
	}
	if executor.input.Request.Capability != "chat" {
		t.Fatalf("executor capability = %q, want chat", executor.input.Request.Capability)
	}
	if executor.input.Request.ClientSurface != surface.OpenAIChat {
		t.Fatalf("executor client surface = %q", executor.input.Request.ClientSurface)
	}
	if executor.input.Request.RequestedModel != "gpt-5.4" {
		t.Fatalf("executor requested model = %q", executor.input.Request.RequestedModel)
	}

	var payload map[string]any
	if err := json.Unmarshal(executor.input.Envelope.ClientBody, &payload); err != nil {
		t.Fatalf("unmarshal executor body: %v", err)
	}
	if payload["model"] != "gpt-5.4" {
		t.Fatalf("executor model = %#v", payload["model"])
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v", payload["messages"])
	}
	system, _ := messages[0].(map[string]any)
	if system["content"] != "system alice" {
		t.Fatalf("system prompt = %#v", system["content"])
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func TestHandleRunRuntimeImageGenerationSupportsStream(t *testing.T) {
	expander := &runInvokeExpanderStub{
		expansion: coreruntime.InvokeExpansion{
			Subject: coreidentity.Subject{
				AuthMethod:    coreidentity.AuthMethodInvokeKey,
				RequestSource: coreidentity.RequestSourceInvokeKey,
				Scope:         coreidentity.ScopeTenant,
				TenantID:      "tenant-a",
			},
			Request: coreruntime.Request{
				RequestedModel:  "gpt-image-1",
				ResolvedModelID: "gpt-image-1",
			},
			InvokeKey: application.InvokeKey{
				Status: application.StatusActive,
			},
			BoundModel: "gpt-image-1",
			App: &application.RuntimeApp{
				App: application.App{
					AppType:        application.AppTypeImageGenerationAgent,
					BoundModelID:   "gpt-image-1",
					Status:         application.StatusActive,
					PromptStrategy: application.PromptStrategyNone,
					DefaultOptions: map[string]any{"image": map[string]any{
						"resolution":                  "1024x1024",
						"default_output_count":        1,
						"max_output_count":            4,
						"allow_output_count_override": true,
					}},
				},
			},
		},
	}
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeInvokeExpander: expander,
		runtimeEngine:         executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/run", strings.NewReader(`{"input":"poster","stream":true,"n":3}`))
	req.Header.Set("Authorization", "Bearer rk_demo")
	rec := httptest.NewRecorder()

	gw.handleRunRuntime(rec, req)

	if executor.input.Request.Capability != "image_generation" {
		t.Fatalf("executor capability = %q, want image_generation", executor.input.Request.Capability)
	}
	if !executor.input.Request.Stream {
		t.Fatal("executor request stream = false, want true")
	}
	if got := executor.input.Envelope.HTTPRequest.Header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(executor.input.Envelope.ClientBody, &payload); err != nil {
		t.Fatalf("unmarshal executor body: %v", err)
	}
	if payload["stream"] != true {
		t.Fatalf("executor body stream = %#v, want true", payload["stream"])
	}
	if payload["n"] != float64(3) {
		t.Fatalf("executor body n = %#v, want 3", payload["n"])
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func imageEditAppExpansion() coreruntime.InvokeExpansion {
	return coreruntime.InvokeExpansion{
		Subject: coreidentity.Subject{TenantID: "tenant-1"},
		InvokeKey: application.InvokeKey{
			Status: application.StatusActive,
		},
		BoundModel: "gpt-image-1",
		App: &application.RuntimeApp{
			App: application.App{
				AppType:        application.AppTypeImageEditAgent,
				BoundModelID:   "gpt-image-1",
				Status:         application.StatusActive,
				PromptStrategy: application.PromptStrategyNone,
				DefaultOptions: map[string]any{"image": map[string]any{"resolution": "1024x1024"}},
			},
		},
	}
}

func TestHandleRunRuntimeImageEditSupportsStream(t *testing.T) {
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("image[]", "input.png")
	if err != nil {
		t.Fatalf("create image part: %v", err)
	}
	if _, err := part.Write(mustRunImageEditPNG(t)); err != nil {
		t.Fatalf("write image part: %v", err)
	}
	if err := writer.WriteField("input", "edit poster"); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := writer.WriteField("stream", "true"); err != nil {
		t.Fatalf("write stream: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	expander := &runInvokeExpanderStub{expansion: imageEditAppExpansion()}
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeInvokeExpander: expander,
		runtimeEngine:         executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/run", bytes.NewReader(form.Bytes()))
	req.Header.Set("Authorization", "Bearer rk_demo")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	gw.handleRunRuntime(rec, req)

	if executor.input.Request.Capability != "image_edit" {
		t.Fatalf("executor capability = %q, want image_edit", executor.input.Request.Capability)
	}
	if !executor.input.Request.Stream {
		t.Fatal("executor request stream = false, want true")
	}
	if got := executor.input.Envelope.HTTPRequest.Header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", got)
	}
	if got := executor.input.Envelope.HTTPRequest.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(executor.input.Envelope.ClientBody, &payload); err != nil {
		t.Fatalf("unmarshal canonical image edit body: %v", err)
	}
	if payload["stream"] != true {
		t.Fatalf("rewritten stream = %#v, want true", payload["stream"])
	}
	images, ok := payload["images"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("images = %#v, want one canonical image", payload["images"])
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func TestHandleRunRuntimeImageEditAcceptsJSONImageURL(t *testing.T) {
	expander := &runInvokeExpanderStub{expansion: imageEditAppExpansion()}
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeInvokeExpander: expander,
		runtimeEngine:         executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/run", strings.NewReader(`{
		"input": "make it brighter",
		"images": [{"image_url": "https://example.com/ref.png"}],
		"n": 1,
		"quality": "high",
		"style": "vivid",
		"response_format": "url",
		"stream": true
	}`))
	req.Header.Set("Authorization", "Bearer rk_demo")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleRunRuntime(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if !executor.input.Request.Stream {
		t.Fatal("executor request stream = false, want true")
	}
	if got := executor.input.Envelope.HTTPRequest.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(executor.input.Envelope.ClientBody, &payload); err != nil {
		t.Fatalf("unmarshal executor body: %v", err)
	}
	if payload["model"] != "gpt-image-1" {
		t.Fatalf("model = %#v, want gpt-image-1", payload["model"])
	}
	images, ok := payload["images"].([]any)
	if !ok || len(images) != 1 || images[0].(map[string]any)["image_url"] != "https://example.com/ref.png" {
		t.Fatalf("images = %#v", payload["images"])
	}
	if _, ok := payload["n"]; ok {
		t.Fatalf("n should not be forwarded: %#v", payload["n"])
	}
	for _, field := range []string{"quality", "style"} {
		if _, ok := payload[field]; ok {
			t.Fatalf("%s should not be forwarded: %#v", field, payload)
		}
	}
	if payload["stream"] != true {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
}

func TestHandleRunRuntimeImageEditAcceptsJSONImageURLArray(t *testing.T) {
	expander := &runInvokeExpanderStub{expansion: imageEditAppExpansion()}
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeInvokeExpander: expander,
		runtimeEngine:         executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/run", strings.NewReader(`{
		"input": "blend both references",
		"images": [
			{"image_url": "https://example.com/ref-a.png"},
			{"image_url": "https://example.com/ref-b.png"}
		]
	}`))
	req.Header.Set("Authorization", "Bearer rk_demo")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleRunRuntime(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(executor.input.Envelope.ClientBody, &payload); err != nil {
		t.Fatalf("unmarshal executor body: %v", err)
	}
	images, ok := payload["images"].([]any)
	if !ok {
		t.Fatalf("images = %#v, want []any", payload["images"])
	}
	if len(images) != 2 {
		t.Fatalf("images len = %d, want 2", len(images))
	}
	first := images[0].(map[string]any)["image_url"]
	second := images[1].(map[string]any)["image_url"]
	if first != "https://example.com/ref-a.png" || second != "https://example.com/ref-b.png" {
		t.Fatalf("images = %#v", images)
	}
}

func TestHandleRunRuntimeImageEditRejectsInvalidCurrentInputs(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "missing images", body: `{"input":"edit"}`},
		{name: "missing image url", body: `{"input":"edit","images":[{}]}`},
		{name: "multiple outputs", body: `{"input":"edit","images":[{"image_url":"https://example.com/ref.png"}],"n":3}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor := &runExecutorStub{}
			gw := &Gateway{
				runtimeInvokeExpander: &runInvokeExpanderStub{expansion: imageEditAppExpansion()},
				runtimeEngine:         executor,
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/run", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer rk_demo")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			gw.handleRunRuntime(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if executor.input.Request.Capability != "" {
				t.Fatalf("executor should not run: %+v", executor.input.Request)
			}
		})
	}
}

const runImageEditPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/lAAAAABJRU5ErkJggg=="

func mustRunImageEditPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(runImageEditPNGBase64)
	if err != nil {
		t.Fatalf("decode test PNG: %v", err)
	}
	return data
}

func TestExtractRunImagesStripsRevisedPrompt(t *testing.T) {
	got := extractRunImages([]byte(`{"data":[{"b64_json":"aGVsbG8=","revised_prompt":"hidden prompt"}]}`))
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if _, ok := got[0]["revised_prompt"]; ok {
		t.Fatalf("revised_prompt should be stripped: %+v", got[0])
	}
	if got[0]["b64_json"] != "aGVsbG8=" {
		t.Fatalf("b64_json = %#v, want preserved", got[0]["b64_json"])
	}
}

func TestExtractRunImagesPreservesAllGeneratedImages(t *testing.T) {
	got := extractRunImages([]byte(`{"data":[{"url":"https://example.test/first.png"},{"url":"https://example.test/second.png"}]}`))
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0]["url"] != "https://example.test/first.png" || got[1]["url"] != "https://example.test/second.png" {
		t.Fatalf("got = %#v", got)
	}
}

func TestExtractRunImagesPreservesPlatformURLMetadata(t *testing.T) {
	got := extractRunImages([]byte(`{"data":[{"url":"https://api.example.test/v1/files/content/capability","asset_ref":"media://b3b1d698-1ee0-4c91-b7aa-f15ad4a856b3","expires_at":"2026-07-16T14:00:00Z"}]}`))
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0]["asset_ref"] != "media://b3b1d698-1ee0-4c91-b7aa-f15ad4a856b3" {
		t.Fatalf("asset_ref = %#v", got[0]["asset_ref"])
	}
	if got[0]["expires_at"] != "2026-07-16T14:00:00Z" {
		t.Fatalf("expires_at = %#v", got[0]["expires_at"])
	}
}

type runInvokeExpanderStub struct {
	keyHash   string
	expansion coreruntime.InvokeExpansion
	err       error
}

func (s *runInvokeExpanderStub) ExpandByKeyHash(_ context.Context, keyHash string, req coreruntime.Request) (coreruntime.InvokeExpansion, error) {
	s.keyHash = keyHash
	if s.err != nil {
		return coreruntime.InvokeExpansion{}, s.err
	}
	out := s.expansion
	out.Request.RequestID = req.RequestID
	out.Request.TraceID = req.TraceID
	out.Request.ReceivedAt = req.ReceivedAt
	return out, nil
}

type runExecutorStub struct {
	input  coreruntime.ExecutionInput
	result coreruntime.Result
	err    error
}

func (s *runExecutorStub) Execute(_ context.Context, in coreruntime.ExecutionInput) (coreruntime.Result, error) {
	s.input = in
	if s.err != nil {
		return s.result, s.err
	}
	if in.Envelope.ResponseWriter != nil {
		in.Envelope.ResponseWriter.WriteHeader(http.StatusCreated)
	}
	return coreruntime.Result{StatusCode: http.StatusCreated, CreatedAt: time.Now()}, nil
}

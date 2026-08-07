package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/ai/core/catalog"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

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

func TestHandleRuntimeUsesRuntimeEngine(t *testing.T) {
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeEngine: executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-opus-4-1","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
	req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
		Subject: coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodAPIKey,
			RequestSource: coreidentity.RequestSourceAPIKey,
			Scope:         coreidentity.ScopeTenant,
			TenantID:      "tenant-a",
			APIKeyID:      "key-1",
			AllowedModels: []string{"gpt-5.4"},
			GroupID:       "group-1",
		},
	}))
	rec := httptest.NewRecorder()

	gw.handleRuntime(domain.CapabilityChat).ServeHTTP(rec, req)

	if executor.input.Subject.AuthMethod != coreidentity.AuthMethodAPIKey {
		t.Fatalf("subject auth method = %q, want api_key", executor.input.Subject.AuthMethod)
	}
	if executor.input.Request.ClientSurface != surface.AnthropicMessages {
		t.Fatalf("client surface = %q, want anthropic_messages", executor.input.Request.ClientSurface)
	}
	if executor.input.Request.RequestedModel != "claude-opus-4-1" {
		t.Fatalf("requested model = %q", executor.input.Request.RequestedModel)
	}
	if !executor.input.Request.Stream {
		t.Fatal("expected stream=true")
	}
	if executor.input.Request.ClientSurface != surface.AnthropicMessages {
		t.Fatalf("executor client surface = %q, want anthropic_messages", executor.input.Request.ClientSurface)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func TestHandleRuntimeInvalidJSONReturnsInvalidBody(t *testing.T) {
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeEngine: executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
		Subject: coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodAPIKey,
			RequestSource: coreidentity.RequestSourceAPIKey,
			Scope:         coreidentity.ScopeTenant,
			TenantID:      "tenant-a",
			APIKeyID:      "key-1",
		},
	}))
	rec := httptest.NewRecorder()

	gw.handleRuntime(domain.CapabilityChat).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_body") {
		t.Fatalf("body = %s, want invalid_body", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "invalid_service_tier") {
		t.Fatalf("body = %s, should not contain invalid_service_tier", rec.Body.String())
	}
}

func TestHandleRuntimeReturnsForbiddenWhenClientSurfaceIsNotAllowed(t *testing.T) {
	executor := &runExecutorStub{err: &serving.APIError{
		Status: http.StatusForbidden, Code: "endpoint_not_allowed", Message: "endpoint not allowed",
	}}
	gw := &Gateway{
		runtimeEngine: executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
		Subject: coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodAPIKey,
			RequestSource: coreidentity.RequestSourceAPIKey,
			Scope:         coreidentity.ScopeTenant,
			TenantID:      "tenant-a",
			APIKeyID:      "key-1",
		},
	}))
	rec := httptest.NewRecorder()

	gw.handleRuntime(domain.CapabilityChat).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "endpoint_not_allowed") {
		t.Fatalf("body = %s, want endpoint_not_allowed", rec.Body.String())
	}
}

func TestHandleEmbeddingsUsesRuntimeEngine(t *testing.T) {
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeEngine: executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding-3-large","input":"hello"}`))
	req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
		Subject: coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodAPIKey,
			RequestSource: coreidentity.RequestSourceAPIKey,
			Scope:         coreidentity.ScopeTenant,
			TenantID:      "tenant-a",
			APIKeyID:      "key-1",
		},
	}))
	rec := httptest.NewRecorder()

	gw.handleRuntime(domain.CapabilityEmbedding).ServeHTTP(rec, req)

	if executor.input.Request.ClientSurface != surface.OpenAIEmbeddings {
		t.Fatalf("client surface = %q, want openai_embeddings", executor.input.Request.ClientSurface)
	}
	if executor.input.Request.Capability != "embedding" {
		t.Fatalf("capability = %q, want embedding", executor.input.Request.Capability)
	}
}

func TestHandleGeminiRuntimeUsesRuntimeEngine(t *testing.T) {
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeEngine: executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/text-embedding-004:embedContent", strings.NewReader(`{"content":{"parts":[{"text":"hello"}]}}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("modelAction", "text-embedding-004:embedContent")
	req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
		Subject: coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodAPIKey,
			RequestSource: coreidentity.RequestSourceAPIKey,
			Scope:         coreidentity.ScopeTenant,
			TenantID:      "tenant-a",
			APIKeyID:      "key-1",
		},
	}))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()

	gw.handleGeminiRuntime(rec, req)

	if executor.input.Request.ClientSurface != surface.GeminiEmbeddings {
		t.Fatalf("client surface = %q, want gemini_embeddings", executor.input.Request.ClientSurface)
	}
	if executor.input.Request.RequestedModel != "text-embedding-004" {
		t.Fatalf("requested model = %q", executor.input.Request.RequestedModel)
	}
	if executor.input.Request.Capability != "embedding" {
		t.Fatalf("capability = %q, want embedding", executor.input.Request.Capability)
	}
}

func TestHandleGeminiImageRuntimeUsesRuntimeEngine(t *testing.T) {
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeEngine: executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.1-flash-image:generateContent", strings.NewReader(`{"generationConfig":{"responseModalities":["TEXT","IMAGE"]},"contents":[{"role":"user","parts":[{"text":"draw a poster"}]}]}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("modelAction", "gemini-3.1-flash-image:generateContent")
	req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
		Subject: coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodAPIKey,
			RequestSource: coreidentity.RequestSourceAPIKey,
			Scope:         coreidentity.ScopeTenant,
			TenantID:      "tenant-a",
			APIKeyID:      "key-1",
		},
	}))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()

	gw.handleGeminiRuntime(rec, req)

	if executor.input.Request.ClientSurface != surface.GeminiImages {
		t.Fatalf("client surface = %q, want gemini_images", executor.input.Request.ClientSurface)
	}
	if executor.input.Request.Capability != catalog.CapabilityImageGeneration {
		t.Fatalf("capability = %q, want image_generation", executor.input.Request.Capability)
	}
	if executor.input.Request.ClientSurface != surface.GeminiImages {
		t.Fatalf("executor client surface = %q, want gemini_images", executor.input.Request.ClientSurface)
	}
	if executor.input.Request.Capability != catalog.CapabilityImageGeneration {
		t.Fatalf("executor capability = %q, want image_generation", executor.input.Request.Capability)
	}
}

func TestHandleRuntimePreservesImageCountBeforeExecution(t *testing.T) {
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeEngine: executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-1","prompt":"poster","n":1,"size":"1024x1024"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
		Subject: coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodAPIKey,
			RequestSource: coreidentity.RequestSourceAPIKey,
			Scope:         coreidentity.ScopeTenant,
			TenantID:      "tenant-a",
			APIKeyID:      "key-1",
		},
	}))
	rec := httptest.NewRecorder()

	gw.handleRuntime(domain.CapabilityImage).ServeHTTP(rec, req)

	var payload map[string]any
	if err := json.Unmarshal(executor.input.Envelope.ClientBody, &payload); err != nil {
		t.Fatalf("unmarshal executor body: %v", err)
	}
	if payload["n"] != float64(1) {
		t.Fatalf("executor body n = %#v, want 1", payload["n"])
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func TestHandleRuntimeRejectsImageEditWithoutImageURLBeforeResolution(t *testing.T) {
	executor := &runExecutorStub{}
	gw := &Gateway{
		runtimeEngine: executor,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(`{
		"model":"gpt-image-1",
		"prompt":"edit this image",
		"images":[{}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
		Subject: coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodAPIKey,
			RequestSource: coreidentity.RequestSourceAPIKey,
			Scope:         coreidentity.ScopeTenant,
			TenantID:      "tenant-a",
			APIKeyID:      "key-1",
		},
	}))
	rec := httptest.NewRecorder()

	gw.handleRuntime(domain.CapabilityImage).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "images[0].image_url") {
		t.Fatalf("body = %s, want current image_url validation error", rec.Body.String())
	}
	if executor.input.Request.RequestID != "" {
		t.Fatal("resolver should not run for an invalid image edit request")
	}
	if executor.input.Request.RequestID != "" {
		t.Fatal("executor should not run for an invalid image edit request")
	}
}

func TestHandleRuntimeFiltersExternalImageQualityAndStyle(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "generation",
			path: "/v1/images/generations",
			body: `{"model":"gpt-image-1","prompt":"poster","quality":"high","style":"vivid"}`,
		},
		{
			name: "edit",
			path: "/v1/images/edits",
			body: `{"model":"gpt-image-1","prompt":"edit","images":[{"image_url":"https://example.com/ref.png"}],"quality":"high","style":"vivid"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			executor := &runExecutorStub{}
			gw := &Gateway{runtimeEngine: executor}
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), runtimeAuthContextKey{}, RuntimeAuth{
				Subject: coreidentity.Subject{
					AuthMethod: coreidentity.AuthMethodAPIKey, RequestSource: coreidentity.RequestSourceAPIKey,
					Scope: coreidentity.ScopeTenant, TenantID: "tenant-a", APIKeyID: "key-1",
				},
			}))
			rec := httptest.NewRecorder()

			gw.handleRuntime(domain.CapabilityImage).ServeHTTP(rec, req)

			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(executor.input.Envelope.ClientBody, &payload); err != nil {
				t.Fatalf("unmarshal executor body: %v", err)
			}
			for _, field := range []string{"quality", "style"} {
				if _, ok := payload[field]; ok {
					t.Fatalf("executor body must not include %s: %#v", field, payload)
				}
			}
		})
	}
}

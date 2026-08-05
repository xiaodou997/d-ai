package serving

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
)

type recordingClientRuntime struct {
	invocations []clientruntime.Invocation
	exchanges   []*clientruntime.Exchange
}

func (r *recordingClientRuntime) SupportsInvocation(domain.FixedProviderType, domain.UpstreamProtocol) bool {
	return true
}

func (r *recordingClientRuntime) Invoke(_ context.Context, in clientruntime.Invocation) (*clientruntime.Exchange, error) {
	r.invocations = append(r.invocations, in)
	exchange := r.exchanges[0]
	r.exchanges = r.exchanges[1:]
	return exchange, nil
}

func TestExecuteUsesClientRuntimeForCodexPool(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := codexPoolRequest(recorder)
	pool := &recordingOAuthPool{credentials: []*domain.OAuthCredential{codexCredential("credential-1")}}
	runtime := &recordingClientRuntime{exchanges: []*clientruntime.Exchange{
		clientExchange(http.StatusOK, clientruntime.CredentialEffectNone, `{"id":"resp_1","output":[]}`),
	}}
	legacyTransport := &sequenceTransport{}

	step := &ExecuteStep{
		Transport:     legacyTransport,
		ClientRuntime: runtime,
		Bridge:        testProtocolBridge{},
		OAuthPool:     pool,
		Budget:        RetryBudget{MaxAttempts: 1},
	}
	if err := step.Execute(context.Background(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if legacyTransport.calls != 0 {
		t.Fatalf("legacy transport calls = %d, want 0", legacyTransport.calls)
	}
	if len(runtime.invocations) != 1 {
		t.Fatalf("runtime invocations = %d", len(runtime.invocations))
	}
	invocation := runtime.invocations[0]
	if invocation.Provider != domain.FixedProviderCodex ||
		invocation.Model != "gpt-5.6-sol" ||
		invocation.Credential.ID != "credential-1" {
		t.Fatalf("invocation = %#v", invocation)
	}
	if len(request.Attempts) != 1 || request.Attempts[0].ProfileRevision != clientruntime.CodexProfileRevision {
		t.Fatalf("attempts = %#v", request.Attempts)
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"resp_1"`) {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestExecuteStreamsThroughClientRuntime(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := codexPoolRequest(recorder)
	request.IsStream = true
	request.Envelope.ClientBody = []byte(`{"model":"public-model","input":"hello","stream":true}`)
	pool := &recordingOAuthPool{credentials: []*domain.OAuthCredential{codexCredential("credential-1")}}
	exchange := clientExchange(
		http.StatusOK,
		clientruntime.CredentialEffectNone,
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n",
	)
	exchange.Response.Headers.Set("Content-Type", "text/event-stream")
	runtime := &recordingClientRuntime{exchanges: []*clientruntime.Exchange{exchange}}

	step := &ExecuteStep{
		Transport:     &sequenceTransport{},
		ClientRuntime: runtime,
		Bridge:        testProtocolBridge{},
		OAuthPool:     pool,
		Budget:        RetryBudget{MaxAttempts: 1},
	}
	if err := step.Execute(context.Background(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(runtime.invocations) != 1 || !runtime.invocations[0].Stream {
		t.Fatalf("runtime invocations = %#v", runtime.invocations)
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type = %q", recorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(recorder.Body.String(), `"response.completed"`) {
		t.Fatalf("stream response = %q", recorder.Body.String())
	}
	if len(request.Attempts) != 1 || request.Attempts[0].ProfileRevision != clientruntime.CodexProfileRevision {
		t.Fatalf("attempts = %#v", request.Attempts)
	}
}

func TestExecuteDoesNotPermanentlyInvalidateCodexForbidden(t *testing.T) {
	request := codexPoolRequest(httptest.NewRecorder())
	pool := &recordingOAuthPool{credentials: []*domain.OAuthCredential{codexCredential("credential-1")}}
	runtime := &recordingClientRuntime{exchanges: []*clientruntime.Exchange{
		clientExchange(http.StatusForbidden, clientruntime.CredentialEffectCooldown, `{"error":"account temporarily blocked"}`),
	}}
	step := &ExecuteStep{
		Transport:     &sequenceTransport{},
		ClientRuntime: runtime,
		Bridge:        testProtocolBridge{},
		OAuthPool:     pool,
		Budget:        RetryBudget{MaxAttempts: 1},
	}

	if err := step.Execute(context.Background(), request); err == nil {
		t.Fatal("Execute() error = nil, want upstream failure")
	}
	if len(pool.invalid) != 0 {
		t.Fatalf("invalid credentials = %v, want none", pool.invalid)
	}
	if len(pool.cooldowns) != 1 || pool.cooldowns[0] != "credential-1" {
		t.Fatalf("cooled down credentials = %v", pool.cooldowns)
	}
}

func TestExecuteRetriesRateLimitedRuntimeCredentialWithinPool(t *testing.T) {
	request := codexPoolRequest(httptest.NewRecorder())
	pool := &recordingOAuthPool{credentials: []*domain.OAuthCredential{
		codexCredential("credential-1"),
		codexCredential("credential-2"),
	}}
	runtime := &recordingClientRuntime{exchanges: []*clientruntime.Exchange{
		clientExchange(http.StatusTooManyRequests, clientruntime.CredentialEffectCooldown, `{"error":"quota"}`),
		clientExchange(http.StatusOK, clientruntime.CredentialEffectNone, `{"id":"resp_1","output":[]}`),
	}}
	step := &ExecuteStep{
		Transport:     &sequenceTransport{},
		ClientRuntime: runtime,
		Bridge:        testProtocolBridge{},
		OAuthPool:     pool,
		Budget:        RetryBudget{MaxAttempts: 2},
	}

	if err := step.Execute(context.Background(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if pool.selected != 2 || len(runtime.invocations) != 2 {
		t.Fatalf("selected = %d, invocations = %d", pool.selected, len(runtime.invocations))
	}
	if len(pool.cooldowns) != 1 || pool.cooldowns[0] != "credential-1" {
		t.Fatalf("cooled down credentials = %v", pool.cooldowns)
	}
	if runtime.invocations[1].Credential.ID != "credential-2" {
		t.Fatalf("second invocation credential = %q", runtime.invocations[1].Credential.ID)
	}
}

func TestExecuteInvalidatesCodexOnlyAfterRuntimeConfirms(t *testing.T) {
	request := codexPoolRequest(httptest.NewRecorder())
	pool := &recordingOAuthPool{credentials: []*domain.OAuthCredential{codexCredential("credential-1")}}
	runtime := &recordingClientRuntime{exchanges: []*clientruntime.Exchange{
		clientExchange(http.StatusUnauthorized, clientruntime.CredentialEffectInvalidate, `{"error":"invalid token"}`),
	}}
	step := &ExecuteStep{
		Transport:     &sequenceTransport{},
		ClientRuntime: runtime,
		Bridge:        testProtocolBridge{},
		OAuthPool:     pool,
		Budget:        RetryBudget{MaxAttempts: 1},
	}

	if err := step.Execute(context.Background(), request); err == nil {
		t.Fatal("Execute() error = nil, want upstream failure")
	}
	if len(pool.invalid) != 1 || pool.invalid[0] != "credential-1" {
		t.Fatalf("invalid credentials = %v", pool.invalid)
	}
}

func codexPoolRequest(writer http.ResponseWriter) *Request {
	candidate := &domain.RouteCandidate{
		RouteID:           "codex-pool-route",
		PoolID:            "pool-1",
		ModelCode:         "public-model",
		UpstreamModel:     "gpt-5.6-sol",
		PoolUpstreamModel: "gpt-5.6-sol",
		Protocol:          domain.ProtocolOpenAIResponses,
		FixedProviderType: domain.FixedProviderCodex,
		OAuthStrategy:     "round_robin",
		Timeouts:          domain.DefaultRouteTimeouts(domain.CapabilityChat),
	}
	return &Request{
		Envelope: &RequestEnvelope{
			W:          writer,
			ClientBody: []byte(`{"model":"public-model","input":"hello"}`),
		},
		RequestID:      "request-1",
		ModelCode:      "public-model",
		RequestedModel: "public-model",
		CapabilityType: domain.CapabilityChat,
		ClientProtocol: domain.ProtocolOpenAIResponses,
		ClientPath:     "/v1/responses",
		Candidates:     []*domain.RouteCandidate{candidate},
		UsedCandidates: map[string]bool{},
	}
}

func codexCredential(id string) *domain.OAuthCredential {
	return &domain.OAuthCredential{
		ID:           id,
		PoolID:       "pool-1",
		ProviderType: domain.FixedProviderCodex,
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		AuthMetadata: map[string]any{"account_id": "account-1"},
		Status:       domain.OAuthCredentialActive,
	}
}

func clientExchange(status int, effect clientruntime.CredentialEffect, body string) *clientruntime.Exchange {
	return &clientruntime.Exchange{
		Response: &clientruntime.WireResponse{
			StatusCode: status,
			Headers:    make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		},
		Trace: clientruntime.Trace{
			ProfileRevision:  clientruntime.CodexProfileRevision,
			RequestURL:       "https://chatgpt.com/backend-api/codex/responses",
			CredentialID:     "credential-1",
			ProviderCalls:    1,
			CredentialEffect: effect,
		},
	}
}

package clientruntime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

type transportFunc func(context.Context, *WireRequest) (*WireResponse, error)

func (f transportFunc) Do(ctx context.Context, req *WireRequest) (*WireResponse, error) {
	return f(ctx, req)
}

type refresherFunc func(context.Context, string) (Credential, error)

func (f refresherFunc) Refresh(ctx context.Context, credentialID string) (Credential, error) {
	return f(ctx, credentialID)
}

func TestCodexProfileBuildsOneVersionedContract(t *testing.T) {
	var captured *WireRequest
	runtime := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		captured = cloneWireRequest(req)
		return response(http.StatusOK, `{"id":"resp_1"}`), nil
	}), nil)

	exchange, err := runtime.Invoke(context.Background(), Invocation{
		Provider:    domain.FixedProviderCodex,
		Operation:   OperationResponses,
		Protocol:    domain.ProtocolOpenAIResponses,
		Model:       "gpt-5.6-sol",
		Body:        []byte(`{"model":"old","input":"hello","reasoning":{"effort":"high"},"temperature":0.2,"unknown":{"keep":true}}`),
		Stream:      true,
		RequestID:   "request-1",
		AffinityKey: "conversation-1",
		Credential: Credential{
			ID:          "credential-1",
			PoolID:      "pool-1",
			AccessToken: "access-token",
			AccountID:   "account-1",
			Metadata:    map[string]any{"installation_id": "install-1"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	defer exchange.Response.Body.Close()

	if exchange.Trace.ProfileRevision != CodexProfileRevision {
		t.Fatalf("profile revision = %q", exchange.Trace.ProfileRevision)
	}
	if captured == nil {
		t.Fatal("transport did not receive a request")
	}
	if captured.URL != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("URL = %q", captured.URL)
	}
	for key, want := range map[string]string{
		"Authorization":           "Bearer access-token",
		"User-Agent":              codexUserAgent,
		"originator":              codexOriginator,
		"version":                 codexClientVersion,
		"OpenAI-Beta":             "responses=experimental",
		"chatgpt-account-id":      "account-1",
		"x-codex-installation-id": "install-1",
		"Accept":                  "text/event-stream",
	} {
		if got := captured.Headers[key]; got != want {
			t.Errorf("header %s = %q, want %q", key, got, want)
		}
	}
	if captured.Headers["session_id"] == "" || captured.Headers["conversation_id"] != captured.Headers["session_id"] {
		t.Fatalf("session identity headers = %#v", captured.Headers)
	}

	var body map[string]any
	if err := json.Unmarshal(captured.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "gpt-5.6-sol" || body["store"] != false || body["stream"] != true {
		t.Fatalf("contract body = %#v", body)
	}
	if _, ok := body["temperature"]; ok {
		t.Error("temperature must be removed")
	}
	if _, ok := body["unknown"]; !ok {
		t.Error("unknown fields must be preserved")
	}
	input, ok := body["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v", body["input"])
	}
	include, ok := body["include"].([]any)
	if !ok || len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v", body["include"])
	}
}

func TestCodexCompactUsesDedicatedEndpointAndProjection(t *testing.T) {
	var captured *WireRequest
	runtime := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		captured = cloneWireRequest(req)
		return response(http.StatusOK, `{}`), nil
	}), nil)

	exchange, err := runtime.Invoke(context.Background(), Invocation{
		Provider:   domain.FixedProviderCodex,
		Operation:  OperationCompact,
		Protocol:   domain.ProtocolOpenAIResponses,
		Model:      "gpt-5.6-sol",
		Body:       []byte(`{"input":[],"stream":true,"store":false,"metadata":{"drop":true},"service_tier":"priority"}`),
		Credential: Credential{ID: "credential-1", AccessToken: "token"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	defer exchange.Response.Body.Close()
	if captured.URL != "https://chatgpt.com/backend-api/codex/responses/compact" {
		t.Fatalf("URL = %q", captured.URL)
	}
	if captured.Headers["Accept"] != "application/json" {
		t.Fatalf("Accept = %q", captured.Headers["Accept"])
	}
	var body map[string]any
	if err := json.Unmarshal(captured.Body, &body); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"stream", "store", "metadata"} {
		if _, ok := body[removed]; ok {
			t.Errorf("%s must be removed from compact body", removed)
		}
	}
	if body["parallel_tool_calls"] != true {
		t.Fatalf("parallel_tool_calls = %#v", body["parallel_tool_calls"])
	}
}

func TestCodexInspectionFetchesLiveVersionedModelCards(t *testing.T) {
	var captured *WireRequest
	runtime := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		captured = cloneWireRequest(req)
		response := response(http.StatusOK, `{
			"models":[
				{"slug":"gpt-5.6-terra","use_responses_lite":true,"supports_parallel_tool_calls":true,"base_instructions":"do not retain"},
				{"id":"gpt-5.6-sol","default_reasoning_level":"high"},
				{"slug":"gpt-5.6-sol"}
			]
		}`)
		response.Headers.Set("ETag", `"manifest-v1"`)
		return response, nil
	}), nil)

	snapshot, err := runtime.Inspect(context.Background(), Inspection{
		Provider: domain.FixedProviderCodex,
		Want:     InspectModels,
		Credential: Credential{
			ID:          "credential-1",
			AccessToken: "access-token",
			AccountID:   "account-1",
		},
		IfNoneMatch: `"manifest-v0"`,
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if captured.Method != http.MethodGet ||
		captured.URL != "https://chatgpt.com/backend-api/codex/models?client_version=0.144.1" {
		t.Fatalf("inspection request = %#v", captured)
	}
	if captured.Headers["If-None-Match"] != `"manifest-v0"` ||
		captured.Headers["User-Agent"] != codexUserAgent ||
		captured.Headers["chatgpt-account-id"] != "account-1" {
		t.Fatalf("inspection headers = %#v", captured.Headers)
	}
	if snapshot.ProfileRevision != CodexProfileRevision ||
		snapshot.Source != "live" ||
		snapshot.ETag != `"manifest-v1"` {
		t.Fatalf("inspection snapshot = %#v", snapshot)
	}
	if len(snapshot.Models) != 2 ||
		snapshot.Models[0].ID != "gpt-5.6-sol" ||
		snapshot.Models[1].ID != "gpt-5.6-terra" {
		t.Fatalf("models = %#v", snapshot.Models)
	}
	if _, retained := snapshot.Models[1].Capabilities["base_instructions"]; retained {
		t.Fatal("large prompt fields must not be retained in the model capability snapshot")
	}
	if snapshot.Models[1].Capabilities["use_responses_lite"] != true {
		t.Fatalf("capabilities = %#v", snapshot.Models[1].Capabilities)
	}
}

func TestClaudeProfileBuildsCurrentClaudeCodeContract(t *testing.T) {
	var captured *WireRequest
	runtime := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		captured = cloneWireRequest(req)
		return response(http.StatusOK, `{}`), nil
	}), nil)

	exchange, err := runtime.Invoke(context.Background(), Invocation{
		Provider:              domain.FixedProviderClaudeOAuth,
		Operation:             OperationMessages,
		Protocol:              domain.ProtocolAnthropicMessages,
		Model:                 "claude-opus-5",
		Body:                  []byte(`{"model":"old","stream":true,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"unsigned"},{"type":"text","text":"kept"}]}]}`),
		IncomingAnthropicBeta: "context-1m-2025-08-07,effort-2025-11-24",
		Credential:            Credential{ID: "credential-1", AccessToken: "token"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	defer exchange.Response.Body.Close()
	if captured.URL != "https://api.anthropic.com/v1/messages" {
		t.Fatalf("URL = %q", captured.URL)
	}
	for key, want := range map[string]string{
		"User-Agent":                  claudeUserAgent,
		"X-App":                       "cli",
		"X-Stainless-Lang":            "js",
		"X-Stainless-Package-Version": "0.94.0",
		"X-Stainless-Runtime-Version": "v24.3.0",
		"anthropic-version":           "2023-06-01",
	} {
		if got := captured.Headers[key]; got != want {
			t.Errorf("header %s = %q, want %q", key, got, want)
		}
	}
	for _, beta := range append(append([]string{}, claudeRequiredBetas...), "context-1m-2025-08-07") {
		if !strings.Contains(captured.Headers["anthropic-beta"], beta) {
			t.Errorf("anthropic-beta %q is missing %q", captured.Headers["anthropic-beta"], beta)
		}
	}
	if strings.Count(captured.Headers["anthropic-beta"], "effort-2025-11-24") != 1 {
		t.Fatalf("anthropic-beta did not deduplicate: %q", captured.Headers["anthropic-beta"])
	}
	var body map[string]any
	if err := json.Unmarshal(captured.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "claude-opus-5" || body["stream"] != false {
		t.Fatalf("contract body = %#v", body)
	}
	content := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["type"] != "text" {
		t.Fatalf("invalid thinking history was not removed: %#v", content)
	}
}

func TestGeminiCLIProfileBuildsVersionedCodeAssistEnvelope(t *testing.T) {
	var captured *WireRequest
	runtime := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		captured = cloneWireRequest(req)
		return response(http.StatusOK, `{}`), nil
	}), nil)

	exchange, err := runtime.Invoke(context.Background(), Invocation{
		Provider:  domain.FixedProviderGeminiCLI,
		Operation: OperationGenerateContent,
		Protocol:  domain.ProtocolGeminiGenerate,
		Model:     "gemini-3.1-pro-preview",
		Body:      []byte(`{"model":"old","stream":false,"contents":[{"role":"user","parts":[{"text":"hello"}]}],"safetySettings":[{"category":"x"}]}`),
		Stream:    true,
		RequestID: "request-1",
		Credential: Credential{
			ID:          "credential-1",
			AccessToken: "token",
			Metadata: map[string]any{
				"gemini_cli": map[string]any{
					"project_id": "project-1",
					"session_id": "session-1",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	defer exchange.Response.Body.Close()
	if captured.URL != "https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse" ||
		captured.Headers["User-Agent"] != geminiCLIUserAgent {
		t.Fatalf("wire request = %#v", captured)
	}
	var body map[string]any
	if err := json.Unmarshal(captured.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "gemini-3.1-pro-preview" ||
		body["project"] != "project-1" ||
		body["user_prompt_id"] != "request-1" {
		t.Fatalf("envelope = %#v", body)
	}
	inner := body["request"].(map[string]any)
	if inner["session_id"] != "session-1" {
		t.Fatalf("inner session = %#v", inner)
	}
	if _, retained := inner["safetySettings"]; !retained {
		t.Fatalf("Gemini CLI semantic safety settings must be preserved: %#v", inner)
	}
	if _, retained := inner["stream"]; retained {
		t.Fatalf("inner stream flag must be removed: %#v", inner)
	}
}

func TestAntigravityProfileBuildsNativeIdentityAndSafeEnvelope(t *testing.T) {
	var captured *WireRequest
	runtime := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		captured = cloneWireRequest(req)
		return response(http.StatusOK, `{}`), nil
	}), nil)

	exchange, err := runtime.Invoke(context.Background(), Invocation{
		Provider:  domain.FixedProviderAntigravity,
		Operation: OperationGenerateContent,
		Protocol:  domain.ProtocolGeminiGenerate,
		Model:     "gemini-3.1-pro-high",
		Body:      []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"tools":[{"functionDeclarations":[]}],"safetySettings":[{"category":"x"}]}`),
		RequestID: "request-1",
		Credential: Credential{
			ID:          "credential-1",
			AccessToken: "token",
			Metadata: map[string]any{
				"antigravity": map[string]any{
					"project_id":     "project-1",
					"client_version": "1.0.16",
					"session_id":     "session-1",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	defer exchange.Response.Body.Close()
	if captured.URL != "https://cloudcode-pa.googleapis.com/v1internal:generateContent" {
		t.Fatalf("URL = %q", captured.URL)
	}
	for key, want := range map[string]string{
		"User-Agent":         antigravityUserAgent,
		"x-client-name":      "antigravity",
		"x-client-version":   "1.0.16",
		"x-goog-api-client":  antigravityGoogleAPIClient,
		"x-vscode-sessionid": "session-1",
		"Authorization":      "Bearer token",
	} {
		if got := captured.Headers[key]; got != want {
			t.Errorf("header %s = %q, want %q", key, got, want)
		}
	}
	var body map[string]any
	if err := json.Unmarshal(captured.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["project"] != "project-1" ||
		body["requestId"] != "request-1" ||
		body["requestType"] != "agent" ||
		body["userAgent"] != antigravityUserAgent {
		t.Fatalf("envelope = %#v", body)
	}
	inner := body["request"].(map[string]any)
	if _, retained := inner["safetySettings"]; retained {
		t.Fatalf("safety settings must be removed: %#v", inner)
	}
	if _, retained := inner["tools"]; !retained {
		t.Fatalf("tools must be preserved by the current Antigravity profile: %#v", inner)
	}
}

func TestRuntimeAdvertisesOnlyRegisteredProviderProtocols(t *testing.T) {
	runtime := New(transportFunc(func(context.Context, *WireRequest) (*WireResponse, error) {
		return response(http.StatusOK, `{}`), nil
	}), nil)
	for _, supported := range []struct {
		provider domain.FixedProviderType
		protocol domain.UpstreamProtocol
	}{
		{domain.FixedProviderCodex, domain.ProtocolOpenAIResponses},
		{domain.FixedProviderClaudeOAuth, domain.ProtocolAnthropicMessages},
		{domain.FixedProviderGeminiCLI, domain.ProtocolGeminiGenerate},
		{domain.FixedProviderAntigravity, domain.ProtocolGeminiGenerate},
	} {
		if !runtime.SupportsInvocation(supported.provider, supported.protocol) {
			t.Errorf("expected support for %s/%s", supported.provider, supported.protocol)
		}
	}
	if runtime.SupportsInvocation(domain.FixedProviderClaudeOAuth, domain.ProtocolOpenAIChat) {
		t.Fatal("Claude profile must not advertise OpenAI chat wire support")
	}
}

func TestRuntimeTurnsRetryAfterIntoCredentialCooldown(t *testing.T) {
	fixedNow := time.Unix(1_000, 0).UTC()
	originalNow := now
	now = func() time.Time { return fixedNow }
	t.Cleanup(func() { now = originalNow })

	runtime := New(transportFunc(func(context.Context, *WireRequest) (*WireResponse, error) {
		result := response(http.StatusTooManyRequests, `{"error":"quota"}`)
		result.Headers.Set("Retry-After", "120")
		return result, nil
	}), nil)
	exchange, err := runtime.Invoke(context.Background(), baseInvocation())
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	defer exchange.Response.Body.Close()
	if exchange.Trace.CredentialEffect != CredentialEffectCooldown ||
		!exchange.Trace.CooldownUntil.Equal(fixedNow.Add(2*time.Minute)) {
		t.Fatalf("trace = %#v", exchange.Trace)
	}
}

func TestRuntimeRefreshesOnceAndReplaysUnauthorizedCredential(t *testing.T) {
	var calls atomic.Int32
	runtime := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		calls.Add(1)
		if req.Headers["Authorization"] == "Bearer old-token" {
			return response(http.StatusUnauthorized, `{"error":"expired"}`), nil
		}
		return response(http.StatusOK, `{"id":"resp_1"}`), nil
	}), refresherFunc(func(_ context.Context, credentialID string) (Credential, error) {
		if credentialID != "credential-1" {
			t.Fatalf("credentialID = %q", credentialID)
		}
		return Credential{
			ID:           "credential-1",
			AccessToken:  "new-token",
			RefreshToken: "refresh-token",
		}, nil
	}))

	exchange, err := runtime.Invoke(context.Background(), baseInvocation())
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	defer exchange.Response.Body.Close()
	if exchange.Response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", exchange.Response.StatusCode)
	}
	if calls.Load() != 2 || exchange.Trace.ProviderCalls != 2 || exchange.Trace.RefreshCalls != 1 {
		t.Fatalf("trace = %#v, calls = %d", exchange.Trace, calls.Load())
	}
	if exchange.Trace.CredentialEffect != CredentialEffectRefreshed {
		t.Fatalf("credential effect = %q", exchange.Trace.CredentialEffect)
	}
}

func TestRuntimeCoalescesConcurrentCredentialRefresh(t *testing.T) {
	const concurrency = 8
	oldCalls := make(chan struct{}, concurrency)
	releaseRefresh := make(chan struct{})
	var refreshCalls atomic.Int32

	runtime := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		if req.Headers["Authorization"] == "Bearer old-token" {
			oldCalls <- struct{}{}
			return response(http.StatusUnauthorized, `{}`), nil
		}
		return response(http.StatusOK, `{}`), nil
	}), refresherFunc(func(_ context.Context, _ string) (Credential, error) {
		refreshCalls.Add(1)
		<-releaseRefresh
		return Credential{
			ID:           "credential-1",
			AccessToken:  "new-token",
			RefreshToken: "refresh-token",
		}, nil
	}))

	var wg sync.WaitGroup
	errs := make(chan error, concurrency)
	wg.Add(concurrency)
	for range concurrency {
		go func() {
			defer wg.Done()
			exchange, err := runtime.Invoke(context.Background(), baseInvocation())
			if exchange != nil && exchange.Response != nil && exchange.Response.Body != nil {
				exchange.Response.Body.Close()
			}
			errs <- err
		}()
	}
	for range concurrency {
		<-oldCalls
	}
	close(releaseRefresh)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Invoke() error = %v", err)
		}
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls.Load())
	}
}

func baseInvocation() Invocation {
	return Invocation{
		Provider:  domain.FixedProviderCodex,
		Operation: OperationResponses,
		Protocol:  domain.ProtocolOpenAIResponses,
		Model:     "gpt-5.6-sol",
		Body:      []byte(`{"input":[]}`),
		Credential: Credential{
			ID:           "credential-1",
			AccessToken:  "old-token",
			RefreshToken: "refresh-token",
		},
	}
}

func response(status int, body string) *WireResponse {
	return &WireResponse{
		StatusCode: status,
		Headers:    make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func cloneWireRequest(req *WireRequest) *WireRequest {
	headers := make(map[string]string, len(req.Headers))
	for key, value := range req.Headers {
		headers[key] = value
	}
	return &WireRequest{
		Method:   req.Method,
		URL:      req.URL,
		Headers:  headers,
		Body:     append([]byte(nil), req.Body...),
		Protocol: req.Protocol,
	}
}

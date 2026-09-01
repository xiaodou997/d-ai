package serving

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/upstreamcompat"
)

func TestLogUpstreamFailureIncludesSafeRequestSummary(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	undo := zap.ReplaceGlobals(zap.New(core))
	defer undo()

	req := &Request{RequestID: "req-1", ModelCode: "gpt-image-2"}
	cand := &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIImages, ProviderCode: "img2-313"}
	logUpstreamFailure(context.Background(), req, cand, "https://images.example.test/v1/images/edits", 400, 10, nil,
		`{"error":{"message":"images[].image_url is required"}}`,
		"application/json",
		`{"images":[{"image_url":"data:image/png;base64,AAAA...<truncated 4000 bytes>"}]}`)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("error log count = %d, want 1", len(entries))
	}
	requestSummary, ok := entries[0].ContextMap()["upstream_request"].(string)
	if !ok || !strings.Contains(requestSummary, `"images"`) || !strings.Contains(requestSummary, "<truncated") {
		t.Fatalf("upstream_request = %#v", entries[0].ContextMap()["upstream_request"])
	}
	if entries[0].ContextMap()["upstream_content_type"] != "application/json" {
		t.Fatalf("upstream_content_type = %#v", entries[0].ContextMap()["upstream_content_type"])
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name   string
		cand   *domain.RouteCandidate
		stream bool
		want   string
	}{
		{
			name: "openai chat default path",
			cand: &domain.RouteCandidate{
				BaseURL:  "https://api.openai.com",
				Protocol: domain.ProtocolOpenAIChat,
			},
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "trailing slash trimmed and custom path",
			cand: &domain.RouteCandidate{
				BaseURL:     "https://example.com/api/",
				RequestPath: "/foo/bar",
				Protocol:    domain.ProtocolOpenAIChat,
			},
			want: "https://example.com/api/foo/bar",
		},
		{
			name: "path without leading slash gets one",
			cand: &domain.RouteCandidate{
				BaseURL:     "https://example.com",
				RequestPath: "rel/path",
				Protocol:    domain.ProtocolOpenAIChat,
			},
			want: "https://example.com/rel/path",
		},
		{
			name: "gemini model placeholder substitution",
			cand: &domain.RouteCandidate{
				BaseURL:       "https://generativelanguage.googleapis.com",
				UpstreamModel: "gemini-2.0-flash",
				Protocol:      domain.ProtocolGeminiGenerate,
			},
			want: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := upstreamcompat.BuildURL(tc.cand, upstreamcompat.RequestMeta{IsStream: tc.stream})
			if err != nil {
				t.Fatalf("BuildURL err = %v", err)
			}
			if tc.want != "" && got != tc.want {
				t.Fatalf("BuildURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildURLImageEditsDefaultPath(t *testing.T) {
	cand := &domain.RouteCandidate{
		BaseURL:  "https://api.openai.com",
		Protocol: domain.ProtocolOpenAIImages,
	}
	got, err := upstreamcompat.BuildURL(cand, upstreamcompat.RequestMeta{ClientPath: "/v1/images/edits"})
	if err != nil {
		t.Fatalf("BuildURL err = %v", err)
	}
	if want := "https://api.openai.com/v1/images/edits"; got != want {
		t.Fatalf("BuildURL = %q, want %q", got, want)
	}
}

func TestBuildURLRejectsUnsupportedProtocol(t *testing.T) {
	cand := &domain.RouteCandidate{
		BaseURL:  "https://api.openai.com",
		Protocol: domain.UpstreamProtocol("unsupported_protocol"),
	}
	if _, err := upstreamcompat.BuildURL(cand, upstreamcompat.RequestMeta{}); err == nil {
		t.Fatal("expected BuildURL to reject unsupported protocol")
	}
}

func TestBuildHeadersAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		cand     *domain.RouteCandidate
		req      *Request
		wantAuth string
		wantKey  string // header name expected to carry the key
	}{
		{
			name: "openai uses Bearer",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolOpenAIChat,
				APIKeyCiphertext: "sk-abc",
			},
			req:      &Request{},
			wantAuth: "Bearer sk-abc",
			wantKey:  "Authorization",
		},
		{
			name: "anthropic uses compatible auth headers + version",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolAnthropicMessages,
				APIKeyCiphertext: "sk-ant",
			},
			req:     &Request{},
			wantKey: "x-api-key",
		},
		{
			name: "gemini exposes x-gemini-api-key for transport to consume",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolGeminiGenerate,
				APIKeyCiphertext: "AIza-test",
			},
			req:     &Request{},
			wantKey: "x-gemini-api-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := upstreamcompat.BuildHeaders(tc.cand, upstreamcompat.RequestMeta{IsStream: tc.req != nil && tc.req.IsStream})
			if h["Content-Type"] != "application/json" {
				t.Fatalf("missing Content-Type")
			}
			switch tc.wantKey {
			case "Authorization":
				if h["Authorization"] != tc.wantAuth {
					t.Fatalf("Authorization = %q, want %q", h["Authorization"], tc.wantAuth)
				}
			case "x-api-key":
				if h["x-api-key"] != tc.cand.APIKeyCiphertext {
					t.Fatalf("x-api-key not set to API key")
				}
				if h["Authorization"] != "Bearer "+tc.cand.APIKeyCiphertext {
					t.Fatalf("Authorization = %q, want compatible Bearer credential", h["Authorization"])
				}
				if h["anthropic-version"] == "" {
					t.Fatalf("anthropic-version missing")
				}
			case "x-gemini-api-key":
				if h["x-gemini-api-key"] != tc.cand.APIKeyCiphertext {
					t.Fatalf("x-gemini-api-key = %q, want %q", h["x-gemini-api-key"], tc.cand.APIKeyCiphertext)
				}
				if _, ok := h["Authorization"]; ok {
					t.Fatalf("Gemini API-key path must not set Authorization header")
				}
			}
		})
	}
}

func TestBuildHeadersStreamAccept(t *testing.T) {
	cand := &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIChat, APIKeyCiphertext: "k"}
	h := upstreamcompat.BuildHeaders(cand, upstreamcompat.RequestMeta{IsStream: true})
	if h["Accept"] != "text/event-stream" {
		t.Fatalf("stream request must set Accept text/event-stream, got %q", h["Accept"])
	}
}

func TestUpstreamStatusToGateway(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{429, http.StatusTooManyRequests},
		{500, http.StatusBadGateway},
		{502, http.StatusBadGateway},
		{503, http.StatusBadGateway},
		{401, http.StatusBadGateway},
		{403, http.StatusBadGateway},
		{400, 400},
		{404, 404},
		{409, 409},
	}
	for _, tc := range tests {
		if got := upstreamStatusToGateway(tc.in); got != tc.want {
			t.Errorf("upstreamStatusToGateway(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestExecuteFailsOverToDifferentCandidateAfter502(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusBadGateway, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"route one failed"}}`))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	recorder := httptest.NewRecorder()
	req := &Request{
		Envelope: &RequestEnvelope{
			W:          recorder,
			ClientBody: []byte(`{"model":"public-model","messages":[{"role":"user","content":"hello"}]}`),
		},
		ModelCode:      "public-model",
		RequestedModel: "public-model",
		CapabilityType: domain.CapabilityChat,
		ClientProtocol: domain.ProtocolOpenAIChat,
		Candidates: []*domain.RouteCandidate{
			{RouteID: "route-1", EndpointID: "upstream-1", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
			{RouteID: "route-2", EndpointID: "upstream-2", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		},
		UsedCandidates: map[string]bool{},
	}
	step := &ExecuteStep{
		Transport: transport,
		Bridge:    testProtocolBridge{},
		Budget:    RetryBudget{MaxAttempts: 2},
	}

	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := attemptRouteIDs(req.Attempts), []string{"route-1", "route-2"}; !slices.Equal(got, want) {
		t.Fatalf("attempt routes = %v, want %v", got, want)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want 200", recorder.Code)
	}
}

// A slot must be returned on every exit path, not just the successful one.
// Failing over without releasing would let one request pin capacity on every
// account it touched for the whole lease TTL.
func TestExecuteReleasesConcurrencySlotOnSuccessAndOnFailover(t *testing.T) {
	limit := 10
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusBadGateway, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"route one failed"}}`))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	limiter := &recordingUpstreamLimiter{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "upstream-1", UpstreamConcurrencyLimit: &limit, ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "route-2", EndpointID: "upstream-2", UpstreamConcurrencyLimit: &limit, ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{
		Transport:       transport,
		UpstreamLimiter: limiter,
		Bridge:          testProtocolBridge{},
		Budget:          RetryBudget{MaxAttempts: 2},
	}

	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := limiter.accounts, []string{"upstream-1", "upstream-2"}; !slices.Equal(got, want) {
		t.Fatalf("acquired accounts = %v, want %v", got, want)
	}
	if got, want := limiter.released, []string{"upstream-1", "upstream-2"}; !slices.Equal(got, want) {
		t.Fatalf("released accounts = %v, want %v", got, want)
	}
}

// Pool routes carry no account identity, so they must not consume account slots.
func TestExecuteDoesNotClaimConcurrencySlotForUnlimitedAccount(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	limiter := &recordingUpstreamLimiter{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "upstream-1", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{
		Transport:       transport,
		UpstreamLimiter: limiter,
		Bridge:          testProtocolBridge{},
		Budget:          RetryBudget{MaxAttempts: 2},
	}

	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(limiter.accounts) != 0 {
		t.Fatalf("acquired accounts = %v, want none", limiter.accounts)
	}
}

func TestExecuteSkipsAccountWhoseConcurrencyIsExhausted(t *testing.T) {
	limit := 10
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	limiter := &recordingUpstreamLimiter{errorsByAccount: map[string]error{
		"upstream-1": ErrUpstreamConcurrencyExceeded,
	}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "upstream-1", UpstreamConcurrencyLimit: &limit, ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "route-2", EndpointID: "upstream-2", UpstreamConcurrencyLimit: &limit, ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{
		Transport:       transport,
		UpstreamLimiter: limiter,
		Bridge:          testProtocolBridge{},
		Budget:          RetryBudget{MaxAttempts: 2},
	}

	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := attemptRouteIDs(req.Attempts), []string{"route-2"}; !slices.Equal(got, want) {
		t.Fatalf("attempt routes = %v, want %v", got, want)
	}
	if got, want := limiter.accounts, []string{"upstream-1", "upstream-2"}; !slices.Equal(got, want) {
		t.Fatalf("limited accounts = %v, want %v", got, want)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
}

func TestExecuteReturns429WhenAllUpstreamConcurrencyIsExhausted(t *testing.T) {
	limit := 10
	limiter := &recordingUpstreamLimiter{errorsByAccount: map[string]error{
		"upstream-1": ErrUpstreamConcurrencyExceeded,
		"upstream-2": ErrUpstreamConcurrencyExceeded,
	}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "upstream-1", UpstreamConcurrencyLimit: &limit, ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "route-2", EndpointID: "upstream-2", UpstreamConcurrencyLimit: &limit, ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{
		Transport:       &sequenceTransport{},
		UpstreamLimiter: limiter,
		Bridge:          testProtocolBridge{},
		Budget:          RetryBudget{MaxAttempts: 2},
	}

	err := step.Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTooManyRequests || apiErr.Code != "upstream_capacity_exhausted" {
		t.Fatalf("Execute error = %#v, want 429 upstream_capacity_exhausted", err)
	}
	if len(req.Attempts) != 0 {
		t.Fatalf("upstream attempts = %d, want 0", len(req.Attempts))
	}
}

func TestExecuteDoesNotAddRoutingMetadataToUpstreamRequest(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "upstream-1", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})

	if err := (&ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}}).Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("upstream requests = %d, want 1", len(transport.requests))
	}
	upstreamRequest := transport.requests[0]
	if strings.Contains(strings.ToLower(upstreamRequest.URL), "weight") {
		t.Fatalf("upstream URL must not include routing metadata: %s", upstreamRequest.URL)
	}
	for name, values := range upstreamRequest.Headers {
		if strings.Contains(strings.ToLower(name), "weight") || strings.Contains(strings.ToLower(values), "weight") {
			t.Fatalf("upstream headers must not include routing metadata: %v", upstreamRequest.Headers)
		}
	}
	if strings.Contains(strings.ToLower(string(upstreamRequest.Body)), "weight") {
		t.Fatalf("upstream body must not include routing metadata: %s", upstreamRequest.Body)
	}
}

func TestExecuteFailsOverAfterEmptySuccessfulResponse(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusNoContent, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(""))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	recorder := httptest.NewRecorder()
	req := executeTestRequest(recorder, []*domain.RouteCandidate{
		{RouteID: "empty-route", EndpointID: "empty-account", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "good-route", EndpointID: "good-account", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	health := &recordingHealth{}

	if err := (&ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, Health: health}).Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := attemptRouteIDs(req.Attempts), []string{"empty-route", "good-route"}; !slices.Equal(got, want) {
		t.Fatalf("attempt routes = %v, want %v", got, want)
	}
	if !slices.Contains(health.failures, "empty-account") {
		t.Fatalf("health failures = %v, want empty-account", health.failures)
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"id":"ok"`) {
		t.Fatalf("response = %d %q, want fallback success", recorder.Code, recorder.Body.String())
	}
}

func TestExecuteDoesNotRetrySamePhysicalTargetThroughAnotherGroup(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusBadGateway, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"primary failed"}`))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "primary-binding", GroupRank: 0, EndpointID: "shared-account", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "fallback-alias", GroupRank: 1, EndpointID: "shared-account", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "fallback-physical", GroupRank: 1, EndpointID: "other-account", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})

	if err := (&ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}}).Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := attemptRouteIDs(req.Attempts), []string{"primary-binding", "fallback-physical"}; !slices.Equal(got, want) {
		t.Fatalf("attempt routes = %v, want %v; a physical target must be attempted once per request", got, want)
	}
}

func TestExecuteKeepsGroupFailoverBoundary(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusBadGateway, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"primary failed"}`))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "primary-group", GroupRank: 0, EndpointID: "upstream-primary", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "fallback-group", GroupRank: 1, EndpointID: "upstream-fallback", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := attemptRouteIDs(req.Attempts), []string{"primary-group", "fallback-group"}; !slices.Equal(got, want) {
		t.Fatalf("attempt routes = %v, want %v", got, want)
	}
}

func TestPickCandidateHonorsStickyBeforeStructuralTiers(t *testing.T) {
	sticky := &domain.RouteCandidate{RouteID: "sticky", GroupRank: 2, ModelCode: "model"}
	req := &Request{
		Candidate: sticky,
		Candidates: []*domain.RouteCandidate{
			sticky,
			{RouteID: "primary", GroupRank: 0, ModelCode: "model"},
		},
		UsedCandidates: map[string]bool{},
		StickyHit:      true,
	}
	got, _ := (&ExecuteStep{}).pickCandidate(context.Background(), req)
	if got != sticky {
		t.Fatalf("picked %q, want sticky route", got.RouteID)
	}
}

func TestExecuteDirect401FailsOverToNextRoute(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusUnauthorized, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"bad key"}`))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	health := &recordingHealth{}
	accountState := &recordingDirectAccountState{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "bad-key", GroupRank: 0, EndpointID: "account-bad", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "good-key", GroupRank: 0, EndpointID: "account-good", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, Health: health, AccountState: accountState}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := attemptRouteIDs(req.Attempts), []string{"bad-key", "good-key"}; !slices.Equal(got, want) {
		t.Fatalf("attempt routes = %v, want %v", got, want)
	}
	if !slices.Contains(health.failures, "account-bad") {
		t.Fatalf("health failures = %v, want rejected direct account", health.failures)
	}
	if got, want := accountState.invalidIDs, []string{"account-bad"}; !slices.Equal(got, want) {
		t.Fatalf("invalid accounts = %v, want %v", got, want)
	}
}

func TestExecuteDirect403FailsOverToNextRoute(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusForbidden, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"permission denied"}`))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	health := &recordingHealth{}
	accountState := &recordingDirectAccountState{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "forbidden", EndpointID: "account-forbidden", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "fallback", EndpointID: "account-good", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})

	if err := (&ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, Health: health, AccountState: accountState}).Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := attemptRouteIDs(req.Attempts), []string{"forbidden", "fallback"}; !slices.Equal(got, want) {
		t.Fatalf("attempt routes = %v, want %v", got, want)
	}
	if !slices.Contains(health.failures, "account-forbidden") {
		t.Fatalf("health failures = %v, want account-forbidden", health.failures)
	}
	if got, want := accountState.invalidIDs, []string{"account-forbidden"}; !slices.Equal(got, want) {
		t.Fatalf("invalid accounts = %v, want %v", got, want)
	}
}

func TestExecuteOAuth401SwapsOnceThenFailsOverRoute(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusUnauthorized, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"credential one rejected"}`))},
		{StatusCode: http.StatusUnauthorized, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"credential two rejected"}`))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	pool := &recordingOAuthPool{credentials: []*domain.OAuthCredential{{ID: "cred-1"}, {ID: "cred-2"}}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "pool-route", GroupRank: 0, PoolID: "pool-1", ModelCode: "public-model", PoolUpstreamModel: "upstream-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "direct-route", GroupRank: 1, EndpointID: "account-good", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, OAuthPool: pool}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := attemptRouteIDs(req.Attempts), []string{"pool-route", "pool-route", "direct-route"}; !slices.Equal(got, want) {
		t.Fatalf("attempt routes = %v, want %v", got, want)
	}
	if got, want := pool.invalid, []string{"cred-1", "cred-2"}; !slices.Equal(got, want) {
		t.Fatalf("invalid credentials = %v, want %v", got, want)
	}
}

func TestExecutePool502TripsPoolWithoutPoisoningCredential(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusBadGateway, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"provider unavailable"}`))},
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	pool := &recordingOAuthPool{credentials: []*domain.OAuthCredential{{ID: "cred-1"}}}
	health := &recordingHealth{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "pool-route", GroupRank: 0, PoolID: "pool-1", ModelCode: "public-model", PoolUpstreamModel: "upstream-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "direct-route", GroupRank: 1, EndpointID: "account-good", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, OAuthPool: pool, Health: health}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !slices.Contains(health.failures, "pool-1") {
		t.Fatalf("health failures = %v, want pool-1", health.failures)
	}
	if len(pool.invalid) != 0 {
		t.Fatalf("credential was invalidated by pool 502: %v", pool.invalid)
	}
}

func TestExecuteReturnsNoHealthyRouteWithoutCallingTransport(t *testing.T) {
	transport := &sequenceTransport{}
	health := &recordingHealth{blocked: map[string]bool{
		"account-1": true,
		"account-2": true,
	}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "account-1", ModelCode: "public-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "route-2", EndpointID: "account-2", ModelCode: "public-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	err := (&ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, Health: health}).Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable || apiErr.Code != "no_healthy_route" {
		t.Fatalf("Execute error = %v, want 503 no_healthy_route", err)
	}
	if transport.calls != 0 {
		t.Fatalf("transport calls = %d, want 0", transport.calls)
	}
}

func TestExecuteReportsAllRoutesFailedAfterEveryCandidate(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: http.StatusBadGateway, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"one"}`))},
		{StatusCode: http.StatusServiceUnavailable, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"two"}`))},
	}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "account-1", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "route-2", EndpointID: "account-2", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	err := (&ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}}).Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadGateway || apiErr.Code != "all_routes_failed" {
		t.Fatalf("Execute error = %v, want 502 all_routes_failed", err)
	}
	if transport.calls != 2 {
		t.Fatalf("transport calls = %d, want 2", transport.calls)
	}
}

func TestExecuteReportsRetryBudgetOnlyWhenAttemptCapLeavesCandidates(t *testing.T) {
	responses := make([]*UpstreamResponse, maxUpstreamAttempts)
	for i := range responses {
		responses[i] = &UpstreamResponse{
			StatusCode: http.StatusBadGateway,
			Headers:    http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"error":"failed"}`)),
		}
	}
	candidates := make([]*domain.RouteCandidate, maxUpstreamAttempts+1)
	for i := range candidates {
		candidates[i] = &domain.RouteCandidate{
			RouteID:       fmt.Sprintf("route-%d", i),
			EndpointID:    fmt.Sprintf("account-%d", i),
			ModelCode:     "public-model",
			UpstreamModel: "upstream-model",
			Protocol:      domain.ProtocolOpenAIChat,
			Timeouts:      domain.DefaultRouteTimeouts(domain.CapabilityChat),
		}
	}
	transport := &sequenceTransport{responses: responses}
	req := executeTestRequest(httptest.NewRecorder(), candidates)
	err := (&ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}}).Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "retry_budget_exhausted" {
		t.Fatalf("Execute error = %v, want retry_budget_exhausted", err)
	}
	if transport.calls != maxUpstreamAttempts {
		t.Fatalf("transport calls = %d, want %d", transport.calls, maxUpstreamAttempts)
	}
}

func TestExecuteClientCancellationDoesNotRetryOrTripHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := &cancelingTransport{cancel: cancel}
	health := &recordingHealth{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "account-1", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "route-2", EndpointID: "account-2", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	err := (&ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, Health: health}).Execute(ctx, req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "client_disconnected" {
		t.Fatalf("Execute error = %v, want client_disconnected", err)
	}
	if transport.calls != 1 || len(req.Attempts) != 1 || req.Attempts[0].Outcome != ResultCanceled {
		t.Fatalf("calls=%d attempts=%+v, want one canceled attempt", transport.calls, req.Attempts)
	}
	if len(health.failures) != 0 {
		t.Fatalf("health failures = %v, want none for client cancellation", health.failures)
	}
	if got, want := health.released, []string{"account-1"}; !slices.Equal(got, want) {
		t.Fatalf("released probes = %v, want %v", got, want)
	}
}

func TestExecuteDoesNotClaimProbeBeforeCredentialPreparation(t *testing.T) {
	health := &recordingHealth{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "pool-route", PoolID: "pool-1", ModelCode: "public-model", PoolUpstreamModel: "upstream-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{
		Transport: &sequenceTransport{},
		Bridge:    testProtocolBridge{},
		Health:    health,
		OAuthPool: &recordingOAuthPool{},
	}

	err := step.Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "no_credential" {
		t.Fatalf("Execute error = %v, want no_credential", err)
	}
	if len(health.checked) != 0 {
		t.Fatalf("health probe was claimed before credential preparation: %v", health.checked)
	}
	if strings.Contains(apiErr.Message, "pool-1") || strings.Contains(apiErr.Message, "no credentials") {
		t.Fatalf("credential selection detail leaked to client: %q", apiErr.Message)
	}
}

func TestExecuteStopsAtTotalRetryDeadline(t *testing.T) {
	health := &recordingHealth{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "route-1", EndpointID: "account-1", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
		{RouteID: "route-2", EndpointID: "account-2", ModelCode: "public-model", UpstreamModel: "upstream-model", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)},
	})
	step := &ExecuteStep{
		Transport: waitForCancellationTransport{},
		Bridge:    testProtocolBridge{},
		Health:    health,
		Budget:    RetryBudget{MaxAttempts: 2, MaxElapsed: 20 * time.Millisecond},
	}

	err := step.Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "retry_deadline_exhausted" || apiErr.Status != http.StatusGatewayTimeout {
		t.Fatalf("Execute error = %v, want 504 retry_deadline_exhausted", err)
	}
	if len(req.Attempts) != 1 {
		t.Fatalf("attempts = %d, want one in-flight attempt stopped by total deadline", len(req.Attempts))
	}
	if len(health.failures) != 0 {
		t.Fatalf("total request deadline must not penalize target health: %v", health.failures)
	}
}

func TestExecuteDoesNotRetryAmbiguousImageResponseHeaderTimeout(t *testing.T) {
	transport := &countingCancellationTransport{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{
		{RouteID: "image-1", EndpointID: "account-1", ModelCode: "image-model", UpstreamModel: "image-model", Protocol: domain.ProtocolOpenAIImages, Timeouts: domain.RouteTimeouts{ResponseHeader: 20 * time.Millisecond, FirstByte: time.Second, Idle: time.Second, MaxDuration: time.Second}},
		{RouteID: "image-2", EndpointID: "account-2", ModelCode: "image-model", UpstreamModel: "image-model", Protocol: domain.ProtocolOpenAIImages, Timeouts: domain.RouteTimeouts{ResponseHeader: 20 * time.Millisecond, FirstByte: time.Second, Idle: time.Second, MaxDuration: time.Second}},
	})
	req.CapabilityType = domain.CapabilityImage
	req.ClientProtocol = domain.ProtocolOpenAIImages
	req.ModelCode = "image-model"
	req.RequestedModel = "image-model"
	req.Envelope.ClientBody = []byte(`{"model":"image-model","prompt":"draw a test image"}`)
	step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, Budget: RetryBudget{MaxAttempts: 2, MaxElapsed: time.Second}}

	err := step.Execute(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "upstream_timeout" || apiErr.Status != http.StatusGatewayTimeout {
		t.Fatalf("Execute error = %v, want 504 upstream_timeout", err)
	}
	if transport.calls != 1 || len(req.Attempts) != 1 {
		t.Fatalf("transport calls/attempts = %d/%d, want 1/1", transport.calls, len(req.Attempts))
	}
}

func executeTestRequest(recorder *httptest.ResponseRecorder, candidates []*domain.RouteCandidate) *Request {
	return &Request{
		Envelope: &RequestEnvelope{
			W:          recorder,
			ClientBody: []byte(`{"model":"public-model","messages":[{"role":"user","content":"hello"}]}`),
		},
		ModelCode:      "public-model",
		RequestedModel: "public-model",
		CapabilityType: domain.CapabilityChat,
		ClientProtocol: domain.ProtocolOpenAIChat,
		Candidates:     candidates,
		UsedCandidates: map[string]bool{},
	}
}

type recordingHealth struct {
	failures []string
	success  []string
	released []string
	checked  []string
	blocked  map[string]bool
}

func (h *recordingHealth) RecordSuccess(targetID string, _ routing.TargetKind) {
	h.success = append(h.success, targetID)
}
func (h *recordingHealth) RecordFailure(targetID string, _ routing.TargetKind) {
	h.failures = append(h.failures, targetID)
}
func (h *recordingHealth) IsBlocked(targetID string, _ time.Duration) bool {
	h.checked = append(h.checked, targetID)
	return h.blocked[targetID]
}
func (h *recordingHealth) ReleaseProbe(targetID string) {
	h.released = append(h.released, targetID)
}
func (*recordingHealth) StateOf(string) routing.HealthState { return routing.StateClosed }
func (*recordingHealth) StatesOf(targetIDs []string) map[string]routing.HealthState {
	out := make(map[string]routing.HealthState, len(targetIDs))
	for _, targetID := range targetIDs {
		out[targetID] = routing.StateClosed
	}
	return out
}
func (*recordingHealth) Snapshot() []routing.HealthRecord { return nil }

type recordingOAuthPool struct {
	credentials []*domain.OAuthCredential
	selected    int
	invalid     []string
	cooldowns   []string
	cooldownAt  []time.Time
}

type recordingDirectAccountState struct {
	invalidIDs []string
	reasons    []string
}

type recordingUpstreamLimiter struct {
	accounts        []string
	released        []string
	errorsByAccount map[string]error
}

func (l *recordingUpstreamLimiter) Acquire(_ context.Context, accountID, _ string, _ int, _ time.Duration) (UpstreamSlot, error) {
	l.accounts = append(l.accounts, accountID)
	if err := l.errorsByAccount[accountID]; err != nil {
		return nil, err
	}
	return &recordingUpstreamSlot{limiter: l, accountID: accountID}, nil
}

type recordingUpstreamSlot struct {
	limiter   *recordingUpstreamLimiter
	accountID string
}

func (s *recordingUpstreamSlot) Release(context.Context) {
	s.limiter.released = append(s.limiter.released, s.accountID)
}

func (s *recordingDirectAccountState) MarkAccountInvalid(_ context.Context, accountID, reason string) (domain.UpstreamAccount, error) {
	s.invalidIDs = append(s.invalidIDs, accountID)
	s.reasons = append(s.reasons, reason)
	return domain.UpstreamAccount{ID: accountID, Status: domain.UpstreamAccountStatusInvalid}, nil
}

func (p *recordingOAuthPool) SelectCredentialFromPool(context.Context, string, string) (*domain.OAuthCredential, error) {
	if p.selected >= len(p.credentials) {
		return nil, errors.New("no credential")
	}
	credential := p.credentials[p.selected]
	p.selected++
	return credential, nil
}
func (p *recordingOAuthPool) MarkInvalid(_ context.Context, credentialID, _ string) error {
	p.invalid = append(p.invalid, credentialID)
	return nil
}
func (p *recordingOAuthPool) MarkCooldown(_ context.Context, credentialID string, until time.Time) error {
	p.cooldowns = append(p.cooldowns, credentialID)
	p.cooldownAt = append(p.cooldownAt, until)
	return nil
}
func (*recordingOAuthPool) RecordSuccess(context.Context, string) {}

type sequenceTransport struct {
	responses []*UpstreamResponse
	requests  []*UpstreamRequest
	calls     int
}

type cancelingTransport struct {
	cancel context.CancelFunc
	calls  int
}

type waitForCancellationTransport struct{}

type countingCancellationTransport struct{ calls int }

func (waitForCancellationTransport) Do(ctx context.Context, _ *UpstreamRequest) (*UpstreamResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (t *countingCancellationTransport) Do(ctx context.Context, _ *UpstreamRequest) (*UpstreamResponse, error) {
	t.calls++
	<-ctx.Done()
	return nil, ctx.Err()
}

func (t *cancelingTransport) Do(ctx context.Context, _ *UpstreamRequest) (*UpstreamResponse, error) {
	t.calls++
	t.cancel()
	return nil, ctx.Err()
}

func (t *sequenceTransport) Do(_ context.Context, req *UpstreamRequest) (*UpstreamResponse, error) {
	if t.calls >= len(t.responses) {
		return nil, errors.New("unexpected transport call")
	}
	t.requests = append(t.requests, &UpstreamRequest{
		Method:   req.Method,
		URL:      req.URL,
		Headers:  req.Headers,
		Body:     append([]byte(nil), req.Body...),
		Protocol: req.Protocol,
	})
	resp := t.responses[t.calls]
	t.calls++
	return resp, nil
}

func attemptRouteIDs(attempts []AttemptRecord) []string {
	ids := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		ids = append(ids, attempt.RouteID)
	}
	return ids
}

// ============================================================================
// Streaming execution — delayed commit / precommit failover / postcommit frame
// ============================================================================

func TestPayloadIsError(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`{"error":{"message":"x"}}`, true},
		{`{"type":"error","error":{}}`, true},
		{`  {"error":{"code":1}}`, true},
		{`{"choices":[{"delta":{}}]}`, false},
		{`{"type":"message","role":"assistant"}`, false},
		{`{"id":"x","object":"response"}`, false},
		// OpenAI Responses success bodies always carry an explicit null error.
		{`{"id":"x","object":"response","status":"completed","error":null,"incomplete_details":null}`, false},
		{`{"id":"x","error":{}}`, false},
		{`{"id":"x","error":""}`, false},
		{`[DONE]`, false},
		{``, false},
		{`not json`, false},
	}
	for _, c := range cases {
		if got := payloadIsError([]byte(c.in)); got != c.want {
			t.Errorf("payloadIsError(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestReadSSELine(t *testing.T) {
	r := bufio.NewReaderSize(strings.NewReader("a\nbb\n\nccc"), 8)
	want := []string{"a\n", "bb\n", "\n", "ccc"}
	for i, w := range want {
		line, err := readSSELine(r, maxSSELineBytes)
		if string(line) != w {
			t.Fatalf("line %d = %q, want %q", i, line, w)
		}
		if i < len(want)-1 && err != nil {
			t.Fatalf("line %d unexpected err %v", i, err)
		}
		if i == len(want)-1 && err != io.EOF {
			t.Fatalf("last line err = %v, want io.EOF", err)
		}
	}
}

// errAfterReader yields data then fails — simulates a mid-stream upstream drop.
type errAfterReader struct {
	data []byte
	pos  int
}

func (e *errAfterReader) Read(p []byte) (int, error) {
	if e.pos >= len(e.data) {
		return 0, errors.New("simulated upstream connection drop")
	}
	n := copy(p, e.data[e.pos:])
	e.pos += n
	return n, nil
}

func (e *errAfterReader) Close() error { return nil }

type delayedFirstByteReader struct {
	ctx   context.Context
	delay time.Duration
	data  []byte
	sent  bool
}

func (r *delayedFirstByteReader) Read(p []byte) (int, error) {
	if !r.sent {
		select {
		case <-time.After(r.delay):
			r.sent = true
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		}
	}
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *delayedFirstByteReader) Close() error { return nil }

func genTestDC() *deadlineController {
	return newDeadlineController(context.Background(), domain.RouteTimeouts{
		ResponseHeader: time.Hour, FirstByte: time.Hour, Idle: time.Hour, MaxDuration: time.Hour,
	})
}

func newStreamReq() *Request {
	return &Request{
		IsStream:       true,
		ClientProtocol: domain.ProtocolOpenAIChat,
		ModelCode:      "public-model",
		Candidate:      &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIChat, RouteID: "r1", UpstreamModel: "upstream-model"},
		Attempts:       []AttemptRecord{{RouteID: "r1"}},
	}
}

func sseResp(body string) *UpstreamResponse {
	return &UpstreamResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newExecuteStepForTests() *ExecuteStep {
	return &ExecuteStep{Bridge: testProtocolBridge{}}
}

func TestExecuteStreamSuccess(t *testing.T) {
	body := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	if err := newExecuteStepForTests().executeStream(dc, req, sseResp(body), w, time.Now()); err != nil {
		t.Fatalf("executeStream err = %v, want nil", err)
	}
	if req.HTTPStatus != http.StatusOK {
		t.Fatalf("HTTPStatus = %d, want 200 (committed)", req.HTTPStatus)
	}
	if req.RequestStatus != domain.RequestSuccess {
		t.Fatalf("RequestStatus = %v, want success", req.RequestStatus)
	}
	if !strings.Contains(w.Body.String(), "hi") || !strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatalf("stream body not forwarded verbatim: %q", w.Body.String())
	}
}

func TestExecuteStreamPreservesClaudeTerminalUsageSnapshot(t *testing.T) {
	body := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-6","usage":{"input_tokens":0,"output_tokens":0}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"\u6211\u662f Claude\uff0c"}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":544,"output_tokens":55}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()
	req.ClientProtocol = domain.ProtocolAnthropicMessages
	req.Candidate.Protocol = domain.ProtocolAnthropicMessages
	req.UpstreamBodySize = 162

	if err := newExecuteStepForTests().executeStream(dc, req, sseResp(body), w, time.Now()); err != nil {
		t.Fatalf("executeStream err = %v, want nil", err)
	}
	if req.TokenUsage.PromptTokens != 544 || req.TokenUsage.CompletionTokens != 55 {
		t.Fatalf("usage = %+v, want 544 input / 55 output", req.TokenUsage)
	}
	if req.TokenCountSource != domain.TokenUsageSourceUpstream {
		t.Fatalf("token source = %q, want upstream", req.TokenCountSource)
	}
}

func TestExecuteStreamSanitizesResponsesModel(t *testing.T) {
	body := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"model\":\"gpt-5.4-mini-2026-03-17\"}}\n\n" +
		"data: [DONE]\n\n"
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()
	req.ClientProtocol = domain.ProtocolOpenAIResponses
	req.ModelCode = "gpt-5.4-mini"
	req.Candidate.Protocol = domain.ProtocolOpenAIResponses
	req.Candidate.UpstreamModel = "gpt-5.4-mini"

	if err := newExecuteStepForTests().executeStream(dc, req, sseResp(body), w, time.Now()); err != nil {
		t.Fatalf("executeStream err = %v, want nil", err)
	}
	got := w.Body.String()
	if !strings.Contains(got, `"model":"gpt-5.4-mini"`) {
		t.Fatalf("stream did not sanitize response.model: %q", got)
	}
	if strings.Contains(got, "gpt-5.4-mini-2026-03-17") {
		t.Fatalf("stream leaked upstream versioned model: %q", got)
	}
}

func TestExecuteStreamPrecommitEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	err := newExecuteStepForTests().executeStream(dc, req, sseResp(""), w, time.Now())
	if !isPrecommitError(err) {
		t.Fatalf("empty stream: err = %v, want *precommitError", err)
	}
	if req.HTTPStatus != 0 {
		t.Fatalf("HTTPStatus must stay 0 (uncommitted), got %d", req.HTTPStatus)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("nothing must be written to the client before commit, got %q", w.Body.String())
	}
}

func TestExecuteStreamPrecommitErrorBody(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	err := newExecuteStepForTests().executeStream(dc, req, sseResp(`{"error":{"message":"bad gateway"}}`), w, time.Now())
	if !isPrecommitError(err) {
		t.Fatalf("200+error body: err = %v, want *precommitError", err)
	}
	if req.HTTPStatus != 0 {
		t.Fatalf("HTTPStatus must stay 0, got %d", req.HTTPStatus)
	}
}

func TestExecuteStreamPrecommitFirstFrameError(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	body := "data: {\"error\":{\"message\":\"upstream boom\"}}\n\n"
	err := newExecuteStepForTests().executeStream(dc, req, sseResp(body), w, time.Now())
	if !isPrecommitError(err) {
		t.Fatalf("first-frame error: err = %v, want *precommitError", err)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("an error first frame must not be forwarded, got %q", w.Body.String())
	}
}

func TestExecuteStreamPostcommitErrorFrame(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
	resp := &UpstreamResponse{StatusCode: http.StatusOK, Headers: http.Header{}, Body: &errAfterReader{data: body}}
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	err := newExecuteStepForTests().executeStream(dc, req, resp, w, time.Now())
	var pc *postcommitError
	if !errors.As(err, &pc) {
		t.Fatalf("mid-stream drop: err = %v, want *postcommitError", err)
	}
	if req.HTTPStatus != http.StatusOK {
		t.Fatalf("stream should have committed before the drop, HTTPStatus = %d", req.HTTPStatus)
	}
	if req.RequestStatus != domain.RequestFailed {
		t.Fatalf("RequestStatus = %v, want failed", req.RequestStatus)
	}
	if !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("client must receive a protocol error frame, got %q", w.Body.String())
	}
}

func TestExecuteSyncFirstByteTimeout(t *testing.T) {
	dc := newDeadlineController(context.Background(), domain.RouteTimeouts{
		ResponseHeader: time.Hour, FirstByte: 20 * time.Millisecond, Idle: time.Hour, MaxDuration: time.Hour,
	})
	defer dc.stop()
	dc.headersReceived()

	resp := &UpstreamResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{},
		Body: &delayedFirstByteReader{
			ctx:   dc.ctx,
			delay: 200 * time.Millisecond,
			data:  []byte(`{"ok":true}`),
		},
	}
	req := &Request{Candidate: &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIChat}}
	w := httptest.NewRecorder()

	err := newExecuteStepForTests().executeSync(dc, req, resp, w)
	var pre *precommitError
	if !errors.As(err, &pre) {
		t.Fatalf("executeSync err = %v, want *precommitError", err)
	}
	if !errors.Is(pre.Unwrap(), ErrFirstByteTimeout) {
		t.Fatalf("precommit cause = %v, want ErrFirstByteTimeout", pre.Unwrap())
	}
	if req.HTTPStatus != 0 {
		t.Fatalf("sync response must stay uncommitted on first-byte timeout, got HTTPStatus=%d", req.HTTPStatus)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("nothing must be written before sync first-byte commit, got %q", w.Body.String())
	}
}

func TestExecuteSyncSanitizesModel(t *testing.T) {
	dc := genTestDC()
	defer dc.stop()
	dc.headersReceived()

	resp := &UpstreamResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"id":"x","model":"upstream-model","choices":[]}`)),
	}
	req := &Request{
		ModelCode: "public-model",
		Candidate: &domain.RouteCandidate{
			Protocol:      domain.ProtocolOpenAIChat,
			UpstreamModel: "upstream-model",
		},
	}
	w := httptest.NewRecorder()

	if err := newExecuteStepForTests().executeSync(dc, req, resp, w); err != nil {
		t.Fatalf("executeSync err = %v, want nil", err)
	}
	if !strings.Contains(w.Body.String(), `"model":"public-model"`) {
		t.Fatalf("sync response did not sanitize model: %q", w.Body.String())
	}
}

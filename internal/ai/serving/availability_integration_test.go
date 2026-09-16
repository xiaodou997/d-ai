package serving

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

func servingAvailability(t *testing.T) (*routing.RedisAvailability, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { client.Close() })
	return routing.NewRedisAvailability(client), server
}
func runtimeCandidate(endpoint string) *domain.RouteCandidate {
	return &domain.RouteCandidate{RouteID: "group-target", CandidateID: endpoint, AccountID: "account", EndpointID: endpoint, GroupID: "group", ModelCode: "public-model", UpstreamModel: "model", Operation: "openai_chat", Protocol: domain.ProtocolOpenAIChat, Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)}
}
func TestSingleTargetCoolsAndRecoversWithoutSyntheticCalls(t *testing.T) {
	a, server := servingAvailability(t)
	ctx := context.Background()
	now := time.Now().UTC()
	server.SetTime(now)
	transport := &sequenceTransport{}
	for i := 0; i < 5; i++ {
		transport.responses = append(transport.responses, &UpstreamResponse{StatusCode: 503, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error"}}`))})
	}
	for i := 0; i < 2; i++ {
		transport.responses = append(transport.responses, jsonResp(`{"choices":[{"message":{"content":"ok"}}]}`))
	}
	step := &ExecuteStep{Availability: a, Transport: transport, Bridge: testProtocolBridge{}}
	c := runtimeCandidate("endpoint")
	for i := 0; i < 5; i++ {
		if step.Execute(ctx, executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{c})) == nil {
			t.Fatal("failure expected")
		}
	}
	w := httptest.NewRecorder()
	err := step.Execute(ctx, executeTestRequest(w, []*domain.RouteCandidate{c}))
	var api *APIError
	if !errors.As(err, &api) || api.Status != 503 || w.Header().Get("Retry-After") == "" || transport.calls != 5 {
		t.Fatalf("cooldown err=%v header=%v calls=%d", err, w.Header(), transport.calls)
	}
	server.SetTime(now.Add(31 * time.Second))
	states, err := a.List(ctx, "direct_upstream", "account")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Scope.Kind == "model" && s.Phase != routing.Recovering {
			t.Fatal(s)
		}
	}
	if transport.calls != 5 {
		t.Fatal("reading expired state sent a model probe")
	}
	for i := 0; i < 2; i++ {
		if err = step.Execute(ctx, executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{c})); err != nil {
			t.Fatal(err)
		}
	}
	states, err = a.List(ctx, "direct_upstream", "account")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Phase != routing.Available {
			t.Fatal(s)
		}
	}
	if transport.calls != 7 {
		t.Fatalf("actual sends=%d", transport.calls)
	}
}

func TestEndpointFallbackKeepsIndependentExecutionAndPricingIdentity(t *testing.T) {
	a, _ := servingAvailability(t)
	first, second := runtimeCandidate("endpoint-one"), runtimeCandidate("endpoint-two")
	tr := &sequenceTransport{responses: []*UpstreamResponse{{StatusCode: 502, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{}}`))}, jsonResp(`{"choices":[{"message":{"content":"ok"}}]}`)}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{first, second})
	req.BillingSnapshots = map[string]domain.BillingSnapshot{first.Key(): {EffectiveUserMultiplier: 2}, second.Key(): {EffectiveUserMultiplier: 3}}
	if err := (&ExecuteStep{Availability: a, Transport: tr, Bridge: testProtocolBridge{}}).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(req.Attempts) != 2 || req.Attempts[0].CandidateID == req.Attempts[1].CandidateID || req.Attempts[0].RouteID != req.Attempts[1].RouteID {
		t.Fatalf("attempts=%+v", req.Attempts)
	}
	if req.Attempts[0].PricingSnapshot.EffectiveUserMultiplier != 2 || req.Attempts[1].PricingSnapshot.EffectiveUserMultiplier != 3 {
		t.Fatal("endpoint price snapshots collided")
	}
}

func TestRateLimitDoesNotDelayHealthyFallbackOrPenalizeServiceCounter(t *testing.T) {
	a, _ := servingAvailability(t)
	first, second := runtimeCandidate("one"), runtimeCandidate("two")
	tr := &sequenceTransport{responses: []*UpstreamResponse{{StatusCode: 429, Headers: http.Header{"Retry-After": []string{"300"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"rate_limit_exceeded"}}`))}, jsonResp(`{"choices":[{"message":{"content":"ok"}}]}`)}}
	started := time.Now()
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{first, second})
	if err := (&ExecuteStep{Availability: a, Transport: tr, Bridge: testProtocolBridge{}}).Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("healthy route waited on another route's Retry-After")
	}
	states, err := a.Read(context.Background(), candidateScopes(first, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Scope.Kind == "rate_limit" && s.Phase != routing.Cooling {
			t.Fatal(s)
		}
		if s.Scope.Kind == "model" && s.ConsecutiveFailures != 0 {
			t.Fatal("429 mixed into ordinary failures")
		}
	}
}

func TestProviderErrorCodesScopeAuthenticationAndUserErrors(t *testing.T) {
	for _, tt := range []struct {
		status int
		body   string
		want   ResultStatus
	}{
		{403, `{"error":{"code":"model_access_denied"}}`, ResultModelError},
		{403, `{"error":{"code":"invalid_api_key"}}`, ResultUnauthorized},
		{404, `{"error":{"code":"model_not_found"}}`, ResultModelError},
		{400, `{"error":{"code":"invalid_parameter"}}`, ResultClientError},
		{429, `{"error":{"code":"rate_limit_exceeded"}}`, ResultRateLimited},
	} {
		if got := ClassifyResponse(tt.status, nil, tt.body, nil); got.Status != tt.want {
			t.Fatalf("%s: %+v", tt.body, got)
		}
	}
}

type expandedTestPool struct{ *recordingOAuthPool }

func (*expandedTestPool) ListCredentialCandidates(context.Context, string) ([]string, int64, error) {
	return []string{"healthy", "trial"}, 0, nil
}
func (*expandedTestPool) SelectPinnedCredential(_ context.Context, _ string, id string) (*domain.OAuthCredential, error) {
	return &domain.OAuthCredential{ID: id, AccessToken: "test"}, nil
}
func TestPoolCredentialsShareOneRecoveryRotationWithDirectCandidates(t *testing.T) {
	a, _ := servingAvailability(t)
	ctx := context.Background()
	pool := &expandedTestPool{&recordingOAuthPool{}}
	c := runtimeCandidate("")
	c.PoolID = "pool"
	c.AccountID = ""
	c.RouteID = "pool-route"
	c.CandidateID = "pool-path"
	scopes := candidateScopes(c, &domain.OAuthCredential{ID: "trial"})
	for _, scope := range scopes {
		if scope.Kind == "model" {
			if err := a.Reset(ctx, []routing.FaultScope{scope}); err != nil {
				t.Fatal(err)
			}
		}
	}
	step := &ExecuteStep{Availability: a, OAuthPool: pool}
	trials := 0
	for i := 0; i < 20; i++ {
		req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{c, runtimeCandidate("direct")})
		req.StickyHit = true
		req.StickyBinding = &routing.StickyBinding{TargetKind: "credential", RouteID: c.RouteID, CredentialID: "healthy"}
		req.Candidate = c
		if err := step.expandCredentialCandidates(ctx, req); err != nil {
			t.Fatal(err)
		}
		got, _ := step.pickCandidate(ctx, req)
		if got == nil {
			t.Fatal(req.SelectionError)
		}
		if got.CredentialID == "trial" {
			trials++
		}
	}
	if trials != 2 {
		t.Fatalf("trial credential selected %d times out of 20; recovery rotations must not multiply", trials)
	}
}

func TestGenericForbiddenDoesNotImmediatelyCoolAllSubsequentRequests(t *testing.T) {
	availability, _ := servingAvailability(t)
	candidate := runtimeCandidate("denied-once")
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		{StatusCode: 403, Headers: http.Header{}, Body: io.NopCloser(strings.NewReader("error code: 1010"))},
		jsonResp(`{"choices":[{"message":{"content":"OK"}}]}`),
	}}
	step := &ExecuteStep{Availability: availability, Transport: transport, Bridge: testProtocolBridge{}}
	if err := step.Execute(context.Background(), executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{candidate})); err == nil {
		t.Fatal("expected first request rejection")
	}
	if err := step.Execute(context.Background(), executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{candidate})); err != nil {
		t.Fatalf("next request blocked: %v", err)
	}
	if transport.calls != 2 {
		t.Fatalf("upstream calls=%d", transport.calls)
	}
}

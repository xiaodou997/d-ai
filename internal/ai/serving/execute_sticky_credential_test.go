package serving

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

// pinnableOAuthPool records which selection path a request took.
type pinnableOAuthPool struct {
	pinned      map[string]*domain.OAuthCredential // credential id → credential
	fallback    *domain.OAuthCredential
	pinRequests []string
	strategyHit int
}

func (p *pinnableOAuthPool) SelectCredentialFromPool(context.Context, string, string) (*domain.OAuthCredential, error) {
	p.strategyHit++
	return p.fallback, nil
}

func (p *pinnableOAuthPool) SelectPinnedCredential(_ context.Context, _, credID string) (*domain.OAuthCredential, error) {
	p.pinRequests = append(p.pinRequests, credID)
	if cred, ok := p.pinned[credID]; ok {
		return cred, nil
	}
	return nil, errors.New("credential is not an active member of the pool")
}

func (*pinnableOAuthPool) MarkInvalid(context.Context, string, string) error { return nil }
func (*pinnableOAuthPool) RecordSuccess(context.Context, string)             {}

func poolCandidate() *domain.RouteCandidate {
	return &domain.RouteCandidate{
		RouteID:           "pool-route",
		PoolID:            "pool-1",
		ModelCode:         "public-model",
		PoolUpstreamModel: "upstream-model",
		UpstreamModel:     "upstream-model",
		Protocol:          domain.ProtocolOpenAIChat,
		Timeouts:          domain.DefaultRouteTimeouts(domain.CapabilityChat),
	}
}

// A sticky-bound conversation must keep hitting the very same upstream account,
// not just the same route — otherwise every turn looks like a new session to
// the provider.
func TestExecuteReusesStickyPinnedCredential(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	pool := &pinnableOAuthPool{
		pinned:   map[string]*domain.OAuthCredential{"cred-sticky": {ID: "cred-sticky"}},
		fallback: &domain.OAuthCredential{ID: "cred-other"},
	}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{poolCandidate()})
	req.StickyHit = true
	req.StickyBinding = &routing.StickyBinding{TargetKind: "credential", RouteID: "pool-route", CredentialID: "cred-sticky"}

	step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, OAuthPool: pool}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.SelectedCredential == nil || req.SelectedCredential.ID != "cred-sticky" {
		t.Fatalf("selected credential = %+v, want cred-sticky", req.SelectedCredential)
	}
	if pool.strategyHit != 0 {
		t.Fatalf("pool strategy selection ran %d times, want 0", pool.strategyHit)
	}
}

// When the pinned account is gone (removed from the pool, disabled, banned),
// the request must still be served by falling back to normal selection.
func TestExecuteFallsBackWhenPinnedCredentialUnusable(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	pool := &pinnableOAuthPool{
		pinned:   map[string]*domain.OAuthCredential{},
		fallback: &domain.OAuthCredential{ID: "cred-other"},
	}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{poolCandidate()})
	req.StickyHit = true
	req.StickyBinding = &routing.StickyBinding{TargetKind: "credential", RouteID: "pool-route", CredentialID: "cred-gone"}

	step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, OAuthPool: pool}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.SelectedCredential == nil || req.SelectedCredential.ID != "cred-other" {
		t.Fatalf("selected credential = %+v, want cred-other", req.SelectedCredential)
	}
	if len(pool.pinRequests) != 1 {
		t.Fatalf("pin attempts = %v, want exactly one", pool.pinRequests)
	}
}

// Without a sticky binding nothing changes: the pool strategy decides.
func TestExecuteIgnoresPinWithoutStickyBinding(t *testing.T) {
	transport := &sequenceTransport{responses: []*UpstreamResponse{
		jsonResp(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"done"}}]}`),
	}}
	pool := &pinnableOAuthPool{
		pinned:   map[string]*domain.OAuthCredential{"cred-sticky": {ID: "cred-sticky"}},
		fallback: &domain.OAuthCredential{ID: "cred-other"},
	}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{poolCandidate()})

	step := &ExecuteStep{Transport: transport, Bridge: testProtocolBridge{}, OAuthPool: pool}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(pool.pinRequests) != 0 {
		t.Fatalf("pin attempts = %v, want none", pool.pinRequests)
	}
	if req.SelectedCredential == nil || req.SelectedCredential.ID != "cred-other" {
		t.Fatalf("selected credential = %+v, want cred-other", req.SelectedCredential)
	}
}

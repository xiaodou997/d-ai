package serving

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
	"xiaodou/dai/internal/ai/routing"

	"xiaodou/dai/internal/ai/domain"
)

type eligiblePoolStub struct {
	*recordingOAuthPool
	ids      []string
	strategy string
	picked   string
}

func (s *eligiblePoolStub) SelectEligibleCredential(_ context.Context, _ string, strategy string, ids []string) (string, error) {
	s.ids = ids
	s.strategy = strategy
	return s.picked, nil
}
func (*eligiblePoolStub) SelectPinnedCredential(_ context.Context, _ string, id string) (*domain.OAuthCredential, error) {
	return &domain.OAuthCredential{ID: id}, nil
}

type poolTargetScorer struct{ seen []*domain.RouteCandidate }

func (s *poolTargetScorer) Pick(_ context.Context, _ RouteScoringContext, cs []*domain.RouteCandidate, _ map[string]bool) *domain.RouteCandidate {
	s.seen = cs
	for _, c := range cs {
		if c.IsPoolRoute() {
			return c
		}
	}
	return nil
}

func TestPoolPolicySelectsOnlyEligibleMembersAfterTargetScoring(t *testing.T) {
	a := runtimeCandidate("")
	a.PoolID = "pool"
	a.AccountID = ""
	a.CredentialID = "a"
	a.CandidateID = "a"
	a.OAuthStrategy = "weighted"
	b := *a
	b.CredentialID = "b"
	b.CandidateID = "b"
	disabled := *a
	disabled.CredentialID = "already-used"
	disabled.CandidateID = "used"
	pool := &eligiblePoolStub{recordingOAuthPool: &recordingOAuthPool{}, picked: "b"}
	scorer := &poolTargetScorer{}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{a, &b, &disabled, runtimeCandidate("direct")})
	req.UsedCandidates[disabled.Key()] = true
	step := &ExecuteStep{OAuthPool: pool, Scorer: scorer}
	selected, _ := step.pickCandidate(context.Background(), req)
	if selected != &b || pool.strategy != "weighted" || !slices.Equal(pool.ids, []string{"a", "b"}) {
		t.Fatalf("selection=%+v strategy=%s ids=%v", selected, pool.strategy, pool.ids)
	}
	if len(scorer.seen) != 2 {
		t.Fatalf("pool multiplied its scoring votes: %d targets", len(scorer.seen))
	}
}

func TestSharedCapacityRetriesSameTargetOnceWithoutCooling(t *testing.T) {
	availability, _ := servingAvailability(t)
	capacity := func() *UpstreamResponse {
		return &UpstreamResponse{StatusCode: 429, Headers: http.Header{"Retry-After": []string{"0"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted, please try again later."}}`))}
	}
	first, other := runtimeCandidate("same"), runtimeCandidate("other")
	transport := &sequenceTransport{responses: []*UpstreamResponse{capacity(), capacity(), jsonResp(`{"choices":[{"message":{"content":"ok"}}]}`)}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{first, other})
	step := &ExecuteStep{Availability: availability, Transport: transport, Bridge: testProtocolBridge{}}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(req.Attempts) != 3 || req.Attempts[0].CandidateID != req.Attempts[1].CandidateID || req.Attempts[1].CandidateID == req.Attempts[2].CandidateID {
		t.Fatalf("capacity retry attempts=%+v", req.Attempts)
	}
	states, err := availability.Read(context.Background(), candidateScopes(first, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Phase != "available" || state.ConsecutiveFailures != 0 {
			t.Fatalf("capacity cooled healthy account: %+v", state)
		}
	}
}

type quotaPoolStub struct {
	*eligiblePoolStub
	quotas map[string][]domain.CredentialQuota
}

func (s *quotaPoolStub) ReadCredentialQuotas(context.Context, []string) (map[string][]domain.CredentialQuota, error) {
	return s.quotas, nil
}
func TestConfirmedQuotaOverridesStickyUntilWindowReset(t *testing.T) {
	availability, _ := servingAvailability(t)
	first := runtimeCandidate("")
	first.AccountID = ""
	first.PoolID = "pool"
	first.CredentialID = "exhausted"
	first.CandidateID = "exhausted"
	second := *first
	second.CredentialID = "healthy"
	second.CandidateID = "healthy"
	used := 100.0
	pool := &quotaPoolStub{eligiblePoolStub: &eligiblePoolStub{recordingOAuthPool: &recordingOAuthPool{}, picked: "healthy"}, quotas: map[string][]domain.CredentialQuota{"exhausted": {{ObservedAt: time.Now().UnixMilli(), Windows: []domain.CredentialQuotaWindow{{UsedPercent: &used, ResetAt: time.Now().Add(time.Hour).UnixMilli()}}}}}}
	step := &ExecuteStep{Availability: availability, OAuthPool: pool}
	request := func() *Request {
		req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{first, &second})
		req.StickyHit = true
		req.Candidate = first
		req.StickyBinding = &routing.StickyBinding{TargetKind: "credential", RouteID: first.RouteID, CredentialID: first.CredentialID}
		return req
	}
	req := request()
	selected, _ := step.pickCandidate(context.Background(), req)
	if selected != &second || !slices.Equal(pool.ids, []string{"healthy"}) {
		t.Fatalf("exhausted sticky account was selected: %+v", selected)
	}
	pool.quotas["exhausted"][0].Windows[0].ResetAt = time.Now().Add(-time.Second).UnixMilli()
	selected, _ = step.pickCandidate(context.Background(), request())
	if selected != first {
		t.Fatal("reset window did not allow the original sticky account")
	}
}

type recoveringWeightedPool struct{ *expandedTestPool }

func (*recoveringWeightedPool) SelectEligibleCredential(_ context.Context, _ string, _ string, ids []string) (string, error) {
	for _, id := range ids {
		if id == "healthy" {
			return id, nil
		}
	}
	return "", domain.ErrNotFound
}
func TestWeightedZeroCannotBypassPolicyThroughRecovery(t *testing.T) {
	availability, _ := servingAvailability(t)
	c := runtimeCandidate("")
	c.AccountID = ""
	c.PoolID = "pool"
	c.OAuthStrategy = "weighted"
	for _, scope := range candidateScopes(c, &domain.OAuthCredential{ID: "trial"}) {
		if scope.Kind == "model" {
			if err := availability.Reset(context.Background(), []routing.FaultScope{scope}); err != nil {
				t.Fatal(err)
			}
		}
	}
	step := &ExecuteStep{Availability: availability, OAuthPool: &recoveringWeightedPool{&expandedTestPool{&recordingOAuthPool{}}}}
	for i := 0; i < 20; i++ {
		req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{c})
		if err := step.expandCredentialCandidates(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		selected, _ := step.pickCandidate(context.Background(), req)
		if selected == nil || selected.CredentialID != "healthy" {
			t.Fatalf("recovery bypassed zero weight: %+v err=%v", selected, req.SelectionError)
		}
	}
}

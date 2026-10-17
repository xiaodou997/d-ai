package serving

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
)

type accountModelsStub struct {
	catalogs map[string]domain.CredentialModelCatalog
	err      error
}

func (s accountModelsStub) ReadCredentialModels(context.Context, string) (map[string]domain.CredentialModelCatalog, error) {
	return s.catalogs, s.err
}

func accountModelCandidate(id, model string) *domain.RouteCandidate {
	c := runtimeCandidate("")
	c.PoolID, c.AccountID, c.CredentialID, c.CandidateID, c.RouteID = "pool", "", id, id+model, "pool-route"
	c.PoolUpstreamModel = model
	c.ModelCode = "public-model-alias"
	return c
}

func TestAccountModelPermissionsOverrideStickyAndRespectUpstreamMapping(t *testing.T) {
	denied, allowed := accountModelCandidate("a", "provider-model"), accountModelCandidate("b", "provider-model")
	now := time.Now().UnixMilli()
	models := accountModelsStub{catalogs: map[string]domain.CredentialModelCatalog{
		"a": {CredentialID: "a", Models: []string{"other-model"}, Source: "cache", ValidUntil: now + 60000},
		"b": {CredentialID: "b", Models: []string{"provider-model"}, Source: "cache", ValidUntil: now + 60000},
	}}
	for _, health := range []bool{false, true} {
		req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{denied, allowed})
		req.Candidate, req.StickyHit = denied, true
		req.StickyBinding = &routing.StickyBinding{TargetKind: "credential", RouteID: denied.RouteID, CredentialID: denied.CredentialID}
		step := &ExecuteStep{CredentialModels: models}
		if health {
			step.Availability, _ = servingAvailability(t)
		}
		picked, _ := step.pickCandidate(context.Background(), req)
		if picked != allowed || !req.UsedCandidates[denied.Key()] || req.SelectionReason == "sticky" {
			t.Fatalf("health=%t picked=%+v reason=%s error=%v", health, picked, req.SelectionReason, req.SelectionError)
		}
		if len(req.SkippedAttempts) != 1 || req.SkippedAttempts[0].Outcome != ResultRejected || req.SkippedAttempts[0].CredentialID != "a" {
			t.Fatalf("model exclusion must identify the account without reporting a circuit failure: %+v", req.SkippedAttempts)
		}
	}
}

func TestUnknownStaleAndFailedModelReadsKeepAccountsEligible(t *testing.T) {
	for _, test := range []struct {
		name    string
		catalog domain.CredentialModelCatalog
		err     error
	}{
		{name: "unknown", catalog: domain.CredentialModelCatalog{Source: "unknown"}},
		{name: "stale", catalog: domain.CredentialModelCatalog{Source: "stale", Models: []string{"other"}, ValidUntil: time.Now().Add(-time.Second).UnixMilli()}},
		{name: "expired", catalog: domain.CredentialModelCatalog{Source: "cache", Models: []string{"other"}, ValidUntil: time.Now().Add(-time.Second).UnixMilli()}},
		{name: "empty-manifest", catalog: domain.CredentialModelCatalog{Source: "cache", Models: []string{}, ValidUntil: time.Now().Add(time.Minute).UnixMilli()}},
		{name: "database-failure", err: errors.New("database unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := accountModelCandidate("a", "model")
			req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{candidate})
			step := &ExecuteStep{CredentialModels: accountModelsStub{catalogs: map[string]domain.CredentialModelCatalog{"a": test.catalog}, err: test.err}}
			picked, _ := step.pickCandidate(context.Background(), req)
			if picked != candidate || req.SelectionError != nil {
				t.Fatalf("advisory model discovery blocked request: picked=%+v error=%v", picked, req.SelectionError)
			}
		})
	}
}

func TestAccountModelExclusionFallsBackToAnotherTarget(t *testing.T) {
	denied, direct := accountModelCandidate("a", "model"), runtimeCandidate("direct")
	step := &ExecuteStep{CredentialModels: accountModelsStub{catalogs: map[string]domain.CredentialModelCatalog{
		"a": {Source: "live", Models: []string{"other"}, ValidUntil: time.Now().Add(time.Minute).UnixMilli()},
	}}}
	req := executeTestRequest(httptest.NewRecorder(), []*domain.RouteCandidate{denied, direct})
	picked, _ := step.pickCandidate(context.Background(), req)
	if picked != direct {
		t.Fatalf("model-ineligible pool prevented target fallback: %+v", picked)
	}
}

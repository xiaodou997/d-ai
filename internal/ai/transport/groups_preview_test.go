package transport

import (
	"testing"

	"xiaodou/dai/internal/ai/commercial"
)

func TestGroupTargetWriteFromRequestRequiresExactlyOneTarget(t *testing.T) {
	if _, err := groupTargetWriteFromRequest(groupTargetWriteRequest{}); err == nil {
		t.Fatal("expected missing target validation")
	}
	if _, err := groupTargetWriteFromRequest(groupTargetWriteRequest{AccountID: "account-1", CredentialPoolID: "pool-1"}); err == nil {
		t.Fatal("expected mutually-exclusive target validation")
	}
	write, err := groupTargetWriteFromRequest(groupTargetWriteRequest{AccountID: " account-1 "})
	if err != nil || write.TargetKind != commercial.TargetKindDirectUpstream || write.TargetID != "account-1" {
		t.Fatalf("account target = %#v, %v", write, err)
	}
}

func TestGroupWriteFromRequestCarriesRoutePolicyVersion(t *testing.T) {
	version := int64(4)
	write := groupWriteFromReq(groupWriteRequest{
		Name: "group", RetailPriceBookID: "book", RoutePolicyVersion: &version,
	})
	if write.ExpectedRoutePolicyVersion != version {
		t.Fatalf("expected route policy version = %d, want %d", write.ExpectedRoutePolicyVersion, version)
	}
}

func TestGroupDispatchPreviewToDTOIncludesRoutePolicy(t *testing.T) {
	dto := groupDispatchPreviewToDTO(commercial.DispatchPreview{
		RequestedModel:  "gpt-latest",
		ClientSurface:   "openai_chat",
		ResolvedModelID: "gpt-5.5",
		RouteStrategy:   commercial.RouteStrategyWeighted,
		RouteObjective:  commercial.RouteObjectiveBalanced,
		CandidateUpstreams: []commercial.DispatchPreviewCandidate{{
			TargetType:    "account",
			Priority:      10,
			RoutingWeight: 2.5,
		}},
	})
	if dto.RouteStrategy != "weighted" || dto.RouteObjective != "balanced" {
		t.Fatalf("route policy = %q/%q", dto.RouteStrategy, dto.RouteObjective)
	}
	if len(dto.CandidateUpstreams) != 1 || dto.CandidateUpstreams[0].RoutingWeight != 2.5 {
		t.Fatalf("candidate weights = %#v", dto.CandidateUpstreams)
	}
}

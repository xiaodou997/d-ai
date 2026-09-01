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
		RoutePolicy:     commercial.RoutePolicyCost,
		CandidateUpstreams: []commercial.DispatchPreviewCandidate{{
			TargetType: "account",
		}},
	})
	if dto.RoutePolicy != "cost" {
		t.Fatalf("route policy = %q", dto.RoutePolicy)
	}
	if len(dto.CandidateUpstreams) != 1 {
		t.Fatalf("candidate upstreams = %#v", dto.CandidateUpstreams)
	}
}

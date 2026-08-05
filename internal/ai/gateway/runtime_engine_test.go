package gateway

import (
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/core/surface"
	coreupstream "xiaodou/dai/internal/ai/core/upstream"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
)

func TestBuildRuntimeServingRequestHasNoPreResolvedCandidates(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-luna","messages":[{"role":"user","content":"hi"}]}`)
	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(string(body)))
	req, err := buildRuntimeServingRequest(coreruntime.ExecutionInput{
		Subject: coreidentity.Subject{Scope: coreidentity.ScopeTenant, TenantID: "tenant-a"},
		Request: coreruntime.Request{
			RequestID:      "req-1",
			Capability:     catalog.CapabilityChat,
			ClientSurface:  surface.OpenAIChat,
			RequestedModel: "gpt-5.6-luna",
			ReceivedAt:     time.Now(),
		},
		Envelope: coreruntime.ExecutionEnvelope{
			ResponseWriter: httptest.NewRecorder(),
			HTTPRequest:    httpReq,
			ClientBody:     body,
		},
	})
	if err != nil {
		t.Fatalf("buildRuntimeServingRequest: %v", err)
	}
	if len(req.Candidates) != 0 || req.Candidate != nil {
		t.Fatalf("routes must be planned inside the pipeline, candidates=%v candidate=%v", req.Candidates, req.Candidate)
	}
	if req.RuntimeCapability != catalog.CapabilityChat || req.RequestedModel != "gpt-5.6-luna" {
		t.Fatalf("runtime request = %+v", req)
	}
}

func TestRuntimeRouteSelectorPreservesCanonicalFailoverPlan(t *testing.T) {
	planner := &routePlannerStub{plan: coreruntime.RoutePlan{Candidates: []coreruntime.PlannedTarget{
		plannedDirectTarget("route-1", "group-1", 0, "upstream-1", 3),
		plannedDirectTarget("route-2", "group-1", 0, "upstream-2", 5),
		plannedDirectTarget("route-3", "group-2", 1, "upstream-3", 1),
	}}}
	planner.plan.Candidates[0].Binding.ConversionBucket = 2
	selector := NewRuntimeRouteSelector(planner, zap.NewNop())
	req := &serving.Request{
		Subject:           &coreidentity.Subject{TenantID: "tenant-a"},
		RequestID:         "req-plan",
		RequestedModel:    "gpt-5.6-luna",
		ModelCode:         "gpt-5.6-luna",
		RuntimeCapability: catalog.CapabilityChat,
		CapabilityType:    domain.CapabilityChat,
		ClientProtocol:    domain.ProtocolOpenAIChat,
	}

	candidates, err := selector.SelectCandidates(context.Background(), req)
	if err != nil {
		t.Fatalf("SelectCandidates: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidate count = %d, want 3", len(candidates))
	}
	for i, want := range []struct {
		route string
		group int
		prio  int
	}{{"route-1", 0, 3}, {"route-2", 0, 5}, {"route-3", 1, 1}} {
		got := candidates[i]
		if got.RouteID != want.route || got.GroupRank != want.group || got.Priority != want.prio {
			t.Fatalf("candidate[%d] = route=%q group=%d priority=%d", i, got.RouteID, got.GroupRank, got.Priority)
		}
	}
	if candidates[0].ConversionBucket != 2 {
		t.Fatalf("conversion bucket = %d, want 2", candidates[0].ConversionBucket)
	}
}

func TestRuntimeRouteSelectorAppliesSubscriptionGroupsBeforePlanning(t *testing.T) {
	planner := &routePlannerStub{plan: coreruntime.RoutePlan{Candidates: []coreruntime.PlannedTarget{
		plannedDirectTarget("route-plan", "group-plan", 0, "upstream-1", 3),
	}}}
	selector := NewRuntimeRouteSelector(planner, zap.NewNop())
	req := &serving.Request{
		Subject:           &coreidentity.Subject{TenantID: "tenant-a"},
		RequestID:         "req-subscription",
		RequestedModel:    "gpt-5.6-luna",
		RuntimeCapability: catalog.CapabilityChat,
		CapabilityType:    domain.CapabilityChat,
		ClientProtocol:    domain.ProtocolOpenAIChat,
		BillingSource:     subscription.BillingSourceSubscription,
		SubscriptionGroupQuotaDebitMultipliers: map[string]float64{
			"group-plan": 1,
		},
	}

	if _, err := selector.SelectCandidates(context.Background(), req); err != nil {
		t.Fatalf("SelectCandidates: %v", err)
	}
	if len(planner.request.AllowedGroupIDs) != 1 || planner.request.AllowedGroupIDs[0] != "group-plan" {
		t.Fatalf("allowed groups = %v", planner.request.AllowedGroupIDs)
	}
}

func TestBuildRuntimeServingRequestNormalizesImageMetadata(t *testing.T) {
	body := []byte(`{"model":"gpt-image-1","n":3,"size":"1024x1024"}`)
	httpReq := httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(string(body)))
	httpReq.Header.Set("Content-Type", "application/json")
	req, err := buildRuntimeServingRequest(coreruntime.ExecutionInput{
		Subject: coreidentity.Subject{TenantID: "tenant-a"},
		Request: coreruntime.Request{
			RequestID:      "req-image",
			Capability:     catalog.CapabilityImageGeneration,
			ClientSurface:  surface.OpenAIImages,
			RequestedModel: "gpt-image-1",
		},
		Envelope: coreruntime.ExecutionEnvelope{HTTPRequest: httpReq, ClientBody: body},
	})
	if err != nil {
		t.Fatalf("buildRuntimeServingRequest: %v", err)
	}
	if req.TokenUsage.ImageCount != 3 || req.TokenUsage.ImageResolution != "1024x1024" {
		t.Fatalf("image metadata = %+v", req.TokenUsage)
	}
}

func TestRuntimeResultUsesSettledCallerCharge(t *testing.T) {
	req := &serving.Request{
		RequestID:         "req-charge",
		Subject:           &coreidentity.Subject{Scope: coreidentity.ScopeUser, TenantID: "tenant-a", UserID: "user-a"},
		ClientProtocol:    domain.ProtocolOpenAIChat,
		RuntimeCapability: catalog.CapabilityChat,
		CapabilityType:    domain.CapabilityChat,
		BillingResult: domain.BillingResult{
			TenantPayableMicro:   700,
			UserPayableMicro:     1_100,
			UserChargedMicro:     900,
			APIKeyQuotaCostMicro: 1_100,
		},
	}
	result := runtimeResultFromServing(req)
	if result.CallerChargeMicro != 900 || result.UserChargedMicro != 900 {
		t.Fatalf("runtime result actual/user debit = %d/%d, want 900/900", result.CallerChargeMicro, result.UserChargedMicro)
	}
	if result.CallerChargeMicro == result.APIKeyQuotaCostMicro {
		t.Fatalf("actual caller charge must not use API key quota meter: %+v", result)
	}
}

func TestRouteTimeoutsFromUpstreamUsesSystemPolicy(t *testing.T) {
	got := routeTimeoutsFromUpstream(domain.CapabilityImage, coreupstream.Upstream{})
	want := domain.DefaultRouteTimeouts(domain.CapabilityImage)
	if got != want {
		t.Fatalf("image timeouts = %+v, want system policy %+v", got, want)
	}
}

type routePlannerStub struct {
	plan    coreruntime.RoutePlan
	err     error
	subject coreidentity.Subject
	request coreruntime.Request
}

func (s *routePlannerStub) Resolve(_ context.Context, subject coreidentity.Subject, req coreruntime.Request) (coreruntime.RoutePlan, error) {
	s.subject = subject
	s.request = req
	return s.plan, s.err
}

func plannedDirectTarget(routeID, groupID string, groupRank int, endpointID string, priority int) coreruntime.PlannedTarget {
	return coreruntime.PlannedTarget{
		RouteID:   routeID,
		GroupRank: groupRank,
		Group: commercial.AccessibleGroup{Group: commercial.Group{
			ID: groupID, Name: groupID, RetailPriceBookID: "sell-book", DefaultUserMultiplier: 1,
		}},
		Target:  commercial.GroupTarget{ID: routeID, GroupID: groupID, TargetID: endpointID, TargetKind: commercial.TargetKindDirectUpstream, Priority: priority},
		ModelID: "gpt-5.6-luna",
		Binding: coreupstream.RuntimeBinding{
			Upstream: coreupstream.Upstream{
				ID: endpointID, Code: endpointID, ProviderFamily: coreupstream.ProviderFamilyOpenAICompatible,
				AccessMode: coreupstream.AccessModeDirect, BaseURL: "https://" + endpointID + ".example.test",
			},
			ModelBinding: coreupstream.ModelBinding{
				UpstreamKind: coreupstream.AccessModeDirect, UpstreamID: endpointID, ModelID: "gpt-5.6-luna",
				Capability: catalog.CapabilityChat, RequestSurface: surface.OpenAIChat, ResponseSurface: surface.OpenAIChat,
				UpstreamModelName: "gpt-5.6-luna",
			},
			CostPriceBookID:  "cost-book",
			TenantMultiplier: 1,
			CostPer1kTokens:  0.5,
		},
	}
}

// 路由不通时服务端必须留下可排查的线索：客户端只会收到一句含糊的 503
// （原因涉及内部分组/目标/凭证，不能外泄），日志是原因唯一的落脚处。
// 主密钥配错曾经在这里一条日志都不留，只表现为 no_available_route。
func TestRuntimeRouteSelectorLogsRejectionReasons(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	planner := &routePlannerStub{err: &coreruntime.NoRouteError{Rejections: []coreruntime.RejectedTarget{
		{
			Target: commercial.GroupTarget{ID: "t1"},
			Code:   coreruntime.RejectionCredentialUnavailable,
			Detail: "decrypt provider credential: cipher: message authentication failed",
		},
		{
			Target: commercial.GroupTarget{ID: "t2"},
			Code:   coreruntime.RejectionTargetInactive,
			Detail: "upstream account is disabled",
		},
	}}}
	selector := NewRuntimeRouteSelector(planner, zap.New(core))

	_, err := selector.SelectCandidates(context.Background(), &serving.Request{
		Subject:           &coreidentity.Subject{TenantID: "tenant-a"},
		RequestID:         "req-no-route",
		RequestedModel:    "gpt-5.6-luna",
		ModelCode:         "gpt-5.6-luna",
		RuntimeCapability: catalog.CapabilityChat,
		CapabilityType:    domain.CapabilityChat,
		ClientProtocol:    domain.ProtocolOpenAIChat,
	})

	// 对客户端的错误契约不变：仍是 no_available_route，不泄露内部细节。
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "no_available_route" {
		t.Fatalf("err = %v, want no_available_route APIError", err)
	}
	if strings.Contains(apiErr.Message, "decrypt") {
		t.Fatalf("client message leaked internals: %q", apiErr.Message)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want exactly one warn", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["request_id"] != "req-no-route" || fields["model"] != "gpt-5.6-luna" {
		t.Fatalf("log fields lack request context: %#v", fields)
	}
	// 两个目标各自的原因都要能在日志里读到，否则排查时仍要猜是哪一个坏了。
	rendered := entries[0].Message + " " + fmt.Sprint(fields)
	for _, want := range []string{"credential_unavailable", "decrypt provider credential", "target_inactive"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("log missing %q: %s", want, rendered)
		}
	}
}

// 规划阶段就失败（无可访问分组等）时也要留痕，且不能因为没有拒绝清单而崩。
func TestRuntimeRouteSelectorLogsPlanningFailureWithoutRejections(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	planner := &routePlannerStub{err: commercial.ErrNoAccessibleGroup}
	selector := NewRuntimeRouteSelector(planner, zap.New(core))

	_, err := selector.SelectCandidates(context.Background(), &serving.Request{
		Subject:           &coreidentity.Subject{TenantID: "tenant-a"},
		RequestID:         "req-no-group",
		RequestedModel:    "gpt-5.6-luna",
		ModelCode:         "gpt-5.6-luna",
		RuntimeCapability: catalog.CapabilityChat,
		CapabilityType:    domain.CapabilityChat,
		ClientProtocol:    domain.ProtocolOpenAIChat,
	})
	var apiErr *serving.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "no_accessible_group" {
		t.Fatalf("err = %v, want no_accessible_group APIError", err)
	}
	if len(logs.All()) != 1 {
		t.Fatalf("log entries = %d, want one warn", len(logs.All()))
	}
}

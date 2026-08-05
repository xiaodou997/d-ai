package runtime

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/core/upstream"
	coreupstream "xiaodou/dai/internal/ai/core/upstream"
)

func TestResolverResolveSkipsUnbindableTarget(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{
			{
				Group:           commercial.AccessibleGroup{Group: commercial.Group{ID: "g1", Name: "Main"}},
				ResolvedModelID: "gpt-5.4",
				Targets: []commercial.GroupTarget{
					{ID: "t1", GroupID: "g1", TargetID: "bad", TargetKind: commercial.TargetKindDirectUpstream},
					{ID: "t2", GroupID: "g1", TargetID: "ok", TargetKind: commercial.TargetKindDirectUpstream},
				},
			},
		},
	}
	binder := &targetBinderStub{
		results: map[string]error{
			"bad": coreupstream.ErrNoRuntimeBinding,
		},
	}
	resolver := NewResolver(NewPlanner(dispatcher), binder)

	got, err := resolver.Resolve(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		RequestID:      "req-1",
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.OpenAIChat,
		RequestedModel: "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Binding.Upstream.ID != "ok" {
		t.Fatalf("resolved candidates = %#v", got.Candidates)
	}
}

func TestResolverInspectReportsUnbindableTargetWithoutTurningItIntoRuntimeSuccess(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{{
			Group:           commercial.AccessibleGroup{Group: commercial.Group{ID: "g1"}},
			ResolvedModelID: "gpt-5.5",
			MatchedRule:     &commercial.DispatchRule{ID: "rule-1", TargetModelID: "gpt-5.5"},
			Targets: []commercial.GroupTarget{{
				ID: "route-1", GroupID: "g1", TargetID: "upstream-1",
				TargetKind: commercial.TargetKindDirectUpstream,
			}},
		}},
	}
	resolver := NewResolver(NewPlanner(dispatcher), &targetBinderStub{results: map[string]error{
		"upstream-1": coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionModelBindingMissing, "no active binding for model gpt-5.5"),
	}})

	inspection, err := resolver.Inspect(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.OpenAIChat,
		RequestedModel: "claude-opus-4-8",
	})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(inspection.Candidates) != 0 {
		t.Fatalf("inspection candidates = %#v, want none", inspection.Candidates)
	}
	if len(inspection.RejectedCandidates) != 1 || inspection.RejectedCandidates[0].Code != RejectionModelBindingMissing {
		t.Fatalf("inspection rejections = %#v", inspection.RejectedCandidates)
	}
	if inspection.RejectedCandidates[0].MatchedRule == nil || inspection.RejectedCandidates[0].MatchedRule.ID != "rule-1" {
		t.Fatalf("inspection matched rule = %#v", inspection.RejectedCandidates[0].MatchedRule)
	}

	_, err = resolver.Resolve(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.OpenAIChat,
		RequestedModel: "claude-opus-4-8",
	})
	if !errors.Is(err, ErrNoRouteCandidates) {
		t.Fatalf("Resolve() error = %v, want ErrNoRouteCandidates", err)
	}
}

func TestResolverResolvePreservesAllBindableTargetsForFailover(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{
			{
				Group:           commercial.AccessibleGroup{Group: commercial.Group{ID: "g1", Name: "Main"}},
				ResolvedModelID: "gpt-5.4",
				Targets: []commercial.GroupTarget{
					{ID: "route-1", GroupID: "g1", TargetID: "upstream-1", TargetKind: commercial.TargetKindDirectUpstream},
					{ID: "route-2", GroupID: "g1", TargetID: "upstream-2", TargetKind: commercial.TargetKindDirectUpstream},
				},
			},
		},
	}
	binder := &targetBinderStub{}
	resolver := NewResolver(NewPlanner(dispatcher), binder)

	got, err := resolver.Resolve(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		RequestID:      "req-failover",
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.OpenAIChat,
		RequestedModel: "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got, want := binder.targetIDs, []string{"upstream-1", "upstream-2"}; !slices.Equal(got, want) {
		t.Fatalf("bound targets = %v, want %v so 50x can fail over", got, want)
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("resolved targets = %d, want 2", len(got.Candidates))
	}
	if got.Candidates[0].Binding.Upstream.ID != "upstream-1" || got.Candidates[1].Binding.Upstream.ID != "upstream-2" {
		t.Fatalf("resolution did not preserve primary and failover targets: %#v", got)
	}
	if got.Candidates[0].RouteID != "route-1" || got.Candidates[1].RouteID != "route-2" {
		t.Fatalf("route ids = %q, %q", got.Candidates[0].RouteID, got.Candidates[1].RouteID)
	}
}

func TestResolverResolveReturnsBinderError(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{
			{
				Group:           commercial.AccessibleGroup{Group: commercial.Group{ID: "g1"}},
				ResolvedModelID: "gpt-5.4",
				Targets:         []commercial.GroupTarget{{ID: "t1", GroupID: "g1", TargetID: "bad", TargetKind: commercial.TargetKindDirectUpstream}},
			},
		},
	}
	binder := &targetBinderStub{
		results: map[string]error{
			"bad": errors.New("database exploded"),
		},
	}
	resolver := NewResolver(NewPlanner(dispatcher), binder)

	_, err := resolver.Resolve(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		Capability:     catalog.CapabilityChat,
		ClientSurface:  surface.OpenAIChat,
		RequestedModel: "gpt-5.4",
	})
	if err == nil || err.Error() != "database exploded" {
		t.Fatalf("expected binder error, got %v", err)
	}
}

func TestResolverResolveFiltersSubscriptionGroupsWithoutChangingPlanOrder(t *testing.T) {
	t.Parallel()
	dispatcher := &runtimeDispatcherStub{options: []commercial.DispatchResolution{
		{
			Group: commercial.AccessibleGroup{Group: commercial.Group{ID: "primary"}}, ResolvedModelID: "gpt-5.4",
			Targets: []commercial.GroupTarget{{ID: "route-primary", GroupID: "primary", TargetID: "up-primary", TargetKind: commercial.TargetKindDirectUpstream}},
		},
		{
			Group: commercial.AccessibleGroup{Group: commercial.Group{ID: "covered"}}, ResolvedModelID: "gpt-5.4",
			Targets: []commercial.GroupTarget{{ID: "route-covered", GroupID: "covered", TargetID: "up-covered", TargetKind: commercial.TargetKindDirectUpstream}},
		},
	}}
	resolver := NewResolver(NewPlanner(dispatcher), &targetBinderStub{})
	plan, err := resolver.Resolve(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		Capability: catalog.CapabilityChat, ClientSurface: surface.OpenAIChat, RequestedModel: "gpt-5.4",
		AllowedGroupIDs: []string{"covered"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].RouteID != "route-covered" || plan.Candidates[0].GroupRank != 1 {
		t.Fatalf("subscription plan = %#v", plan.Candidates)
	}
}

func TestResolverResolveKeepsUsablePlanWhenLaterBindingErrors(t *testing.T) {
	t.Parallel()
	dispatcher := &runtimeDispatcherStub{options: []commercial.DispatchResolution{{
		Group: commercial.AccessibleGroup{Group: commercial.Group{ID: "g1"}}, ResolvedModelID: "gpt-5.4",
		Targets: []commercial.GroupTarget{
			{ID: "route-ok", GroupID: "g1", TargetID: "ok", TargetKind: commercial.TargetKindDirectUpstream},
			{ID: "route-error", GroupID: "g1", TargetID: "error", TargetKind: commercial.TargetKindDirectUpstream},
		},
	}}}
	resolver := NewResolver(NewPlanner(dispatcher), &targetBinderStub{results: map[string]error{"error": errors.New("temporary database error")}})
	plan, err := resolver.Resolve(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		Capability: catalog.CapabilityChat, ClientSurface: surface.OpenAIChat, RequestedModel: "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].RouteID != "route-ok" {
		t.Fatalf("usable plan = %#v", plan.Candidates)
	}
}

type targetBinderStub struct {
	results   map[string]error
	targetIDs []string
}

func (s *targetBinderStub) ResolveRuntimeBinding(_ context.Context, req coreupstream.RuntimeBindingRequest) (coreupstream.RuntimeBinding, error) {
	s.targetIDs = append(s.targetIDs, req.TargetID)
	if err := s.results[req.TargetID]; err != nil {
		return coreupstream.RuntimeBinding{}, err
	}
	return coreupstream.RuntimeBinding{
		Upstream: upstream.Upstream{
			ID: req.TargetID,
		},
		ModelBinding: upstream.ModelBinding{
			ModelID: req.ResolvedModelID,
		},
	}, nil
}

// 所有候选都被拒时，Resolve 必须把拒绝原因带出来。客户端只会看到一句含糊的
// "no available upstream route"（原因涉及内部分组/目标/凭证，不能外泄），
// 服务端因此是原因唯一能落脚的地方 —— 主密钥配错曾经在这里一条日志都不留。
func TestResolveCarriesRejectionReasonsWhenNoCandidateSurvives(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{{
			Group:           commercial.AccessibleGroup{Group: commercial.Group{ID: "g1", Name: "Main"}},
			ResolvedModelID: "gpt-5.4",
			Targets: []commercial.GroupTarget{
				{ID: "t1", GroupID: "g1", TargetID: "bad-key", TargetKind: commercial.TargetKindDirectUpstream},
				{ID: "t2", GroupID: "g1", TargetID: "inactive", TargetKind: commercial.TargetKindDirectUpstream},
			},
		}},
	}
	binder := &targetBinderStub{
		results: map[string]error{
			"bad-key": coreupstream.NewRuntimeBindingRejection(
				coreupstream.BindingRejectionCredentialUnavailable,
				"decrypt provider credential: cipher: message authentication failed"),
			"inactive": coreupstream.NewRuntimeBindingRejection(
				coreupstream.BindingRejectionTargetInactive, "upstream account is disabled"),
		},
	}
	resolver := NewResolver(NewPlanner(dispatcher), binder)

	_, err := resolver.Resolve(context.Background(), identity.Subject{TenantID: "tenant-1"}, Request{
		RequestID: "req-1", Capability: catalog.CapabilityChat,
		ClientSurface: surface.OpenAIChat, RequestedModel: "gpt-5.4",
	})

	// 既有的错误映射靠 errors.Is 判定，不能被这次改动打破。
	if !errors.Is(err, ErrNoRouteCandidates) {
		t.Fatalf("err = %v, want it to unwrap to ErrNoRouteCandidates", err)
	}
	var noRoute *NoRouteError
	if !errors.As(err, &noRoute) {
		t.Fatalf("err = %v, want *NoRouteError carrying the reasons", err)
	}
	if len(noRoute.Rejections) != 2 {
		t.Fatalf("rejections = %#v, want both targets", noRoute.Rejections)
	}

	byCode := map[RejectionCode]string{}
	for _, rejected := range noRoute.Rejections {
		byCode[rejected.Code] = rejected.Detail
	}
	detail, ok := byCode[RejectionCredentialUnavailable]
	if !ok {
		t.Fatalf("rejections = %#v, want a credential_unavailable entry", noRoute.Rejections)
	}
	// 解密失败的根因必须留住：否则「密钥配错」与「账号没配凭证」在日志里长得一模一样。
	if !strings.Contains(detail, "decrypt provider credential") {
		t.Fatalf("detail = %q, want the decryption cause preserved", detail)
	}
	if _, ok := byCode[RejectionTargetInactive]; !ok {
		t.Fatalf("rejections = %#v, want a target_inactive entry", noRoute.Rejections)
	}
}

// 有可用候选时不构造 NoRouteError（它只描述失败）。
func TestResolveReturnsNoRejectionErrorOnSuccess(t *testing.T) {
	t.Parallel()

	dispatcher := &runtimeDispatcherStub{
		options: []commercial.DispatchResolution{{
			Group:           commercial.AccessibleGroup{Group: commercial.Group{ID: "g1"}},
			ResolvedModelID: "gpt-5.4",
			Targets:         []commercial.GroupTarget{{ID: "t1", GroupID: "g1", TargetID: "ok", TargetKind: commercial.TargetKindDirectUpstream}},
		}},
	}
	resolver := NewResolver(NewPlanner(dispatcher), &targetBinderStub{})
	if _, err := resolver.Resolve(context.Background(), identity.Subject{TenantID: "t"}, Request{
		RequestID: "req-1", Capability: catalog.CapabilityChat,
		ClientSurface: surface.OpenAIChat, RequestedModel: "gpt-5.4",
	}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

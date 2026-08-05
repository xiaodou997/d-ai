package serving

import (
	"context"
	"testing"

	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/subscription"
)

// fakeGateSubs 返回预置的 GateDecision，供 gate 分支单测。
type fakeGateSubs struct {
	decision subscription.GateDecision
}

func (f fakeGateSubs) ResolveForGate(context.Context, string, string) (subscription.GateDecision, error) {
	return f.decision, nil
}
func gateReq(forced, keyGroup string) *Request {
	return &Request{
		RequestID: "r",
		Subject: &coreidentity.Subject{
			Scope:         coreidentity.ScopeUser,
			TenantID:      "t",
			UserID:        "u",
			ForcedGroupID: forced,
			GroupID:       keyGroup,
		},
	}
}

func runGate(t *testing.T, decision subscription.GateDecision, forced, keyGroup string) *Request {
	t.Helper()
	req := gateReq(forced, keyGroup)
	step := &SubscriptionGateStep{Subs: fakeGateSubs{decision: decision}, Logger: zap.NewNop()}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("gate execute: %v", err)
	}
	return req
}

func TestGateReleaseBranches(t *testing.T) {
	covered := subscription.GateDecision{Covered: true, SubscriptionID: "s1", GroupQuotaDebitMultipliers: map[string]float64{"gA": 1, "gB": 2}}

	// 覆盖 + 无 forced/无 key 绑定 ⇒ subscription，携带权重
	if req := runGate(t, covered, "", ""); req.BillingSource != subscription.BillingSourceSubscription ||
		req.SubscriptionID != "s1" || req.SubscriptionGroupQuotaDebitMultipliers["gB"] != 2 {
		t.Fatalf("covered plain: bs=%q id=%q w=%v", req.BillingSource, req.SubscriptionID, req.SubscriptionGroupQuotaDebitMultipliers)
	}
	// 未覆盖 ⇒ payg
	if req := runGate(t, subscription.GateDecision{Covered: false}, "", ""); req.BillingSource != subscription.BillingSourcePayg {
		t.Fatalf("not covered should be payg, got %q", req.BillingSource)
	}
	// 覆盖但快照为空（存量订阅）⇒ payg
	empty := subscription.GateDecision{Covered: true, SubscriptionID: "s1"}
	if req := runGate(t, empty, "", ""); req.BillingSource != subscription.BillingSourcePayg {
		t.Fatalf("empty weights should be payg, got %q", req.BillingSource)
	}
	// forced 在套餐集内 ⇒ subscription
	if req := runGate(t, covered, "gA", ""); req.BillingSource != subscription.BillingSourceSubscription {
		t.Fatalf("forced in-set should be subscription, got %q", req.BillingSource)
	}
	// forced 在套餐集外 ⇒ payg（显式指定套餐外分组）
	if req := runGate(t, covered, "gZ", ""); req.BillingSource != subscription.BillingSourcePayg {
		t.Fatalf("forced out-of-set should be payg, got %q", req.BillingSource)
	}
	// key 绑定分组在套餐集内 ⇒ subscription
	if req := runGate(t, covered, "", "gB"); req.BillingSource != subscription.BillingSourceSubscription {
		t.Fatalf("key group in set should be subscription, got %q", req.BillingSource)
	}
	// key 绑定分组不在套餐集内 ⇒ payg
	if req := runGate(t, covered, "", "gZ"); req.BillingSource != subscription.BillingSourcePayg {
		t.Fatalf("key group outside set should be payg, got %q", req.BillingSource)
	}
}

func TestSubscriptionGateAlwaysUsesPaygForAsyncExecution(t *testing.T) {
	covered := subscription.GateDecision{
		Covered: true, SubscriptionID: "s1",
		GroupQuotaDebitMultipliers: map[string]float64{"gA": 1},
	}
	req := gateReq("", "")
	req.ExecutionMode = coreruntime.ExecutionModeAsync
	step := &SubscriptionGateStep{Subs: fakeGateSubs{decision: covered}, Logger: zap.NewNop()}
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("gate execute: %v", err)
	}
	if req.BillingSource != subscription.BillingSourcePayg || req.SubscriptionID != "" {
		t.Fatalf("async execution must be payg, source=%q subscription=%q", req.BillingSource, req.SubscriptionID)
	}
}

func TestSubscriptionDebitMicro(t *testing.T) {
	weights := map[string]float64{"gA": 2, "gB": 0.4}
	mk := func(base int64, gid string) *Request {
		return &Request{
			Candidate:                              &domain.RouteCandidate{GroupID: gid},
			SubscriptionGroupQuotaDebitMultipliers: weights,
			BillingResult:                          domain.BillingResult{RetailBaseMicro: base},
		}
	}
	// 基准价 × 权重（整数）
	if m, ok := SubscriptionDebitMicro(mk(1000, "gA")); !ok || m != 2000 {
		t.Fatalf("gA: got %d ok=%v, want 2000 true", m, ok)
	}
	// 四舍五入：1001 × 0.4 = 400.4 → 400
	if m, ok := SubscriptionDebitMicro(mk(1001, "gB")); !ok || m != 400 {
		t.Fatalf("gB round: got %d ok=%v, want 400 true", m, ok)
	}
	// 基准价 <=0 ⇒ 不可计量
	if _, ok := SubscriptionDebitMicro(mk(0, "gA")); ok {
		t.Fatalf("base 0 should be unmeterable")
	}
	// 命中分组不在权重表 ⇒ 不可计量
	if _, ok := SubscriptionDebitMicro(mk(1000, "gZ")); ok {
		t.Fatalf("group not in weights should be unmeterable")
	}
	// 无候选 ⇒ 不可计量
	noCand := &Request{SubscriptionGroupQuotaDebitMultipliers: weights, BillingResult: domain.BillingResult{RetailBaseMicro: 1000}}
	if _, ok := SubscriptionDebitMicro(noCand); ok {
		t.Fatalf("nil candidate should be unmeterable")
	}
}

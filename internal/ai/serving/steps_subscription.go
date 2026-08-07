package serving

import (
	"context"

	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/subscription"
)

// Subscriptions is the narrow port the pipeline needs from the subscription
// domain service. *subscription.Service satisfies it. Kept small so the gate
// step and ledger finalizer stay testable without the full service.
type Subscriptions interface {
	// ResolveForGate performs lazy expiry/activation + three-window remaining
	// check for the (tenant,user) pair. Hot path: one point read per request.
	ResolveForGate(ctx context.Context, tenantID, userID string) (subscription.GateDecision, error)
}

// ============================================================================
// SubscriptionGateStep — decides whether this request is covered by an active
// subscription. Runs right after QuotaCheck (only depends on Subject), before
// routing. Writes req.BillingSource / req.SubscriptionID for UsageLog + Ledger.
// ============================================================================

// SubscriptionGateStep is inserted into both the main and resolved pipelines
// after quota_check. It is stateless and safe to share across pipelines.
//
// A lookup failure is an unknown billing state. It must fail closed before the
// upstream call instead of silently changing the request to pay-as-you-go.
type SubscriptionGateStep struct {
	Subs   Subscriptions
	Logger *zap.Logger
}

func (s *SubscriptionGateStep) Name() string { return "subscription_gate" }

func (s *SubscriptionGateStep) Execute(ctx context.Context, req *Request) error {
	// Default is always pay-as-you-go so the billing_source column is never
	// left empty (CHECK IN ('payg','subscription')).
	req.BillingSource = subscription.BillingSourcePayg
	req.SubscriptionID = ""

	if s.Subs == nil {
		return nil
	}
	if req.ExecutionMode == coreruntime.ExecutionModeAsync {
		return nil
	}
	subject := req.RuntimeSubject()
	// Only end-user–scoped requests with a tenant can carry a subscription;
	// tenant-owned keys (userMicro==0) never touch this path.
	if subject == nil || subject.Scope != coreidentity.ScopeUser ||
		subject.TenantID == "" || subject.UserID == "" {
		return nil
	}

	decision, err := s.Subs.ResolveForGate(ctx, subject.TenantID, subject.UserID)
	if err != nil {
		s.logger().Warn("subscription gate resolve failed",
			zap.Error(err),
			zap.String("request_id", req.RequestID),
			zap.String("tenant_id", subject.TenantID),
			zap.String("user_id", subject.UserID),
		)
		return apiError(503, "subscription_state_unavailable", "unable to determine subscription billing state")
	}
	if !decision.Covered {
		return nil
	}

	// 覆盖判定通过后再校验路由约束前提；任一命中则保持 payg（按量放行）：
	//   a) 存量无快照订阅（GroupQuotaDebitMultipliers 为空）——二期前售出、不参与硬路由约束；
	//   b) 显式指定套餐外分组（ForcedGroupID）——视为主动离开套餐；
	//   c) key 绑定分组不在套餐集内——直接按量。
	weights := decision.GroupQuotaDebitMultipliers
	if len(weights) == 0 {
		return nil
	}
	if subject.ForcedGroupID != "" {
		if _, ok := weights[subject.ForcedGroupID]; !ok {
			return nil
		}
	}
	if subject.GroupID != "" {
		if _, ok := weights[subject.GroupID]; !ok {
			return nil
		}
	}

	req.BillingSource = subscription.BillingSourceSubscription
	req.SubscriptionID = decision.SubscriptionID
	req.SubscriptionGroupQuotaDebitMultipliers = weights
	return nil
}

// Rollback is a no-op: the gate step has no side effects.
func (s *SubscriptionGateStep) Rollback(_ context.Context, _ *Request) {}

func (s *SubscriptionGateStep) logger() *zap.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return zap.L()
}

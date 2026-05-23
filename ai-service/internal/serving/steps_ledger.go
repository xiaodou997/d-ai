package serving

import (
	"context"

	"go.uber.org/zap"

	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/ledger"
)

// LedgerStep is a pipeline Finalizer that accrues the resolved per-request
// micro-credit cost into the local aggregation ledger
// (ai_user_credit_ledger.pending_*_micro).
//
// It runs unconditionally after every pipeline outcome. On early-failure paths
// (auth/quota/routing) BillingResult is zero and ledger.AddCharge no-ops. On
// success it always writes the row even if the request itself failed mid-stream
// — per design "失败计费：按真实 usage 扣".
//
// The actual URM Consume call happens out-of-band in the settle worker; this
// finalizer only persists the local pending row.
type LedgerStep struct {
	Ledger *ledger.Ledger
	// Trigger is optional. When non-nil, LedgerStep performs a non-blocking
	// send after a successful AddCharge so the settle worker can opportunistically
	// drain large pending balances. The channel should be buffered (cap >= 1).
	Trigger chan<- struct{}
}

func (s *LedgerStep) Name() string { return "ledger" }

func (s *LedgerStep) Finalize(ctx context.Context, req *Request) {
	if s.Ledger == nil {
		return
	}
	identity := req.RuntimeIdentity()
	if identity == nil || identity.TenantID == "" {
		return
	}

	tenantMicro := req.BillingResult.PlatformCost
	userMicro := req.BillingResult.UserCost
	if tenantMicro <= 0 && userMicro <= 0 {
		return
	}

	userID := identity.UserID
	if identity.OwnerType == domain.OwnerTenant {
		// Tenant-owned keys never carry per-user cost — keep the ledger
		// invariant clean regardless of upstream pricing config.
		userMicro = 0
		userID = ""
	}

	if err := s.Ledger.AddCharge(ctx, ledger.AddChargeParams{
		RequestID:   req.RequestID,
		OwnerType:   identity.OwnerType,
		TenantID:    identity.TenantID,
		UserID:      userID,
		TenantMicro: tenantMicro,
		UserMicro:   userMicro,
	}); err != nil {
		zap.L().Warn("ledger add charge failed",
			zap.Error(err),
			zap.String("request_id", req.RequestID),
			zap.String("tenant_id", identity.TenantID),
		)
		return
	}

	if s.Trigger != nil {
		select {
		case s.Trigger <- struct{}{}:
		default:
		}
	}
}

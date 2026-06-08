package ledger

import "xiaodou/unihub/ai-service/internal/domain"

// settleAction is the pure-math decision of "given the current ledger state,
// what should the next settle round push to URM and what leftover should we
// keep in the row?". Extracted from SettleOne so it can be unit-tested
// without a database.
type settleAction struct {
	NoOp                bool  // both sides floor to 0 — skip URM call entirely
	SettleTenantMicro   int64 // micro-credits to deduct this round (= multiple of 10000)
	SettleUserMicro     int64
	TenantCredits       int64 // = SettleTenantMicro / 10000
	UserCredits         int64
	LeftoverTenantMicro int64 // remainder kept for next window (< 10000 typically)
	LeftoverUserMicro   int64
}

// computeSettleAction implements the floor-and-keep-remainder logic.
// pendingTenant / pendingUser are the current ledger pending_*_micro values.
//
// If reuseWindowTenant/reuseWindowUser > 0 (i.e. a previous attempt persisted
// a window but crashed before URM finalized), those values are returned
// verbatim so the retry replays the exact same Consume call (idempotent).
func computeSettleAction(pendingTenant, pendingUser, reuseWindowTenant, reuseWindowUser int64) settleAction {
	var settleT, settleU int64
	if reuseWindowTenant > 0 || reuseWindowUser > 0 {
		settleT = reuseWindowTenant
		settleU = reuseWindowUser
	} else {
		settleT = domain.MicroToCreditsFloor(pendingTenant) * domain.MicroCreditsPerCredit
		settleU = domain.MicroToCreditsFloor(pendingUser) * domain.MicroCreditsPerCredit
	}

	tc := settleT / domain.MicroCreditsPerCredit
	uc := settleU / domain.MicroCreditsPerCredit

	return settleAction{
		NoOp:                tc == 0 && uc == 0,
		SettleTenantMicro:   settleT,
		SettleUserMicro:     settleU,
		TenantCredits:       tc,
		UserCredits:         uc,
		LeftoverTenantMicro: pendingTenant - settleT,
		LeftoverUserMicro:   pendingUser - settleU,
	}
}

package serving

import (
	"context"
	"net/http"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/subscription"
)

// AccountBalance is the live spendable USD balance used for request admission.
// AvailableMicroUSD excludes expired or unavailable balance lots. Debt is kept
// separate because a recharge must clear it before new balance becomes usable.
type AccountBalance struct {
	AvailableMicroUSD int64
	DebtMicroUSD      int64
}

// AccountBalanceResolver reads the current tenant and end-user account state.
// The check is deliberately side-effect free so the same step can run for
// synchronous execution and asynchronous task admission.
type AccountBalanceResolver interface {
	ResolveTenantBalance(ctx context.Context, tenantID string) (AccountBalance, error)
	ResolveUserBalance(ctx context.Context, tenantID, userID string) (AccountBalance, error)
}

// BalanceGateStep rejects an unpaid caller before any upstream work starts.
// Tenant balance is always required because the tenant pays the platform cost.
// A subscription-covered end user does not need PAYG balance, but existing user
// debt still blocks service until it is cleared.
type BalanceGateStep struct {
	Resolver AccountBalanceResolver
}

func (s *BalanceGateStep) Name() string { return "balance_gate" }

func (s *BalanceGateStep) Execute(ctx context.Context, req *Request) error {
	if s == nil || s.Resolver == nil {
		return apiError(http.StatusServiceUnavailable, "balance_state_unavailable",
			"unable to determine account balance")
	}
	subject := req.RuntimeSubject()
	if subject == nil || subject.TenantID == "" {
		return nil
	}

	tenantBalance, err := s.Resolver.ResolveTenantBalance(ctx, subject.TenantID)
	if err != nil {
		return apiErrorWithCause(http.StatusServiceUnavailable, "balance_state_unavailable",
			"unable to determine tenant balance", err)
	}
	if tenantBalance.DebtMicroUSD > 0 {
		return apiError(http.StatusPaymentRequired, "insufficient_balance",
			"tenant account has outstanding debt; recharge before continuing")
	}
	if tenantBalance.AvailableMicroUSD <= 0 {
		return apiError(http.StatusPaymentRequired, "insufficient_balance",
			"tenant USD balance is exhausted")
	}

	if subject.Scope != coreidentity.ScopeUser || subject.UserID == "" {
		return nil
	}
	userBalance, err := s.Resolver.ResolveUserBalance(ctx, subject.TenantID, subject.UserID)
	if err != nil {
		return apiErrorWithCause(http.StatusServiceUnavailable, "balance_state_unavailable",
			"unable to determine user balance", err)
	}
	if userBalance.DebtMicroUSD > 0 {
		return apiError(http.StatusPaymentRequired, "insufficient_balance",
			"user account has outstanding debt; recharge before continuing")
	}
	if req.BillingSource != subscription.BillingSourceSubscription && userBalance.AvailableMicroUSD <= 0 {
		return apiError(http.StatusPaymentRequired, "insufficient_balance",
			"user USD balance is exhausted")
	}
	return nil
}

func (s *BalanceGateStep) Rollback(_ context.Context, _ *Request) {}

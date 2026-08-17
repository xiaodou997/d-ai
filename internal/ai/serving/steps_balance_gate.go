package serving

import (
	"context"
	"net/http"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/subscription"
)

// AccountBalances carries the signed spendable balance of the two accounts a
// request can draw on. Negative means the account owes money; there is no
// separate debt field, because a balance below zero already says so.
type AccountBalances struct {
	TenantMicroUSD int64
	UserMicroUSD   int64
	// UserPresent distinguishes "the user has no balance" from "this request
	// has no user account", which matters because only the former is a refusal.
	UserPresent bool
}

// AccountBalanceResolver reads live balances for admission. One call returns
// both accounts so the gate can never compare numbers read at different
// instants, and so display paths and the gate share a single query shape.
//
// The check is side-effect free, which lets the same step run for synchronous
// execution and for async task admission.
type AccountBalanceResolver interface {
	ResolveBalances(ctx context.Context, tenantID, userID string) (AccountBalances, error)
}

// BalanceGateStep refuses an unfunded caller before any upstream work starts.
//
// The rule is the whole rule: a request is admitted while the balance is
// strictly positive. Settlement is free to take that balance negative — a
// request already served must be recorded — and the next request is what stops.
// Bounding the overshoot is not this step's job and needs no credit limit: the
// gap is one round of in-flight requests, which is small because each request
// is small.
//
// The tenant always pays the platform, so tenant balance is always required. A
// subscription-covered end user draws on plan quota rather than PAYG balance,
// so only their tenant is checked.
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
		return nil // internal/untenanted traffic is not billed
	}

	userID := ""
	if subject.Scope == coreidentity.ScopeUser {
		userID = subject.UserID
	}

	balances, err := s.Resolver.ResolveBalances(ctx, subject.TenantID, userID)
	if err != nil {
		return apiErrorWithCause(http.StatusServiceUnavailable, "balance_state_unavailable",
			"unable to determine account balance", err)
	}
	if balances.TenantMicroUSD <= 0 {
		return apiError(http.StatusPaymentRequired, "insufficient_balance",
			"tenant USD balance is exhausted; recharge before continuing")
	}
	if userID == "" {
		return nil
	}
	if !balances.UserPresent {
		return apiError(http.StatusServiceUnavailable, "balance_state_unavailable",
			"unable to determine user balance")
	}
	// A subscription covers what a request costs, not what the user already
	// owes: plan quota pays for new usage, but a negative balance is settled
	// debt and still has to be cleared with money.
	if req.BillingSource == subscription.BillingSourceSubscription {
		if balances.UserMicroUSD < 0 {
			return apiError(http.StatusPaymentRequired, "insufficient_balance",
				"user account has outstanding debt; recharge before continuing")
		}
		return nil
	}
	if balances.UserMicroUSD <= 0 {
		return apiError(http.StatusPaymentRequired, "insufficient_balance",
			"user USD balance is exhausted; recharge before continuing")
	}
	return nil
}

func (s *BalanceGateStep) Rollback(_ context.Context, _ *Request) {}

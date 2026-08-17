package serving

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/subscription"
)

type fakeAccountBalanceResolver struct {
	balances AccountBalances
	err      error
	calls    int
	lastUser string
}

func (r *fakeAccountBalanceResolver) ResolveBalances(_ context.Context, _, userID string) (AccountBalances, error) {
	r.calls++
	r.lastUser = userID
	return r.balances, r.err
}

func balanceGateRequest(scope coreidentity.Scope, billingSource string) *Request {
	subject := &coreidentity.Subject{Scope: scope, TenantID: "tenant-1"}
	if scope == coreidentity.ScopeUser {
		subject.UserID = "user-1"
	}
	return &Request{Subject: subject, BillingSource: billingSource}
}

func requireBalanceGateError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != status || apiErr.Code != code {
		t.Fatalf("error = %#v, want status=%d code=%s", err, status, code)
	}
}

// Both balances come from one resolver call, so the gate can never compare a
// tenant balance and a user balance observed at different instants.
func TestBalanceGateReadsBothAccountsInOneCall(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		balances: AccountBalances{TenantMicroUSD: 1_000_000, UserMicroUSD: 1_000_000, UserPresent: true},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourcePayg))
	if err != nil {
		t.Fatalf("funded request was rejected: %v", err)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
	if resolver.lastUser != "user-1" {
		t.Fatalf("resolver user = %q, want user-1", resolver.lastUser)
	}
}

func TestBalanceGateRejectsTenantInDebt(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		balances: AccountBalances{TenantMicroUSD: -1, UserMicroUSD: 1_000_000, UserPresent: true},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
}

func TestBalanceGateRejectsExhaustedTenant(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{balances: AccountBalances{TenantMicroUSD: 0}}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeTenant, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
}

// The overdraft the business accepts: settlement took the balance negative and
// the very next request is refused. This is the regression that kept recurring.
func TestBalanceGateRejectsUserAfterOverdraft(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		balances: AccountBalances{TenantMicroUSD: 1_000_000, UserMicroUSD: -500_000, UserPresent: true},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
}

func TestBalanceGateRejectsPaygUserWithoutBalance(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		balances: AccountBalances{TenantMicroUSD: 1_000_000, UserMicroUSD: 0, UserPresent: true},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
}

func TestBalanceGateAllowsSubscriptionUserWithoutPaygBalance(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		balances: AccountBalances{TenantMicroUSD: 1_000_000, UserMicroUSD: 0, UserPresent: true},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourceSubscription))
	if err != nil {
		t.Fatalf("subscription-covered user was rejected: %v", err)
	}
}

func TestBalanceGateRejectsSubscriptionUserInDebt(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		balances: AccountBalances{TenantMicroUSD: 1_000_000, UserMicroUSD: -1, UserPresent: true},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourceSubscription))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
}

func TestBalanceGateFailsClosedWhenBalanceCannotBeRead(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{err: errors.New("database unavailable")}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeTenant, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusServiceUnavailable, "balance_state_unavailable")
}

// A user-scoped request whose account row is missing is an unknown billing
// state, not a zero balance, and must fail closed rather than be refused as
// unfunded.
func TestBalanceGateFailsClosedWhenUserAccountMissing(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		balances: AccountBalances{TenantMicroUSD: 1_000_000, UserPresent: false},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusServiceUnavailable, "balance_state_unavailable")
}

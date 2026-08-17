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
	tenant    AccountBalance
	user      AccountBalance
	tenantErr error
	userErr   error
	userCalls int
}

func (r *fakeAccountBalanceResolver) ResolveTenantBalance(context.Context, string) (AccountBalance, error) {
	return r.tenant, r.tenantErr
}

func (r *fakeAccountBalanceResolver) ResolveUserBalance(context.Context, string, string) (AccountBalance, error) {
	r.userCalls++
	return r.user, r.userErr
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

func TestBalanceGateRejectsTenantDebtBeforeUserLookup(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		tenant: AccountBalance{AvailableMicroUSD: 1_000_000, DebtMicroUSD: 1},
		user:   AccountBalance{AvailableMicroUSD: 1_000_000},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
	if resolver.userCalls != 0 {
		t.Fatalf("user balance was queried %d time(s) after tenant rejection", resolver.userCalls)
	}
}

func TestBalanceGateRejectsExhaustedTenant(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{tenant: AccountBalance{}}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeTenant, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
}

func TestBalanceGateRejectsPaygUserWithoutBalance(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		tenant: AccountBalance{AvailableMicroUSD: 1_000_000},
		user:   AccountBalance{},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
}

func TestBalanceGateAllowsSubscriptionUserWithoutPaygBalance(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		tenant: AccountBalance{AvailableMicroUSD: 1_000_000},
		user:   AccountBalance{},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourceSubscription))
	if err != nil {
		t.Fatalf("subscription-covered user was rejected: %v", err)
	}
}

func TestBalanceGateRejectsSubscriptionUserDebt(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{
		tenant: AccountBalance{AvailableMicroUSD: 1_000_000},
		user:   AccountBalance{DebtMicroUSD: 1},
	}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeUser, subscription.BillingSourceSubscription))
	requireBalanceGateError(t, err, http.StatusPaymentRequired, "insufficient_balance")
}

func TestBalanceGateFailsClosedWhenBalanceCannotBeRead(t *testing.T) {
	resolver := &fakeAccountBalanceResolver{tenantErr: errors.New("database unavailable")}
	err := (&BalanceGateStep{Resolver: resolver}).Execute(context.Background(),
		balanceGateRequest(coreidentity.ScopeTenant, subscription.BillingSourcePayg))
	requireBalanceGateError(t, err, http.StatusServiceUnavailable, "balance_state_unavailable")
}

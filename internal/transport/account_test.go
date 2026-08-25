package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/auth"
	billingdomain "xiaodou/dai/internal/billing"
	billingports "xiaodou/dai/internal/billing/ports"
	"xiaodou/dai/internal/domain"
)

type accountQueryStub struct {
	tenantBalanceID string
	userBalanceID   string
	balanceDetail   bool
	rechargeQuery   billingports.RechargeRecordsQuery
	statsTenantID   string
	balanceErr      error
	listErr         error
	statsErr        error
}

func (s *accountQueryStub) GetTenantBalance(_ context.Context, tenantID string, detail bool) (*billingports.BalanceResponse, error) {
	s.tenantBalanceID = tenantID
	s.balanceDetail = detail
	return &billingports.BalanceResponse{Currency: "USD"}, s.balanceErr
}

func (s *accountQueryStub) GetUserBalance(_ context.Context, userID string, detail bool) (*billingports.BalanceResponse, error) {
	s.userBalanceID = userID
	s.balanceDetail = detail
	return &billingports.BalanceResponse{Currency: "USD"}, s.balanceErr
}

func (s *accountQueryStub) ListRechargeRecords(_ context.Context, query billingports.RechargeRecordsQuery) ([]billingports.RechargeRecordRow, int64, error) {
	s.rechargeQuery = query
	return []billingports.RechargeRecordRow{{OrderID: "ORD-1"}}, 1, s.listErr
}

func (s *accountQueryStub) GetAccountStats(_ context.Context, tenantID string) (*billingports.AccountStatsResult, error) {
	s.statsTenantID = tenantID
	return &billingports.AccountStatsResult{EndUserCount: 1}, s.statsErr
}

func accountClaims(userType int) context.Context {
	return context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{
		UserID: "user-1", TenantID: "tenant-1", UserType: userType,
	})
}

func TestAccountHandlersScopeQueriesThroughApplicationPort(t *testing.T) {
	queries := &accountQueryStub{}
	h := newAccountHandlers(queries)

	balance, err := h.balance(accountClaims(3), &accountBalanceInput{AccountType: 2, AccountID: "other", Detail: true})
	if err != nil || balance.Body.Currency != "USD" || queries.tenantBalanceID != "tenant-1" || !queries.balanceDetail {
		t.Fatalf("tenant balance = %#v, query = %#v, error = %v", balance, queries, err)
	}

	if _, err := h.balance(accountClaims(4), &accountBalanceInput{AccountType: 1, AccountID: "other"}); err != nil {
		t.Fatalf("user balance error = %v", err)
	}
	if queries.userBalanceID != "user-1" {
		t.Fatalf("user balance id = %q, want user-1", queries.userBalanceID)
	}

	records, err := h.rechargeRecords(accountClaims(4), &rechargeRecordsInput{TenantID: "other", UserID: "other", RechargeType: "1", Page: 1, Size: 10})
	if err != nil || records.Body.Total != 1 || records.Body.Items[0].OrderID != "ORD-1" {
		t.Fatalf("recharge records = %#v, error = %v", records, err)
	}
	if queries.rechargeQuery.TenantID != "tenant-1" || queries.rechargeQuery.UserID != "user-1" || len(queries.rechargeQuery.OrderTypes) != len(billingdomain.UserRechargeOrderTypes) {
		t.Fatalf("recharge scope = %#v", queries.rechargeQuery)
	}

	stats, err := h.stats(accountClaims(2), &accountStatsInput{AccountID: "tenant-2"})
	if err != nil || stats.Body.EndUserCount != 1 || queries.statsTenantID != "tenant-2" {
		t.Fatalf("account stats = %#v, query = %#v, error = %v", stats, queries, err)
	}
}

func TestAccountQueryErrorsMapToHTTPErrors(t *testing.T) {
	queries := &accountQueryStub{balanceErr: domain.ErrAccountNotFound}
	h := newAccountHandlers(queries)
	if _, err := h.balance(accountClaims(1), &accountBalanceInput{AccountType: 1, AccountID: "tenant-1"}); err == nil || err.Error() != "Not Found: 账户不存在" {
		t.Fatalf("not found error = %v", err)
	}
	if _, err := newAccountHandlers(nil).stats(accountClaims(1), &accountStatsInput{AccountID: "tenant-1"}); err == nil || err.Error() != "Service Unavailable: 账户查询服务不可用" {
		t.Fatalf("unavailable error = %v", err)
	}
}

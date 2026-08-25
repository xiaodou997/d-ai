package service

import (
	"context"
	"errors"
	"testing"

	billingports "xiaodou/dai/internal/billing/ports"
)

type accountQueryReaderStub struct {
	balance *billingports.BalanceResponse
	rows    []billingports.RechargeRecordRow
	stats   *billingports.AccountStatsResult
	query   billingports.RechargeRecordsQuery
}

func (s *accountQueryReaderStub) GetTenantBalance(context.Context, string, bool) (*billingports.BalanceResponse, error) {
	return s.balance, nil
}

func (s *accountQueryReaderStub) GetUserBalance(context.Context, string, bool) (*billingports.BalanceResponse, error) {
	return s.balance, nil
}

func (s *accountQueryReaderStub) ListRechargeRecords(_ context.Context, query billingports.RechargeRecordsQuery) ([]billingports.RechargeRecordRow, int64, error) {
	s.query = query
	return s.rows, int64(len(s.rows)), nil
}

func (s *accountQueryReaderStub) GetAccountStats(context.Context, string) (*billingports.AccountStatsResult, error) {
	return s.stats, nil
}

func TestAccountQueryServiceDelegatesReadUseCases(t *testing.T) {
	reader := &accountQueryReaderStub{
		balance: &billingports.BalanceResponse{Currency: "USD"},
		rows:    []billingports.RechargeRecordRow{{OrderID: "ORD-1"}},
		stats:   &billingports.AccountStatsResult{EndUserCount: 2},
	}
	service := NewAccountQueryService(reader)

	if got, err := service.GetTenantBalance(context.Background(), "tenant-1", true); err != nil || got.Currency != "USD" {
		t.Fatalf("GetTenantBalance() = %#v, error = %v", got, err)
	}
	if got, total, err := service.ListRechargeRecords(context.Background(), billingports.RechargeRecordsQuery{TenantID: "tenant-1"}); err != nil || total != 1 || got[0].OrderID != "ORD-1" {
		t.Fatalf("ListRechargeRecords() = %#v/%d, error = %v", got, total, err)
	}
	if reader.query.Page != 1 || reader.query.Size != 20 {
		t.Fatalf("normalized recharge query = %#v", reader.query)
	}
	if got, err := service.GetAccountStats(context.Background(), "tenant-1"); err != nil || got.EndUserCount != 2 {
		t.Fatalf("GetAccountStats() = %#v, error = %v", got, err)
	}
}

func TestAccountQueryServiceReportsMissingReader(t *testing.T) {
	service := NewAccountQueryService(nil)
	if _, err := service.GetUserBalance(context.Background(), "user-1", false); !errors.Is(err, billingports.ErrAccountQueryUnavailable) {
		t.Fatalf("GetUserBalance() error = %v", err)
	}
}

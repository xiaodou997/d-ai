package service

import (
	"context"

	billingports "xiaodou/dai/internal/billing/ports"
)

// AccountQueryService is the application boundary for Portal account views.
// It deliberately exposes only read use cases while the PostgreSQL adapter
// remains responsible for ledger and projection joins.
type AccountQueryService struct {
	reader billingports.AccountQueryReader
}

var _ billingports.AccountQueryReader = (*AccountQueryService)(nil)

func NewAccountQueryService(reader billingports.AccountQueryReader) *AccountQueryService {
	return &AccountQueryService{reader: reader}
}

func (s *AccountQueryService) GetTenantBalance(ctx context.Context, tenantID string, detail bool) (*billingports.BalanceResponse, error) {
	if s == nil || s.reader == nil {
		return nil, billingports.ErrAccountQueryUnavailable
	}
	return s.reader.GetTenantBalance(ctx, tenantID, detail)
}

func (s *AccountQueryService) GetUserBalance(ctx context.Context, userID string, detail bool) (*billingports.BalanceResponse, error) {
	if s == nil || s.reader == nil {
		return nil, billingports.ErrAccountQueryUnavailable
	}
	return s.reader.GetUserBalance(ctx, userID, detail)
}

func (s *AccountQueryService) ListRechargeRecords(ctx context.Context, query billingports.RechargeRecordsQuery) ([]billingports.RechargeRecordRow, int64, error) {
	if s == nil || s.reader == nil {
		return nil, 0, billingports.ErrAccountQueryUnavailable
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Size < 1 || query.Size > 100 {
		query.Size = 20
	}
	return s.reader.ListRechargeRecords(ctx, query)
}

func (s *AccountQueryService) GetAccountStats(ctx context.Context, tenantID string) (*billingports.AccountStatsResult, error) {
	if s == nil || s.reader == nil {
		return nil, billingports.ErrAccountQueryUnavailable
	}
	return s.reader.GetAccountStats(ctx, tenantID)
}

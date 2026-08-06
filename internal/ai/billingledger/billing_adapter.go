package billingledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	billingsvc "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/domain"
)

// BillingLeaseAdapter connects the AI usage coordinator to the billing domain.
type BillingLeaseAdapter struct {
	leases   *billingsvc.CreditLeaseService
	clientID string
}

func NewBillingLeaseAdapter(leases *billingsvc.CreditLeaseService, clientID string) *BillingLeaseAdapter {
	return &BillingLeaseAdapter{leases: leases, clientID: clientID}
}

func (p *BillingLeaseAdapter) AcquireCreditLease(ctx context.Context, req AcquireLease) (*CreditLease, error) {
	lease, err := p.leases.Acquire(ctx, billingsvc.AcquireLeaseParams{
		ClientID:             p.clientID,
		ClientWindowID:       req.ClientWindowID,
		TenantID:             req.TenantID,
		UserID:               req.UserID,
		Description:          req.Description,
		RequestedTenantMicro: req.RequestedTenantMicro,
		RequestedUserMicro:   req.RequestedUserMicro,
		TTL:                  time.Duration(req.TTLSeconds) * time.Second,
		Grace:                time.Duration(req.GraceSeconds) * time.Second,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return leaseToResponse(lease), nil
}

func (p *BillingLeaseAdapter) RenewCreditLease(ctx context.Context, leaseID string, req RenewLease) (*CreditLease, error) {
	lease, err := p.leases.Renew(ctx, billingsvc.RenewLeaseParams{
		LeaseID:  leaseID,
		ClientID: p.clientID,
		Version:  req.Version,
		TTL:      time.Duration(req.TTLSeconds) * time.Second,
		Grace:    time.Duration(req.GraceSeconds) * time.Second,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return leaseToResponse(lease), nil
}

func (p *BillingLeaseAdapter) SettleCreditLease(ctx context.Context, leaseID string, req SettleLease) (*CreditLease, error) {
	lease, err := p.leases.Settle(ctx, billingsvc.SettleLeaseParams{
		LeaseID:           leaseID,
		ClientID:          p.clientID,
		SettlementID:      req.SettlementID,
		ActualTenantMicro: req.ActualTenantMicro,
		ActualUserMicro:   req.ActualUserMicro,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return leaseToResponse(lease), nil
}

func (p *BillingLeaseAdapter) GetCreditLease(ctx context.Context, leaseID string) (*CreditLease, error) {
	lease, err := p.leases.Get(ctx, leaseID, p.clientID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return leaseToResponse(lease), nil
}

// ─── 映射 ─────────────────────────────────────────────

func leaseToResponse(l *billingsvc.CreditLease) *CreditLease {
	if l == nil {
		return nil
	}
	resp := &CreditLease{
		LeaseID:              l.LeaseID,
		ClientWindowID:       l.ClientWindowID,
		TenantID:             l.TenantID,
		UserID:               l.UserID,
		GrantedTenantMicro:   l.GrantedTenantMicro,
		GrantedUserMicro:     l.GrantedUserMicro,
		EscrowState:          l.EscrowState,
		SettlementState:      l.SettlementState,
		Version:              l.Version,
		ExpiresAt:            l.ExpiresAt,
		GraceUntil:           l.GraceUntil,
		SettlementID:         l.SettlementID,
		ActualTenantMicro:    l.ActualTenantMicro,
		ActualUserMicro:      l.ActualUserMicro,
		SettledEventID:       l.SettledEventID,
		SettledAt:            l.SettledAt,
		TenantDeductedMicro:  l.TenantDeducted,
		UserDeductedMicro:    l.UserDeducted,
		TenantDebtAddedMicro: l.TenantDebtAdded,
		UserDebtAddedMicro:   l.UserDebtAdded,
		AccountState:         AccountState(l.AccountState),
		AllowFurtherUsage:    l.AllowFurtherUsage,
	}
	return resp
}

// mapServiceError 将 billing service 的 domain error 映射为
// billingledger 期望的错误语义（余额不足 / 冲突 / 不可用）。
func mapServiceError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrInsufficientBalance),
		errors.Is(err, domain.ErrTenantInsufficientBalance),
		errors.Is(err, domain.ErrUserInsufficientBalance),
		errors.Is(err, domain.ErrTenantInOverdraft),
		errors.Is(err, domain.ErrUserInOverdraft),
		errors.Is(err, domain.ErrTenantOverdraftExceeded),
		errors.Is(err, domain.ErrUserOverdraftExceeded):
		return fmt.Errorf("%w: %w", ErrInsufficientBalance, err)
	case errors.Is(err, domain.ErrCreditLeaseVersion),
		errors.Is(err, domain.ErrCreditLeaseSettlement):
		return fmt.Errorf("%w: %w", ErrAdmissionConflict, err)
	case errors.Is(err, domain.ErrCreditLeaseNotFound),
		errors.Is(err, domain.ErrCreditLeaseNotRenewable),
		errors.Is(err, domain.ErrForbidden),
		errors.Is(err, domain.ErrBadRequest),
		errors.Is(err, domain.ErrInvalidAmount):
		return fmt.Errorf("%w: %w", ErrProtocolViolation, err)
	default:
		return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}
}

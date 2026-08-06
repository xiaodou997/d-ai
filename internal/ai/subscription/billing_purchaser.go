package subscription

import (
	"context"
	"errors"
	"fmt"

	billingsvc "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/domain"
)

// BillingPurchaser connects subscription purchases to the billing domain.
type BillingPurchaser struct {
	deduction *billingsvc.DeductionService
	clientID  string
}

func NewBillingPurchaser(deduction *billingsvc.DeductionService, clientID string) *BillingPurchaser {
	return &BillingPurchaser{deduction: deduction, clientID: clientID}
}

func (p *BillingPurchaser) DebitStrict(ctx context.Context, req DebitRequest) (*DebitReceipt, error) {
	res, err := p.deduction.Consume(billingsvc.ConsumeParams{
		IdempotencyKey:    req.IdempotencyKey,
		ClientID:          p.clientID,
		TenantID:          req.TenantID,
		UserID:            req.UserID,
		Description:       req.Description,
		TenantAmount:      req.TenantMicro,
		UserAmount:        req.UserMicro,
		DisallowOverdraft: true,
	})
	if err != nil {
		return nil, mapDeductionError(err)
	}
	return &DebitReceipt{AuthorizationID: res.EventID}, nil
}

func mapDeductionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrInsufficientBalance) ||
		errors.Is(err, domain.ErrTenantInsufficientBalance) ||
		errors.Is(err, domain.ErrUserInsufficientBalance) ||
		errors.Is(err, domain.ErrTenantInOverdraft) ||
		errors.Is(err, domain.ErrUserInOverdraft) ||
		errors.Is(err, domain.ErrTenantOverdraftExceeded) ||
		errors.Is(err, domain.ErrUserOverdraftExceeded) {
		return fmt.Errorf("%w: %w", ErrInsufficientBalance, err)
	}
	return err
}

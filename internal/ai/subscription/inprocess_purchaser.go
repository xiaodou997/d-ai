package subscription

import (
	"context"
	"fmt"

	"xiaodou/dai/internal/ai/platform"
	billingsvc "xiaodou/dai/internal/billing/service"
)

// InProcessPurchaser 直接连接订阅购买与统一计费域。
type InProcessPurchaser struct {
	deduction *billingsvc.DeductionService
	clientID  string
}

func NewInProcessPurchaser(deduction *billingsvc.DeductionService, clientID string) *InProcessPurchaser {
	return &InProcessPurchaser{deduction: deduction, clientID: clientID}
}

func (p *InProcessPurchaser) DebitStrict(ctx context.Context, req platform.StrictDebitRequest) (*platform.StrictDebitResponse, error) {
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
	return &platform.StrictDebitResponse{
		AuthorizationID:      res.EventID,
		TenantDeductedMicro:  res.TenantDeducted,
		UserDeductedMicro:    res.UserDeducted,
		TenantDebtAddedMicro: res.TenantOverdraftAdd,
		UserDebtAddedMicro:   res.UserOverdraftAdd,
		AccountState:         platform.AccountState(res.AccountState),
		AllowFurtherUsage:    res.AllowFurtherUsage,
	}, nil
}

func mapDeductionError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if contains(msg, "insufficient") || contains(msg, "overdraft") {
		return fmt.Errorf("%w: %s", platform.ErrInsufficientBalance, msg)
	}
	return err
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return len(substr) == 0
}

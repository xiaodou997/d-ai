package subscription

import (
	"context"
	"fmt"

	"xiaodou/dai/internal/ai/urm"
	billingsvc "xiaodou/dai/internal/billing/service"
)

// InProcessPurchaser 是 Purchaser 接口的进程内实现——直接调用
// billingsvc.DeductionService.Consume()，不再走 HTTP。
//
// 原 ai-service 通过 urm.Client.DebitStrict HTTP 调用
// urm-service 的 /internal/v2/ledger/debits。合并后直接函数调用。
type InProcessPurchaser struct {
	deduction *billingsvc.DeductionService
	clientID  string
}

func NewInProcessPurchaser(deduction *billingsvc.DeductionService, clientID string) *InProcessPurchaser {
	return &InProcessPurchaser{deduction: deduction, clientID: clientID}
}

func (p *InProcessPurchaser) DebitStrict(ctx context.Context, req urm.StrictDebitRequest) (*urm.StrictDebitResponse, error) {
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
	return &urm.StrictDebitResponse{
		AuthorizationID:      res.EventID,
		TenantDeductedMicro:  res.TenantDeducted,
		UserDeductedMicro:    res.UserDeducted,
		TenantDebtAddedMicro: res.TenantOverdraftAdd,
		UserDebtAddedMicro:   res.UserOverdraftAdd,
		AccountState:         urm.AccountState(res.AccountState),
		AllowFurtherUsage:    res.AllowFurtherUsage,
	}, nil
}

func mapDeductionError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if contains(msg, "insufficient") || contains(msg, "overdraft") {
		return fmt.Errorf("%w: %s", urm.ErrInsufficientBalance, msg)
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

package billingledger

import (
	"context"
	"fmt"
	"time"

	"xiaodou/dai/internal/ai/urm"
	billingsvc "xiaodou/dai/internal/billing/service"
)

// InProcessLeasePort 是 LeasePort 的进程内实现——直接调用
// billingsvc.CreditLeaseService，不再走 HTTP。
//
// 原 ai-service 通过 urm.Client.HTTP 调用 urm-service 的
// /internal/v3/ledger/leases 系列端点。合并后同一进程，
// 直接函数调用，消除网络延迟和序列化开销。
type InProcessLeasePort struct {
	leases   *billingsvc.CreditLeaseService
	clientID string
}

func NewInProcessLeasePort(leases *billingsvc.CreditLeaseService, clientID string) *InProcessLeasePort {
	return &InProcessLeasePort{leases: leases, clientID: clientID}
}

func (p *InProcessLeasePort) AcquireCreditLease(ctx context.Context, req urm.AcquireCreditLeaseRequest) (*urm.CreditLeaseResponse, error) {
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

func (p *InProcessLeasePort) RenewCreditLease(ctx context.Context, leaseID string, req urm.RenewCreditLeaseRequest) (*urm.CreditLeaseResponse, error) {
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

func (p *InProcessLeasePort) SettleCreditLease(ctx context.Context, leaseID string, req urm.SettleCreditLeaseRequest) (*urm.CreditLeaseResponse, error) {
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

func (p *InProcessLeasePort) GetCreditLease(ctx context.Context, leaseID string) (*urm.CreditLeaseResponse, error) {
	lease, err := p.leases.Get(ctx, leaseID, p.clientID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return leaseToResponse(lease), nil
}

// ─── 映射 ─────────────────────────────────────────────

func leaseToResponse(l *billingsvc.CreditLease) *urm.CreditLeaseResponse {
	if l == nil {
		return nil
	}
	resp := &urm.CreditLeaseResponse{
		LeaseID:            l.LeaseID,
		ClientWindowID:     l.ClientWindowID,
		TenantID:           l.TenantID,
		UserID:             l.UserID,
		GrantedTenantMicro: l.GrantedTenantMicro,
		GrantedUserMicro:   l.GrantedUserMicro,
		EscrowState:        l.EscrowState,
		SettlementState:    l.SettlementState,
		Version:            l.Version,
		ExpiresAt:          l.ExpiresAt,
		GraceUntil:         l.GraceUntil,
		SettlementID:       l.SettlementID,
		ActualTenantMicro:  l.ActualTenantMicro,
		ActualUserMicro:    l.ActualUserMicro,
		SettledEventID:     l.SettledEventID,
		SettledAt:          l.SettledAt,
		TenantDeductedMicro: l.TenantDeducted,
		UserDeductedMicro:   l.UserDeducted,
		TenantDebtAddedMicro: l.TenantDebtAdded,
		UserDebtAddedMicro:   l.UserDebtAdded,
		AccountState:        urm.AccountState(l.AccountState),
		AllowFurtherUsage:   l.AllowFurtherUsage,
	}
	return resp
}

// mapServiceError 将 billing service 的 domain error 映射为
// billingledger 期望的错误语义（余额不足 / 冲突 / 不可用）。
func mapServiceError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case contains(msg, "insufficient") || contains(msg, "overdraft"):
		return fmt.Errorf("%w: %s", ErrInsufficientBalance, msg)
	case contains(msg, "conflict") || contains(msg, "version"):
		return fmt.Errorf("%w: %s", ErrAdmissionConflict, msg)
	case contains(msg, "not found"):
		return fmt.Errorf("%w: %s", ErrProtocolViolation, msg)
	default:
		return fmt.Errorf("%w: %s", ErrDependencyUnavailable, msg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

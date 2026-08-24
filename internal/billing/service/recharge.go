package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/ledger"
	shared "xiaodou/dai/internal/domain"
)

// ManualRechargeTargetLocker locks and validates the identity target inside
// the same transaction that creates the recharge order and credit lot.
//
// The billing application service owns the transaction; the identity adapter
// owns the iam_* query. This keeps the row-lock boundary out of HTTP without
// making billing depend on a concrete tenant repository.
type ManualRechargeTargetLocker interface {
	LockManualRechargeTarget(ctx context.Context, tx pgx.Tx, tenantID, userID string) error
}

// RechargeService owns manual recharge lifecycle transactions. Payment
// settlement deliberately keeps using GrantBalance with its payment-order
// transaction because it must also update pay_orders and the cash ledger.
type RechargeService struct {
	pool         *pgxpool.Pool
	targetLocker ManualRechargeTargetLocker
}

func NewRechargeService(pool *pgxpool.Pool, targetLocker ManualRechargeTargetLocker) *RechargeService {
	return &RechargeService{pool: pool, targetLocker: targetLocker}
}

// GrantParams records a USD balance grant and creates one expiring or permanent
// balance lot for it.
//
// 由外部事务传入 tx，内部不 Begin/Commit —— 微信支付 Settle 使用它把支付订单、
// 充值订单、额度包和现金流水放在同一事务内。人工充值请使用 RechargeService.GrantManual。
type GrantParams struct {
	OrderType      string // billing.OrderType*
	TenantID       string
	UserID         string // 为空表示租户充值
	AmountMicroUSD int64
	PaidAmount     int64
	PaymentRef     string
	PaymentOrderID string
	Note           string
	OperatorID     string
	Source         string // billing.PackageSource*
	ExpiresAt      *time.Time
}

// GrantResult 记账结果。
type GrantResult struct {
	OrderID             string
	BalanceLotID        string // empty when the entire grant cleared debt
	ClearedDebtMicroUSD int64
	LotAmountMicroUSD   int64
	OrderTime           time.Time
}

// GrantBalance 在外部事务内写充值订单并把金额加到账户余额上。
//
// 负余额不需要任何特殊处理：余额是一个有符号数，加钱这一个动作同时完成了「清欠」
// 和「充值」。ledger 只为真正变成可用余额的那部分建额度包，已经被过去用量消耗掉的
// 部分不会重新变成可花的钱。
func GrantBalance(ctx context.Context, tx pgx.Tx, p GrantParams) (*GrantResult, error) {
	if err := validateGrantParams(p); err != nil {
		return nil, err
	}

	now := billing.NowUTC()
	orderID := "ORD_" + uuid.New().String()[:24]

	var userIDVal, paymentRefVal, paymentOrderIDVal, noteVal any
	if p.UserID != "" {
		userIDVal = p.UserID
	}
	if p.PaymentRef != "" {
		paymentRefVal = p.PaymentRef
	}
	if p.PaymentOrderID != "" {
		paymentOrderIDVal = p.PaymentOrderID
	}
	if p.Note != "" {
		noteVal = p.Note
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_recharge_orders
		(order_id, order_type, tenant_id, user_id, credit_amount, paid_amount,
		 payment_ref, payment_order_id, expires_at, operator_id, note, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active', $12)
	`, orderID, p.OrderType, p.TenantID, userIDVal, p.AmountMicroUSD, p.PaidAmount,
		paymentRefVal, paymentOrderIDVal, p.ExpiresAt, p.OperatorID, noteVal, now); err != nil {
		return nil, err
	}

	ref := GrantRef(p.TenantID, p.UserID)
	balanceBefore, err := ledger.Balance(ctx, tx, ref)
	if err != nil {
		return nil, err
	}
	lotID, err := ledger.Grant(ctx, tx, ref, p.AmountMicroUSD, p.ExpiresAt, p.Source, orderID)
	if err != nil {
		return nil, err
	}

	clearedDebt := int64(0)
	if balanceBefore < 0 {
		clearedDebt = min(-balanceBefore, p.AmountMicroUSD)
	}

	return &GrantResult{
		OrderID:             orderID,
		BalanceLotID:        lotID,
		ClearedDebtMicroUSD: clearedDebt,
		LotAmountMicroUSD:   p.AmountMicroUSD - clearedDebt,
		OrderTime:           now,
	}, nil
}

// GrantManual creates a platform_to_tenant or tenant_to_user recharge and its
// balance lot atomically. The target row is locked before GrantBalance runs,
// so a concurrent account disable/delete cannot pass the HTTP preflight and
// still receive a grant.
func (s *RechargeService) GrantManual(ctx context.Context, p GrantParams) (*GrantResult, error) {
	if err := validateGrantParams(p); err != nil {
		return nil, err
	}
	if p.OrderType != billing.OrderTypePlatformToTenant && p.OrderType != billing.OrderTypeTenantToUser {
		return nil, grantValidationError("manual recharge requires a manual order type")
	}
	if s == nil || s.pool == nil || s.targetLocker == nil {
		return nil, fmt.Errorf("manual recharge target locker is not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := s.targetLocker.LockManualRechargeTarget(ctx, tx, p.TenantID, p.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if p.UserID != "" {
				return nil, grantValidationError("用户不存在、已删除或已停用")
			}
			return nil, grantValidationError("目标租户不存在")
		}
		return nil, err
	}

	grant, err := GrantBalance(ctx, tx, p)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return grant, nil
}

func validateGrantParams(p GrantParams) error {
	if p.TenantID == "" {
		return grantValidationError("tenant id is required")
	}
	if p.OperatorID == "" {
		return grantValidationError("operator id is required")
	}
	if p.AmountMicroUSD <= 0 {
		return shared.ErrInvalidAmount
	}
	if p.PaidAmount < 0 {
		return grantValidationError("paid amount must not be negative")
	}

	online := false
	expectedSource := ""
	switch p.OrderType {
	case billing.OrderTypePlatformToTenant:
		expectedSource = billing.PackageSourceAdminRecharge
		if p.UserID != "" || p.PaymentOrderID != "" {
			return grantValidationError("platform recharge cannot target a user or payment order")
		}
	case billing.OrderTypeTenantToUser:
		expectedSource = billing.PackageSourceTenantRecharge
		if p.UserID == "" || p.PaymentOrderID != "" {
			return grantValidationError("tenant recharge requires a user and cannot link a payment order")
		}
	case billing.OrderTypeOnlineUserTopup:
		online = true
		expectedSource = billing.PackageSourceOnlineTopup
		if p.UserID == "" || p.PaymentOrderID == "" {
			return grantValidationError("online user recharge requires a user and payment order")
		}
	case billing.OrderTypeOnlineTenantTopup:
		online = true
		expectedSource = billing.PackageSourceOnlineTopup
		if p.UserID != "" || p.PaymentOrderID == "" {
			return grantValidationError("online tenant recharge requires a payment order and no user")
		}
	case billing.OrderTypeUserTopupIncome:
		online = true
		expectedSource = billing.PackageSourceUserTopupIncome
		if p.UserID != "" || p.PaymentOrderID == "" {
			return grantValidationError("user topup income requires a payment order and no user")
		}
	default:
		return grantValidationError("unsupported recharge order type")
	}
	if p.Source != expectedSource {
		return grantValidationError(fmt.Sprintf("recharge source %q does not match order type %q", p.Source, p.OrderType))
	}
	if online && p.PaymentRef == "" {
		return grantValidationError("online recharge requires a payment reference")
	}
	return nil
}

func grantValidationError(detail string) error {
	return shared.NewErrorWithDetail(shared.ErrBadRequest.Code, shared.ErrBadRequest.Message, detail)
}

// GrantRef addresses the account a grant or charge belongs to: the end user
// when one is named, the tenant otherwise.
func GrantRef(tenantID, userID string) ledger.Ref {
	if userID != "" {
		return ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}
	}
	return ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}
}

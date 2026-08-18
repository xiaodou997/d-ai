package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/ledger"
)

// GrantParams records a USD balance grant and creates one expiring or permanent
// balance lot for it.
//
// 由外部事务传入 tx，内部不 Begin/Commit —— 调用方（admin recharge handler /
// 微信支付 Settle）各自决定事务边界。
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

// GrantRef addresses the account a grant or charge belongs to: the end user
// when one is named, the tenant otherwise.
func GrantRef(tenantID, userID string) ledger.Ref {
	if userID != "" {
		return ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}
	}
	return ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}
}

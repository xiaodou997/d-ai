package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/pg"
)

// GrantParams 记账入参：写充值订单 + 清欠 + 视剩余量发积分包。
// 由外部事务传入 tx，内部不 Begin/Commit —— 调用方（admin recharge handler /
// 微信支付 Settle / 现金购积分）各自决定事务边界。
type GrantParams struct {
	OrderType         string // billing.OrderType*
	TenantID          string
	UserID            string // 为空表示租户充值
	CreditAmountMicro int64
	PaidAmount        int64
	PaymentRef        string
	Note              string
	OperatorID        string
	Source            string // billing.PackageSource*
	ExpiresAt         *time.Time
}

// GrantResult 记账结果。
type GrantResult struct {
	OrderID          string
	PackageID        string // 为空表示全部抵扣透支，未产生新积分包
	ClearedDebtMicro int64
	PackageMicro     int64
	OrderTime        time.Time
}

// GrantCredits 在外部事务内写充值订单，先抵扣对应主体的透支额度，剩余部分发新积分包。
// 抽自原 admin_finance.go 的 recharge handler 裸 SQL，行为不变；现由 admin 手动充值、
// 微信支付回调核销、现金购积分三处复用。
func GrantCredits(ctx context.Context, tx pgx.Tx, p GrantParams) (*GrantResult, error) {
	now := billing.NowUTC()
	orderID := "ORD_" + uuid.New().String()[:24]
	packageID := "PKG_" + uuid.New().String()[:24]

	var userIDVal, paymentRefVal, noteVal any
	if p.UserID != "" {
		userIDVal = p.UserID
	}
	if p.PaymentRef != "" {
		paymentRefVal = p.PaymentRef
	}
	if p.Note != "" {
		noteVal = p.Note
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_recharge_orders
		(order_id, order_type, tenant_id, user_id, credit_amount, paid_amount,
		 payment_ref, expires_at, operator_id, note, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'active', $11)
	`, orderID, p.OrderType, p.TenantID, userIDVal, p.CreditAmountMicro, p.PaidAmount,
		paymentRefVal, p.ExpiresAt, p.OperatorID, noteVal, now); err != nil {
		return nil, err
	}

	// 自动清欠：优先抵扣对应主体的 current_overdraft，剩余进新积分包
	remaining := p.CreditAmountMicro
	var clearedOverdraft int64
	var pkgType string
	if p.UserID == "" {
		pkgType = billing.PackageTypeTenant
		cleared, err := pg.DecreaseTenantOverdraft(ctx, tx, p.TenantID, remaining)
		if err != nil {
			return nil, err
		}
		clearedOverdraft, remaining = cleared, remaining-cleared
	} else {
		pkgType = billing.PackageTypeUser
		cleared, err := pg.DecreaseUserOverdraft(ctx, tx, p.UserID, remaining)
		if err != nil {
			return nil, err
		}
		clearedOverdraft, remaining = cleared, remaining-cleared
	}

	createdPackageID := ""
	if remaining > 0 {
		createdPackageID = packageID
		if _, err := tx.Exec(ctx, `
			INSERT INTO bill_credit_packages
			(package_id, package_type, tenant_id, user_id,
			 total_credits, remaining_credits, expires_at, status, source,
			 recharge_order_id, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'available', $8, $9, 0, $10, $10)
		`, packageID, pkgType, p.TenantID, userIDVal, remaining, remaining, p.ExpiresAt, p.Source, orderID, now); err != nil {
			return nil, err
		}
	}

	return &GrantResult{
		OrderID:          orderID,
		PackageID:        createdPackageID,
		ClearedDebtMicro: clearedOverdraft,
		PackageMicro:     remaining,
		OrderTime:        now,
	}, nil
}

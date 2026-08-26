package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/audit"
	"xiaodou/dai/internal/billing/ledger"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/payment"
	paymentpg "xiaodou/dai/internal/payment/pg"
)

type RecordCompletedRefundParams struct {
	PaymentOrderID  string
	Method          string
	RefundReference string
	ChannelRefundID string
	RefundedAt      time.Time
	Reason          string
	Note            string
	OperatorID      string
}

type rechargeGrantForRefund struct {
	OrderID     string
	OrderType   string
	TenantID    string
	UserID      string
	CreditMicro int64
	Status      string
}

// RecordCompletedRefund records an already completed full refund and reverses
// every balance grant produced by the payment in one database transaction.
func (s *PaymentService) RecordCompletedRefund(ctx context.Context, p RecordCompletedRefundParams) (*payment.Refund, error) {
	p.PaymentOrderID = strings.TrimSpace(p.PaymentOrderID)
	p.Method = strings.TrimSpace(p.Method)
	p.RefundReference = strings.TrimSpace(p.RefundReference)
	p.ChannelRefundID = strings.TrimSpace(p.ChannelRefundID)
	p.Reason = strings.TrimSpace(p.Reason)
	p.Note = strings.TrimSpace(p.Note)
	if p.PaymentOrderID == "" || p.RefundReference == "" || p.Reason == "" || p.OperatorID == "" || p.RefundedAt.IsZero() {
		return nil, domain.ErrBadRequest
	}
	if p.Method != payment.RefundMethodWechat && p.Method != payment.RefundMethodOffline {
		return nil, domain.ErrBadRequest
	}
	if p.Method == payment.RefundMethodWechat && p.ChannelRefundID == "" {
		return nil, domain.ErrBadRequest
	}
	if p.RefundedAt.After(time.Now().UTC().Add(5 * time.Minute)) {
		return nil, domain.ErrBadRequest
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	order, err := paymentpg.GetOrderByIDForUpdate(ctx, tx, p.PaymentOrderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentOrderNotFound
		}
		return nil, err
	}
	existing, err := paymentpg.GetRefundByPaymentOrderTx(ctx, tx, order.OrderID)
	if err == nil {
		if existing.Method == p.Method && existing.RefundReference == p.RefundReference && existing.ChannelRefundID == p.ChannelRefundID {
			return existing, tx.Commit(ctx)
		}
		return nil, domain.ErrPaymentAlreadyRefunded
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if order.Status != payment.OrderStatusPaid || order.FulfillmentStatus != payment.FulfillmentStatusCredited || order.RefundStatus != payment.RefundStatusNone {
		return nil, domain.ErrPaymentRefundNotAllowed
	}
	if order.PaidAt == nil || p.RefundedAt.Before(*order.PaidAt) {
		return nil, domain.ErrPaymentRefundNotAllowed
	}

	grants, err := loadRechargeGrantsForRefund(ctx, tx, order.OrderID, order.BalanceOrderID)
	if err != nil {
		return nil, err
	}
	if err := validateRefundGrants(order, grants); err != nil {
		return nil, err
	}
	grantSnapshot := make([]map[string]any, 0, len(grants))
	for _, grant := range grants {
		grantSnapshot = append(grantSnapshot, map[string]any{
			"order_id": grant.OrderID, "order_type": grant.OrderType,
			"tenant_id": grant.TenantID, "user_id": grant.UserID,
			"credit_micro": grant.CreditMicro, "status": grant.Status,
		})
	}
	beforeState, err := audit.Snapshot(map[string]any{
		"payment_order_id": order.OrderID, "status": order.Status,
		"fulfillment_status": order.FulfillmentStatus, "refund_status": order.RefundStatus,
		"balance_order_id":          order.BalanceOrderID,
		"credited_amount_micro_usd": order.CreditedAmountMicroUSD,
		"tenant_income_micro_usd":   order.TenantIncomeMicroUSD,
		"grants":                    grantSnapshot,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	refund := &payment.Refund{
		RefundID:          "RFD_" + uuid.New().String()[:24],
		PaymentOrderID:    order.OrderID,
		Method:            p.Method,
		RefundReference:   p.RefundReference,
		ChannelRefundID:   p.ChannelRefundID,
		RefundAmountMinor: order.PaymentAmountMinor,
		Status:            "completed",
		RefundedAt:        p.RefundedAt.UTC(),
		Reason:            p.Reason,
		Note:              p.Note,
		OperatorID:        p.OperatorID,
		CreatedAt:         now,
	}
	if err := paymentpg.InsertRefundTx(ctx, tx, refund); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrDuplicateRequest
		}
		return nil, err
	}

	effectSnapshot := make([]map[string]any, 0, len(grants))
	for _, grant := range grants {
		accountID := grant.UserID
		if accountID == "" {
			accountID = grant.TenantID
		}
		effect, err := ledger.ReverseGrantForRefund(ctx, tx, accountID, grant.OrderID, grant.CreditMicro)
		if err != nil {
			if errors.Is(err, ledger.ErrRefundReversalUnreconciled) {
				return nil, domain.ErrPaymentRefundNotAllowed
			}
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE bill_recharge_orders
			SET status = 'reversed', reversed_at = $1, reversed_by = $2,
			    reversal_reason = $3, reversed_amount_micro = $4, lost_amount_micro = $5
			WHERE order_id = $6
		`, now, p.OperatorID, "退款冲正: "+p.Reason, effect.AvailableReclaimedMicro, effect.NonAvailableDebitMicro, grant.OrderID); err != nil {
			return nil, fmt.Errorf("更新退款关联充值单失败: %w", err)
		}
		reversalEffect := payment.RefundReversalEffect{
			ReversalID:                 "RVR_" + uuid.New().String()[:24],
			RefundID:                   refund.RefundID,
			RechargeOrderID:            grant.OrderID,
			AccountID:                  accountID,
			CreditAmountMicroUSD:       grant.CreditMicro,
			AvailableReclaimedMicroUSD: effect.AvailableReclaimedMicro,
			NonAvailableDebitMicroUSD:  effect.NonAvailableDebitMicro,
			ExpiredAmountMicroUSD:      effect.ExpiredMicro,
			AccountDebitMicroUSD:       effect.AccountDebitMicro,
			BalanceAfterMicroUSD:       effect.BalanceAfterMicro,
		}
		if err := paymentpg.InsertRefundReversalEffectTx(ctx, tx, reversalEffect); err != nil {
			return nil, err
		}
		effectSnapshot = append(effectSnapshot, map[string]any{
			"recharge_order_id": grant.OrderID, "account_id": accountID,
			"credit_micro": effect.CreditMicro, "available_reclaimed_micro": effect.AvailableReclaimedMicro,
			"non_available_debit_micro": effect.NonAvailableDebitMicro,
			"expired_micro":             effect.ExpiredMicro, "account_debit_micro": effect.AccountDebitMicro,
			"balance_after_micro": effect.BalanceAfterMicro,
		})
		if grant.OrderType == billing.OrderTypeUserTopupIncome {
			entry := &payment.CashLedgerEntry{
				TxnID: "CSH_" + uuid.New().String()[:24], TenantID: grant.TenantID,
				TxnType: payment.CashTxnRefundReversal, AmountMicroUSD: -grant.CreditMicro,
				BalanceAfterMicroUSD: effect.BalanceAfterMicro, RefType: "refund", RefID: refund.RefundID,
				OperatorID: p.OperatorID, Note: "用户在线充值退款收入冲正: " + p.Reason,
			}
			if err := paymentpg.InsertCashLedgerTx(ctx, tx, entry, "refund:"+refund.RefundID); err != nil {
				return nil, err
			}
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pay_orders
		SET refund_status = 'refunded', fulfillment_status = 'reversed', updated_at = $1
		WHERE order_id = $2
	`, now, order.OrderID); err != nil {
		return nil, fmt.Errorf("更新退款订单状态失败: %w", err)
	}
	afterState, err := audit.Snapshot(map[string]any{
		"payment_order_id": order.OrderID, "status": order.Status,
		"fulfillment_status": payment.FulfillmentStatusReversed, "refund_status": payment.RefundStatusRefunded,
		"refund_id": refund.RefundID, "refund_reference": refund.RefundReference,
		"refund_amount_minor": refund.RefundAmountMinor, "effects": effectSnapshot,
	})
	if err != nil {
		return nil, err
	}
	if err := audit.Append(ctx, tx, audit.Event{
		RepairID: audit.NewRepairID(), Action: "payment_refund",
		IdempotencyKey: "payment-refund:" + order.OrderID,
		TargetType:     "pay_orders", TargetID: order.OrderID,
		OperatorID: p.OperatorID, Reason: p.Reason,
		BeforeState: beforeState, AfterState: afterState,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return refund, nil
}

func loadRechargeGrantsForRefund(ctx context.Context, tx pgx.Tx, paymentOrderID, primaryOrderID string) ([]rechargeGrantForRefund, error) {
	rows, err := tx.Query(ctx, `
		SELECT order_id, order_type, tenant_id, COALESCE(user_id, ''), credit_amount, status
		FROM bill_recharge_orders
		WHERE payment_order_id = $1
		ORDER BY (order_id = $2) DESC, order_id
		FOR UPDATE
	`, paymentOrderID, primaryOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var grants []rechargeGrantForRefund
	for rows.Next() {
		var grant rechargeGrantForRefund
		if err := rows.Scan(&grant.OrderID, &grant.OrderType, &grant.TenantID, &grant.UserID, &grant.CreditMicro, &grant.Status); err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func validateRefundGrants(order *payment.Order, grants []rechargeGrantForRefund) error {
	expectedGrantCount := 1
	if order.TenantIncomeMicroUSD > 0 {
		expectedGrantCount++
	}
	if len(grants) != expectedGrantCount {
		return domain.ErrPaymentRefundNotAllowed
	}
	primaryFound := false
	incomeFound := false
	for _, grant := range grants {
		if grant.Status != billing.OrderStatusActive || grant.TenantID != order.TenantID {
			return domain.ErrPaymentRefundNotAllowed
		}
		if grant.OrderID == order.BalanceOrderID {
			primaryFound = true
			expected := billing.OrderTypeOnlineTenantTopup
			if order.Scene == payment.SceneUserTopup {
				expected = billing.OrderTypeOnlineUserTopup
			}
			if grant.OrderType != expected || grant.CreditMicro != order.CreditedAmountMicroUSD {
				return domain.ErrPaymentRefundNotAllowed
			}
		} else if grant.OrderType == billing.OrderTypeUserTopupIncome && !incomeFound {
			incomeFound = true
			if grant.CreditMicro != order.TenantIncomeMicroUSD {
				return domain.ErrPaymentRefundNotAllowed
			}
		} else {
			return domain.ErrPaymentRefundNotAllowed
		}
	}
	if !primaryFound || (order.TenantIncomeMicroUSD > 0 && !incomeFound) {
		return domain.ErrPaymentRefundNotAllowed
	}
	return nil
}

package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/payment"
)

const refundColumns = `
	refund_id, payment_order_id, refund_method, refund_reference,
	COALESCE(channel_refund_id, ''), refund_amount_minor, status,
	refunded_at, reason, COALESCE(note, ''), operator_id, created_at`

func scanRefund(row pgx.Row) (*payment.Refund, error) {
	var refund payment.Refund
	if err := row.Scan(
		&refund.RefundID, &refund.PaymentOrderID, &refund.Method, &refund.RefundReference,
		&refund.ChannelRefundID, &refund.RefundAmountMinor, &refund.Status,
		&refund.RefundedAt, &refund.Reason, &refund.Note, &refund.OperatorID, &refund.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &refund, nil
}

func GetRefundByPaymentOrderTx(ctx context.Context, tx pgx.Tx, paymentOrderID string) (*payment.Refund, error) {
	return scanRefund(tx.QueryRow(ctx, `SELECT `+refundColumns+` FROM pay_refunds WHERE payment_order_id = $1`, paymentOrderID))
}

func InsertRefundTx(ctx context.Context, tx pgx.Tx, refund *payment.Refund) error {
	var channelRefundID, note any
	if refund.ChannelRefundID != "" {
		channelRefundID = refund.ChannelRefundID
	}
	if refund.Note != "" {
		note = refund.Note
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO pay_refunds
		(refund_id, payment_order_id, refund_method, refund_reference, channel_refund_id,
		 refund_amount_minor, status, refunded_at, reason, note, operator_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'completed', $7, $8, $9, $10, $11)
	`, refund.RefundID, refund.PaymentOrderID, refund.Method, refund.RefundReference, channelRefundID,
		refund.RefundAmountMinor, refund.RefundedAt, refund.Reason, note, refund.OperatorID, refund.CreatedAt)
	if err != nil {
		return fmt.Errorf("记录退款失败: %w", err)
	}
	return nil
}

func InsertRefundReversalEffectTx(ctx context.Context, tx pgx.Tx, effect payment.RefundReversalEffect) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO bill_refund_reversal_effects
		(reversal_id, refund_id, recharge_order_id, account_id, credit_amount_micro,
		 available_reclaimed_micro, non_available_debit_micro, expired_amount_micro,
		 account_debit_micro, balance_after_micro, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, effect.ReversalID, effect.RefundID, effect.RechargeOrderID, effect.AccountID,
		effect.CreditAmountMicroUSD, effect.AvailableReclaimedMicroUSD,
		effect.NonAvailableDebitMicroUSD, effect.ExpiredAmountMicroUSD,
		effect.AccountDebitMicroUSD, effect.BalanceAfterMicroUSD, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("记录退款额度冲正结果失败: %w", err)
	}
	return nil
}

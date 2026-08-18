package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrRefundReversalUnreconciled = errors.New("refund reversal cannot reconcile legacy expired credit")

// RefundReversal is the balance effect of financially reversing one grant.
// Consumed credit and credit previously used to clear debt are debited again;
// expired unused credit has already left the balance and is not double-debited.
type RefundReversal struct {
	CreditMicro             int64
	AvailableReclaimedMicro int64
	NonAvailableDebitMicro  int64
	ExpiredMicro            int64
	AccountDebitMicro       int64
	BalanceAfterMicro       int64
}

func ReverseGrantForRefund(ctx context.Context, tx pgx.Tx, accountID, rechargeOrderID string, creditMicro int64) (RefundReversal, error) {
	result := RefundReversal{CreditMicro: creditMicro}
	if accountID == "" || rechargeOrderID == "" || creditMicro <= 0 {
		return result, fmt.Errorf("refund reversal requires account, recharge order and positive credit")
	}

	rows, err := tx.Query(ctx, `
		SELECT granted_micro, LEAST(consumed_micro, granted_micro), expired_at,
		       expired_unused_micro, revoked_at
		FROM bill_credit_lots
		WHERE recharge_order_id = $1
		ORDER BY id
		FOR UPDATE
	`, rechargeOrderID)
	if err != nil {
		return result, fmt.Errorf("load refund reversal lots for %s: %w", rechargeOrderID, err)
	}
	var grantedByLots int64
	for rows.Next() {
		var granted, consumed int64
		var expiredAt, revokedAt *time.Time
		var expiredUnused *int64
		if err := rows.Scan(&granted, &consumed, &expiredAt, &expiredUnused, &revokedAt); err != nil {
			rows.Close()
			return result, fmt.Errorf("scan refund reversal lot for %s: %w", rechargeOrderID, err)
		}
		if revokedAt != nil {
			rows.Close()
			return result, fmt.Errorf("recharge order %s contains an already revoked lot", rechargeOrderID)
		}
		grantedByLots += granted
		remaining := granted - consumed
		if expiredAt != nil {
			if expiredUnused == nil || *expiredUnused != remaining {
				rows.Close()
				return result, fmt.Errorf("%w: recharge order %s", ErrRefundReversalUnreconciled, rechargeOrderID)
			}
			result.NonAvailableDebitMicro += consumed
			result.ExpiredMicro += *expiredUnused
		} else {
			if expiredUnused != nil {
				rows.Close()
				return result, fmt.Errorf("refund reversal lot has expiry snapshot without expiry for %s", rechargeOrderID)
			}
			result.NonAvailableDebitMicro += consumed
			result.AvailableReclaimedMicro += remaining
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("load refund reversal lots for %s: %w", rechargeOrderID, err)
	}
	if grantedByLots > creditMicro {
		return result, fmt.Errorf("refund reversal lots exceed recharge credit for %s", rechargeOrderID)
	}

	// Credit without a lot was consumed immediately by an existing debt.
	result.NonAvailableDebitMicro += creditMicro - grantedByLots
	result.AccountDebitMicro = result.AvailableReclaimedMicro + result.NonAvailableDebitMicro
	if result.AccountDebitMicro+result.ExpiredMicro != creditMicro {
		return result, fmt.Errorf("refund reversal does not reconcile for %s", rechargeOrderID)
	}

	if err := tx.QueryRow(ctx, `
		UPDATE bill_accounts
		SET balance_micro = balance_micro - $1, updated_at = now()
		WHERE account_id = $2
		RETURNING balance_micro
	`, result.AccountDebitMicro, accountID).Scan(&result.BalanceAfterMicro); err != nil {
		return result, fmt.Errorf("debit refund reversal from account %s: %w", accountID, err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bill_credit_lots
		SET revoked_at = now(), consumed_micro = granted_micro, updated_at = now()
		WHERE recharge_order_id = $1 AND expired_at IS NULL AND revoked_at IS NULL
	`, rechargeOrderID); err != nil {
		return result, fmt.Errorf("revoke active refund lots for %s: %w", rechargeOrderID, err)
	}
	return result, nil
}

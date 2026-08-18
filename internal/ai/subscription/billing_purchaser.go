package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/billing/ledger"
)

// BillingPurchaser charges for a subscription purchase.
//
// Unlike AI settlement this is a strict, pre-paid debit: nothing has been
// delivered yet, so an account that cannot afford the plan is simply refused
// and no balance moves. That is why it uses ledger.ChargeIfFunded rather than
// ledger.Charge — the two are different transactions in the business sense, not
// just different flags on one.
type BillingPurchaser struct {
	pool     *pgxpool.Pool
	clientID string
}

func NewBillingPurchaser(pool *pgxpool.Pool, clientID string) *BillingPurchaser {
	return &BillingPurchaser{pool: pool, clientID: clientID}
}

var _ Purchaser = (*BillingPurchaser)(nil)

func (p *BillingPurchaser) DebitStrict(ctx context.Context, req DebitRequest) (*DebitReceipt, error) {
	if req.IdempotencyKey == "" {
		return nil, errors.New("subscription debit requires an idempotency key")
	}
	if req.TenantID == "" {
		return nil, errors.New("subscription debit requires a tenant id")
	}
	if req.TenantMicro < 0 || req.UserMicro < 0 {
		return nil, fmt.Errorf("subscription debit amounts must not be negative: tenant=%d user=%d",
			req.TenantMicro, req.UserMicro)
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	const keyPrefix = "ai-sub-"
	orderNo := strings.TrimPrefix(req.IdempotencyKey, keyPrefix)
	if orderNo == "" || orderNo == req.IdempotencyKey {
		return nil, errors.New("subscription debit idempotency key has invalid format")
	}

	// The purchase order is the idempotency anchor. It is locked before the
	// ledger debit, so a retry returns the same debit reference without charging
	// a second time.
	var orderTenantID, orderUserID, debitReference, orderStatus string
	err = tx.QueryRow(ctx, `
		SELECT tenant_id, user_id, COALESCE(debit_reference, ''), status
		FROM ai_sub_orders WHERE order_no = $1 FOR UPDATE
	`, orderNo).Scan(&orderTenantID, &orderUserID, &debitReference, &orderStatus)
	if err == nil {
		if orderTenantID != req.TenantID || orderUserID != req.UserID {
			return nil, errors.New("subscription debit order ownership mismatch")
		}
		if debitReference != "" {
			return &DebitReceipt{AuthorizationID: debitReference}, nil
		}
		if orderStatus != OrderDeducting && orderStatus != OrderCreated {
			return nil, fmt.Errorf("subscription order is not debitable (status=%s)", orderStatus)
		}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		if err != nil {
			return nil, fmt.Errorf("look up subscription debit: %w", err)
		}
	} else {
		return nil, fmt.Errorf("subscription order not found: %s", orderNo)
	}

	if req.TenantMicro > 0 {
		if err := ledger.ChargeIfFunded(ctx, tx, ledger.Ref{
			Kind: ledger.KindTenant, ID: req.TenantID, TenantID: req.TenantID,
		}, req.TenantMicro); err != nil {
			return nil, mapLedgerError(err)
		}
	}
	if req.UserMicro > 0 {
		if req.UserID == "" {
			return nil, errors.New("subscription user debit requires a user id")
		}
		if err := ledger.ChargeIfFunded(ctx, tx, ledger.Ref{
			Kind: ledger.KindUser, ID: req.UserID, TenantID: req.TenantID,
		}, req.UserMicro); err != nil {
			return nil, mapLedgerError(err)
		}
	}

	debitReference = "subscription:" + orderNo
	if _, err := tx.Exec(ctx, `
		UPDATE ai_sub_orders
		SET debit_reference = $1, debited_at = now(), updated_at = now()
		WHERE order_no = $2 AND debit_reference IS NULL
	`, debitReference, orderNo); err != nil {
		return nil, fmt.Errorf("record subscription debit reference: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &DebitReceipt{AuthorizationID: debitReference}, nil
}

func mapLedgerError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ledger.ErrInsufficientBalance) {
		return fmt.Errorf("%w: %w", ErrInsufficientBalance, err)
	}
	return err
}

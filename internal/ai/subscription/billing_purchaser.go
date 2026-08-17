package subscription

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
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

	// The idempotency key is unique on bill_events, so a retried purchase
	// returns the original receipt instead of charging a second time.
	var existingEventID string
	err = tx.QueryRow(ctx, `
		SELECT event_id FROM bill_events WHERE idempotency_key = $1
	`, req.IdempotencyKey).Scan(&existingEventID)
	if err == nil {
		return &DebitReceipt{AuthorizationID: existingEventID}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("look up subscription debit: %w", err)
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

	eventID := "EV_" + uuid.New().String()[:24]
	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_events
		(event_id, idempotency_key, tenant_id, user_id, description, client_id,
		 event_type, tenant_credits, user_credits, status, created_at, finished_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6,
		        'charge', NULLIF($7, 0), NULLIF($8, 0), 'succeeded', now(), now())
	`, eventID, req.IdempotencyKey, req.TenantID, req.UserID, req.Description, p.clientID,
		req.TenantMicro, req.UserMicro); err != nil {
		return nil, fmt.Errorf("record subscription debit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &DebitReceipt{AuthorizationID: eventID}, nil
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

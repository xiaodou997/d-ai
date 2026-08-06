package billingledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type ReconciliationItem struct {
	RequestID        string    `json:"request_id"`
	WindowID         string    `json:"window_id"`
	LeaseID          string    `json:"lease_id"`
	TenantID         string    `json:"tenant_id"`
	UserID           string    `json:"user_id,omitempty"`
	RequestExpiresAt time.Time `json:"request_expires_at"`
	ErrorCode        string    `json:"error_code,omitempty"`
	ErrorDetail      string    `json:"error_detail,omitempty"`
}

// ListReconciliations returns requests whose completion could not be proven.
// The lease may already have left escrow; settlement remains legal in billing.
func (c *Coordinator) ListReconciliations(ctx context.Context) ([]ReconciliationItem, error) {
	if c == nil || c.pool == nil {
		return nil, ErrDependencyUnavailable
	}
	rows, err := c.pool.Query(ctx, `
		SELECT a.request_id, a.window_id, a.lease_id, w.tenant_id, w.user_id,
		       a.request_expires_at, COALESCE(w.last_error_code,''),
		       COALESCE(w.last_error_detail,'')
		FROM ai_billing_request_admissions a
		JOIN ai_billing_windows w ON w.window_id=a.window_id
		WHERE a.status='reconciling'
		ORDER BY a.request_expires_at, a.request_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReconciliationItem
	for rows.Next() {
		var item ReconciliationItem
		if err := rows.Scan(&item.RequestID, &item.WindowID, &item.LeaseID,
			&item.TenantID, &item.UserID, &item.RequestExpiresAt,
			&item.ErrorCode, &item.ErrorDetail); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ResolveReconciliation records an operator-reviewed amount using the same
// atomic completion path as runtime usage. A note is mandatory so a manual
// financial decision never becomes an unexplained number.
func (c *Coordinator) ResolveReconciliation(ctx context.Context, completion Completion) error {
	if c == nil || c.pool == nil {
		return ErrDependencyUnavailable
	}
	if completion.RequestID == "" || completion.Note == "" {
		return fmt.Errorf("%w: request_id and reconciliation note are required", ErrProtocolViolation)
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM ai_billing_request_admissions
		WHERE request_id=$1 FOR UPDATE
	`, completion.RequestID).Scan(&status); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("%w: reconciliation request not found", ErrProtocolViolation)
		}
		return err
	}
	if status != "reconciling" {
		return fmt.Errorf("%w: request is %s, not reconciling", ErrAdmissionConflict, status)
	}
	completion.Source = "manual"
	if err := c.Complete(ctx, tx, completion); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	c.Trigger()
	return nil
}

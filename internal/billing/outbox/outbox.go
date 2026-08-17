// Package outbox moves AI usage charges from the request that incurred them to
// the ledger that records them.
//
// The runtime writes an outbox row in the same transaction as the usage log, so
// the two can never disagree: if a usage record exists, the charge it implies
// exists too and will be applied. Settlement then happens outside the request,
// which keeps the hot path free of the account row lock that used to serialise
// every request belonging to one tenant.
//
// The queue lives in PostgreSQL rather than Redis on purpose. An external queue
// can be unavailable exactly when a charge needs to be durable, and the only
// thing left to do then is drop it. Here the charge is committed with the usage
// row or not at all.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/dai/internal/billing/ledger"
)

// Charge is one settled AI request awaiting ledger application.
type Charge struct {
	RequestID   string
	TenantID    string
	UserID      string
	TenantMicro int64
	UserMicro   int64
	Description string
}

// Enqueue records a charge for later application. It must be called inside the
// transaction that writes the usage record — that shared commit is the entire
// durability guarantee.
//
// RequestID is the idempotency anchor and is unique, so a replayed completion
// enqueues nothing rather than charging twice.
func Enqueue(ctx context.Context, tx ledger.Execer, c Charge) error {
	if c.RequestID == "" {
		return errors.New("outbox charge requires a request id")
	}
	if c.TenantID == "" {
		return errors.New("outbox charge requires a tenant id")
	}
	if c.TenantMicro < 0 || c.UserMicro < 0 {
		return fmt.Errorf("outbox charge amounts must not be negative: tenant=%d user=%d", c.TenantMicro, c.UserMicro)
	}
	if c.TenantMicro == 0 && c.UserMicro == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_charge_outbox
		(request_id, tenant_id, user_id, tenant_micro, user_micro, description)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6)
		ON CONFLICT (request_id) DO NOTHING
	`, c.RequestID, c.TenantID, c.UserID, c.TenantMicro, c.UserMicro, c.Description); err != nil {
		return fmt.Errorf("enqueue charge for %s: %w", c.RequestID, err)
	}
	return nil
}

// ============================================================================
// Consumer
// ============================================================================

const (
	defaultInterval    = 500 * time.Millisecond
	defaultBatchSize   = 100
	defaultMaxAttempts = 10
)

// Consumer drains the outbox into the ledger.
//
// Rows are claimed with FOR UPDATE SKIP LOCKED, which makes running one
// consumer per instance safe and — more importantly — means a row that cannot
// be applied never blocks the rows behind it. Each row is applied inside its
// own savepoint, so one bad charge is isolated from its whole batch.
type Consumer struct {
	pool        *pgxpool.Pool
	logger      *zap.Logger
	interval    time.Duration
	batchSize   int
	maxAttempts int
}

func NewConsumer(pool *pgxpool.Pool, logger *zap.Logger) *Consumer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Consumer{
		pool:        pool,
		logger:      logger,
		interval:    defaultInterval,
		batchSize:   defaultBatchSize,
		maxAttempts: defaultMaxAttempts,
	}
}

// Run drains the outbox until ctx is cancelled. It keeps draining without
// pausing while batches come back full, so a backlog clears at full speed.
func (c *Consumer) Run(ctx context.Context) {
	if c == nil || c.pool == nil {
		return
	}
	timer := time.NewTimer(c.interval)
	defer timer.Stop()
	for ctx.Err() == nil {
		applied, err := c.DrainOnce(ctx)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("billing outbox drain failed", zap.Error(err))
		}
		if applied == c.batchSize && err == nil {
			continue
		}
		timer.Reset(c.interval)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

// DrainOnce applies at most one batch and reports how many rows it claimed.
// Exported so tests and operators can settle deterministically.
func (c *Consumer) DrainOnce(ctx context.Context) (int, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin outbox drain: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, request_id, tenant_id, COALESCE(user_id, ''),
		       tenant_micro, user_micro, description, attempts
		FROM bill_charge_outbox
		WHERE status = 'pending'
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, c.batchSize)
	if err != nil {
		return 0, fmt.Errorf("claim outbox batch: %w", err)
	}
	type claimed struct {
		id       int64
		attempts int
		charge   Charge
	}
	var batch []claimed
	for rows.Next() {
		var row claimed
		if err := rows.Scan(&row.id, &row.charge.RequestID, &row.charge.TenantID, &row.charge.UserID,
			&row.charge.TenantMicro, &row.charge.UserMicro, &row.charge.Description, &row.attempts); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox row: %w", err)
		}
		batch = append(batch, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("claim outbox batch: %w", err)
	}
	if len(batch) == 0 {
		return 0, tx.Commit(ctx)
	}

	for _, row := range batch {
		// A savepoint per row: a charge that cannot be applied rolls back only
		// itself, and the rest of the batch still settles.
		sp, err := tx.Begin(ctx)
		if err != nil {
			return 0, fmt.Errorf("open outbox savepoint: %w", err)
		}
		if applyErr := c.apply(ctx, sp, row.charge); applyErr != nil {
			_ = sp.Rollback(ctx)
			if err := c.recordFailure(ctx, tx, row.id, row.attempts+1, applyErr); err != nil {
				return 0, err
			}
			c.logger.Error("billing outbox charge failed",
				zap.String("request_id", row.charge.RequestID),
				zap.Int("attempts", row.attempts+1),
				zap.Error(applyErr))
			continue
		}
		if err := sp.Commit(ctx); err != nil {
			return 0, fmt.Errorf("release outbox savepoint: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit outbox drain: %w", err)
	}
	return len(batch), nil
}

// apply moves the money and records the statement line. It deliberately does
// not check account status: the upstream cost was already incurred, so a
// suspended or deleted account still owes it. Refusing here would lose the
// charge and, with it, the usage record's financial half.
func (c *Consumer) apply(ctx context.Context, tx pgx.Tx, charge Charge) error {
	if charge.TenantMicro > 0 {
		if err := ledger.Charge(ctx, tx, ledger.Ref{
			Kind: ledger.KindTenant, ID: charge.TenantID, TenantID: charge.TenantID,
		}, charge.TenantMicro); err != nil {
			return err
		}
	}
	if charge.UserMicro > 0 {
		if charge.UserID == "" {
			return errors.New("user charge without a user id")
		}
		if err := ledger.Charge(ctx, tx, ledger.Ref{
			Kind: ledger.KindUser, ID: charge.UserID, TenantID: charge.TenantID,
		}, charge.UserMicro); err != nil {
			return err
		}
	}

	eventID := "EV_" + uuid.New().String()[:24]
	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_events
		(event_id, idempotency_key, tenant_id, user_id, description, client_id,
		 event_type, tenant_credits, user_credits, status, created_at, finished_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, 'dai-ai',
		        'charge', NULLIF($6, 0), NULLIF($7, 0), 'succeeded', now(), now())
		ON CONFLICT (idempotency_key) DO NOTHING
	`, eventID, "ai-usage:"+charge.RequestID, charge.TenantID, charge.UserID,
		charge.Description, charge.TenantMicro, charge.UserMicro); err != nil {
		return fmt.Errorf("record charge event: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE bill_charge_outbox
		SET status = 'done', attempts = attempts + 1, settled_at = now(), last_error = NULL
		WHERE request_id = $1
	`, charge.RequestID); err != nil {
		return fmt.Errorf("mark charge settled: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE ai_usage_logs
		SET billing_event_id = $1, billing_status = 'confirmed'
		WHERE request_id = $2
	`, eventID, charge.RequestID); err != nil {
		return fmt.Errorf("link charge event to usage: %w", err)
	}
	return nil
}

// recordFailure runs in the outer transaction so it survives the savepoint
// rollback that discarded the failed attempt. A charge that keeps failing is
// parked in 'failed' rather than retried forever, which takes it out of the
// pending index and makes it visible to operators.
func (c *Consumer) recordFailure(ctx context.Context, tx pgx.Tx, id int64, attempts int, cause error) error {
	status := "pending"
	if attempts >= c.maxAttempts {
		status = "failed"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bill_charge_outbox
		SET status = $1, attempts = $2, last_error = $3
		WHERE id = $4
	`, status, attempts, cause.Error(), id); err != nil {
		return fmt.Errorf("record outbox failure: %w", err)
	}
	return nil
}

// PendingStats reports queue health for monitoring and for tests that need to
// assert the queue actually drained.
type PendingStats struct {
	Pending int64
	Failed  int64
	OldestS float64
}

func Stats(ctx context.Context, q ledger.Querier) (PendingStats, error) {
	var s PendingStats
	if err := q.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE status = 'pending'),
		  COUNT(*) FILTER (WHERE status = 'failed'),
		  COALESCE(EXTRACT(EPOCH FROM now() - MIN(created_at) FILTER (WHERE status = 'pending')), 0)
		FROM bill_charge_outbox
	`).Scan(&s.Pending, &s.Failed, &s.OldestS); err != nil {
		return PendingStats{}, fmt.Errorf("read outbox stats: %w", err)
	}
	return s, nil
}

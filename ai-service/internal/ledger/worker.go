package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// WorkerConfig governs how aggressively the settle worker drains the ledger.
//
// Defaults (when zero):
//   - TickInterval   : 60s             (定时兜底)
//   - MinTotalMicro  : 1_000_000 (=100 credits) (阈值触发的总余额下限)
//   - MaxIdleAge     : 5 * TickInterval (5 分钟)
//   - PickLimit      : 32              (单轮一次最多锁多少行)
type WorkerConfig struct {
	TickInterval  time.Duration
	MinTotalMicro int64
	MaxIdleAge    time.Duration
	PickLimit     int
}

func (c WorkerConfig) withDefaults() WorkerConfig {
	if c.TickInterval <= 0 {
		c.TickInterval = 60 * time.Second
	}
	if c.MinTotalMicro <= 0 {
		c.MinTotalMicro = 1_000_000 // 100 credits, per design
	}
	if c.MaxIdleAge <= 0 {
		c.MaxIdleAge = 5 * c.TickInterval
	}
	if c.PickLimit <= 0 {
		c.PickLimit = 32
	}
	return c
}

// Worker runs the periodic settlement loop. It pulls eligible rows from
// ai_user_credit_ledger via PickPending → SettleOne, then stamps the matching
// ai_usage_logs rows with the settle event_id in the same transaction.
//
// Two trigger sources:
//   - Ticker (TickInterval, 兜底)
//   - Trigger() channel (LedgerStep 写入后立即推一轮)
//
// Trigger() is non-blocking via a buffered chan (cap 1) — coalesces bursts.
type Worker struct {
	Ledger  *Ledger
	Config  WorkerConfig
	trigger chan struct{}
}

// NewWorker constructs a Worker. The returned channel from Trigger() should be
// passed to LedgerStep.Trigger.
func NewWorker(l *Ledger, cfg WorkerConfig) *Worker {
	return &Worker{
		Ledger:  l,
		Config:  cfg.withDefaults(),
		trigger: make(chan struct{}, 1),
	}
}

// Trigger returns the channel that opportunistic callers (LedgerStep) send to
// in order to wake the worker before the next tick. Send is non-blocking — drop
// if the buffer is full.
func (w *Worker) Trigger() chan<- struct{} { return w.trigger }

// Run drives the settle loop until ctx is cancelled. Blocking; intended to be
// launched as `go worker.Run(ctx)` from server bootstrap.
func (w *Worker) Run(ctx context.Context) {
	if w.Ledger == nil {
		return
	}
	logger := w.Ledger.logger
	logger.Info("ledger settle worker started",
		zap.Duration("tick", w.Config.TickInterval),
		zap.Int64("min_total_micro", w.Config.MinTotalMicro),
		zap.Duration("max_idle_age", w.Config.MaxIdleAge),
	)

	ticker := time.NewTicker(w.Config.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("ledger settle worker stopped")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		case <-w.trigger:
			w.runOnce(ctx)
		}
	}
}

// runOnce executes one drain pass: PickPending → SettleOne loop → commit.
// Errors are logged and swallowed; the next tick/trigger will retry.
func (w *Worker) runOnce(ctx context.Context) {
	logger := w.Ledger.logger
	tx, keys, err := w.Ledger.PickPending(ctx, PickOptions{
		MinTotalMicro: w.Config.MinTotalMicro,
		MaxIdleAge:    w.Config.MaxIdleAge,
		Limit:         w.Config.PickLimit,
	})
	if err != nil {
		logger.Warn("ledger pick pending failed", zap.Error(err))
		return
	}
	if len(keys) == 0 {
		_ = tx.Rollback(ctx)
		return
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	settled := 0
	for _, k := range keys {
		res, err := w.Ledger.SettleOne(ctx, tx, k)
		if err != nil {
			// Abort the whole batch: SettleOne is per-row but a single failure
			// (e.g. URM unavailable) means the rest will likely also fail. Roll
			// back to release all row locks so other workers / next tick retry.
			logger.Warn("ledger settle one failed",
				zap.Error(err),
				zap.String("owner_type", string(k.OwnerType)),
				zap.String("tenant_id", k.TenantID),
				zap.String("user_id", k.UserID),
			)
			return
		}
		if res.NoOp {
			continue
		}
		if err := stampUsageLogs(ctx, tx, k, res.EventID); err != nil {
			logger.Warn("ledger stamp usage logs failed",
				zap.Error(err),
				zap.String("event_id", res.EventID),
			)
			return
		}
		settled++
	}

	if err := tx.Commit(ctx); err != nil {
		logger.Warn("ledger settle commit failed", zap.Error(err))
		return
	}
	committed = true

	if settled > 0 {
		logger.Info("ledger settle batch committed",
			zap.Int("picked", len(keys)),
			zap.Int("settled", settled),
		)
	}
}

// stampUsageLogs flags all currently-pending usage logs for the given account
// as settled, attributing them to the just-emitted settle event_id. Runs inside
// the same tx as SettleOne so a single failure rolls the whole settle back.
//
// Note: ai_usage_logs.user_id is NULL for tenant-owned keys, but the ledger
// row's user_id is the empty string. We match accordingly.
func stampUsageLogs(ctx context.Context, tx pgx.Tx, k Key, eventID string) error {
	if eventID == "" {
		return errors.New("ledger: stampUsageLogs requires event_id")
	}
	var cmd string
	var args []any
	if k.UserID == "" {
		cmd = `
			UPDATE ai_usage_logs
			SET settled_event_id = $1,
			    settled_at       = NOW(),
			    billing_status   = 'settled'
			WHERE key_owner_type = $2
			  AND tenant_id      = $3
			  AND user_id IS NULL
			  AND billing_status = 'pending_settle'
			  AND settled_event_id IS NULL
		`
		args = []any{eventID, string(k.OwnerType), k.TenantID}
	} else {
		cmd = `
			UPDATE ai_usage_logs
			SET settled_event_id = $1,
			    settled_at       = NOW(),
			    billing_status   = 'settled'
			WHERE key_owner_type = $2
			  AND tenant_id      = $3
			  AND user_id        = $4
			  AND billing_status = 'pending_settle'
			  AND settled_event_id IS NULL
		`
		args = []any{eventID, string(k.OwnerType), k.TenantID, k.UserID}
	}
	if _, err := tx.Exec(ctx, cmd, args...); err != nil {
		return fmt.Errorf("ledger: stamp usage logs: %w", err)
	}
	return nil
}

package asynctask

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// reaperLoop recovers orphaned leases and deletes expired rows.
func (e *Engine) reaperLoop(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.ReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.reapOnce(ctx)
			e.sweepExpired(ctx)
		}
	}
}

// reapOnce takes back tasks whose worker stopped renewing the lease.
//
// Two passes, because an orphan is not automatically retryable: an attempt that
// already reached billing must not run twice. reapRetryable takes only those
// with attempts left and no usage log; reapDead fails whatever is left.
//
// Order matters — retryable first, or reapDead would fail rows that deserved
// another attempt.
func (e *Engine) reapOnce(ctx context.Context) {
	retried, err := e.store.reapRetryable(ctx, e.cfg.ReapBatch)
	if err != nil {
		e.logger.Warn("async task reaper: requeueing orphans failed", zap.Error(err))
		return
	}
	if retried > 0 {
		e.logger.Info("async task reaper: requeued orphaned tasks", zap.Int64("count", retried))
		e.signal()
	}

	failed, err := e.store.reapDead(ctx, e.cfg.ReapBatch)
	if err != nil {
		e.logger.Warn("async task reaper: failing dead orphans failed", zap.Error(err))
		return
	}
	if failed > 0 {
		e.logger.Info("async task reaper: failed orphaned tasks with no attempts left",
			zap.Int64("count", failed))
		e.signalDelivery()
	}
	deadDeliveries, err := e.store.reapDeadDeliveries(ctx, e.cfg.ReapBatch)
	if err != nil {
		e.logger.Warn("async task reaper: failing exhausted webhook deliveries failed", zap.Error(err))
	} else if deadDeliveries > 0 {
		e.logger.Info("async task reaper: failed exhausted webhook deliveries",
			zap.Int64("count", deadDeliveries))
	}
}

// sweepExpired deletes rows past their TTL, giving each type a chance to clean
// up side files it owns first.
func (e *Engine) sweepExpired(ctx context.Context) {
	rows, err := e.store.listExpired(ctx, e.cfg.ReapBatch)
	if err != nil {
		e.logger.Warn("async task reaper: listing expired tasks failed", zap.Error(err))
		return
	}
	for _, row := range rows {
		if reg, ok := e.registry.lookup(row.Type); ok {
			if expirer, ok := reg.handler.(Expirer); ok {
				// Best effort: a handler that cannot clean up must not wedge
				// expiry, or one broken type would grow the table forever.
				if err := expirer.OnExpire(ctx, taskFromRow(row)); err != nil {
					e.logger.Warn("async task reaper: expiry hook failed; deleting anyway",
						zap.String("task_id", row.ID),
						zap.String("task_type", row.Type),
						zap.Error(err))
				}
			}
		}
		if err := e.store.deleteTask(ctx, row.ID); err != nil {
			e.logger.Warn("async task reaper: deleting expired task failed",
				zap.String("task_id", row.ID), zap.Error(err))
		}
	}
	if len(rows) > 0 {
		e.logger.Info("async task reaper: removed expired tasks", zap.Int("count", len(rows)))
	}
}

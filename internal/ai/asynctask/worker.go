package asynctask

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
)

// workerLoop claims and runs tasks until ctx is cancelled.
//
// After finishing one task it loops straight back to claiming, and only waits
// on the wake signal or the poll tick once the queue is dry. That keeps a burst
// draining at full speed without depending on one signal per task.
func (e *Engine) workerLoop(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	for {
		worked, err := e.claimAndRun(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			e.logger.Warn("async task worker: claim failed", zap.Error(err))
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-ticker.C:
		}
	}
}

// claimAndRun takes at most one task and runs it to completion. It reports
// whether it did any work, so the caller knows to keep draining.
func (e *Engine) claimAndRun(ctx context.Context) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	claimed, ok, err := e.store.claim(ctx, e.registry.types(), e.cfg.MaxInFlightPerTenant, e.workerID, e.cfg.LeaseTTL)
	if err != nil || !ok {
		return false, err
	}
	e.runClaimed(ctx, claimed)
	return true, nil
}

// runClaimed executes one claimed task and writes its terminal state.
func (e *Engine) runClaimed(parent context.Context, claimed claimedTask) {
	// The run context is detached from the poll loop only by cancellation
	// scope, not lifetime: shutdown must still interrupt in-flight work.
	runCtx, cancel := context.WithCancel(parent)

	e.registerCancel(claimed.ID, cancel)
	defer e.unregisterCancel(claimed.ID)

	// The heartbeat both holds the lease and listens for cancellation: zero rows
	// means the lease was taken or the task was cancelled, and either way this
	// worker must stop.
	beat := time.NewTicker(e.cfg.LeaseTTL / 3)
	defer beat.Stop()
	heartbeatDone := make(chan struct{})
	defer func() {
		cancel()
		<-heartbeatDone
	}()
	go func() {
		defer close(heartbeatDone)
		for {
			select {
			case <-runCtx.Done():
				return
			case <-beat.C:
				heartbeatCtx, heartbeatCancel := context.WithTimeout(runCtx, 5*time.Second)
				held, err := e.store.heartbeat(heartbeatCtx, claimed.ID, e.workerID, e.cfg.LeaseTTL)
				heartbeatCancel()
				if err != nil {
					e.logger.Warn("async task: heartbeat failed",
						zap.String("task_id", claimed.ID), zap.Error(err))
					continue
				}
				if !held {
					e.logger.Info("async task: lease lost or task cancelled; stopping",
						zap.String("task_id", claimed.ID))
					cancel()
					return
				}
			}
		}
	}()

	res, err := e.execute(runCtx, claimed)
	if err != nil {
		// Retryable and out of attempts are handled by the reaper via the lease,
		// not here: dropping the lease is the one signal that works whether we
		// failed cleanly or the process died mid-task.
		if IsRetryable(err) && claimed.Attempt < e.maxAttempts(claimed.Type) {
			e.logger.Warn("async task: retryable failure; leaving lease to expire",
				zap.String("task_id", claimed.ID), zap.Int("attempt", claimed.Attempt), zap.Error(err))
			return
		}
		res = Result{
			Status: domain.TaskFailed,
			Failure: &Failure{
				Code:           "task_execution_failed",
				Message:        "the task could not be executed",
				InternalDetail: err.Error(),
				Step:           "execute",
			},
		}
	}

	e.writeTerminal(parent, claimed, res)
}

// execute resolves the subject and hands off to the registered handler.
func (e *Engine) execute(ctx context.Context, claimed claimedTask) (Result, error) {
	reg, ok := e.registry.lookup(claimed.Type)
	if !ok {
		// Claimed by type filter, so this means the row's type was registered at
		// claim time and vanished — impossible while the registry is frozen.
		return Result{}, fmt.Errorf("no handler registered for task type %q", claimed.Type)
	}

	subject, err := e.deps.Subjects.Resolve(ctx, claimed.SubjectRef)
	if err != nil {
		// The credential is gone (revoked, deleted). Running the task anyway
		// would execute it unauthorized, so fail it outright.
		return Result{
			Status: domain.TaskFailed,
			Failure: &Failure{
				Code:           "subject_unavailable",
				Message:        "the credential that submitted this task is no longer valid",
				InternalDetail: err.Error(),
				Step:           "authn",
			},
		}, nil
	}

	return reg.handler.Execute(ctx, Task{
		ID:        claimed.ID,
		Type:      claimed.Type,
		ModelCode: claimed.ModelCode,
		Input:     claimed.Input,
		Attempt:   claimed.Attempt,
		Subject:   subject,
		RequestID: claimed.RequestID,
	})
}

// writeTerminal persists the outcome, scrubbing admin-only detail first.
func (e *Engine) writeTerminal(ctx context.Context, claimed claimedTask, res Result) {
	if res.Status != domain.TaskCompleted && res.Status != domain.TaskFailed {
		e.logger.Error("async task: handler returned a non-terminal status",
			zap.String("task_id", claimed.ID), zap.String("status", string(res.Status)))
		res = Result{
			Status: domain.TaskFailed,
			Failure: &Failure{
				Code:    "task_execution_failed",
				Message: "the task could not be executed",
				Step:    "execute",
			},
		}
	}
	if res.Status == domain.TaskFailed {
		if res.Failure == nil {
			res.Failure = &Failure{Code: "task_execution_failed", Message: "the task could not be executed"}
		}
		res.Failure.InternalDetail = e.deps.RedactDetail(res.Failure.InternalDetail)
	}

	// The run context may already be cancelled (shutdown, lost lease); the
	// terminal write must still land, so use a fresh deadline.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	written, err := e.store.complete(writeCtx, claimed.ID, e.workerID, res)
	if err != nil {
		e.logger.Error("async task: writing terminal state failed",
			zap.String("task_id", claimed.ID), zap.Error(err))
		return
	}
	if !written {
		// Lost the lease or the task was cancelled. Not an error: the row is
		// authoritative and we are the stale writer.
		e.logger.Info("async task: terminal state discarded; no longer lease holder",
			zap.String("task_id", claimed.ID))
		return
	}
	e.signalDelivery()
	e.logger.Info("async task finished",
		zap.String("task_id", claimed.ID),
		zap.String("task_type", claimed.Type),
		zap.String("status", string(res.Status)),
		zap.Int("attempt", claimed.Attempt),
	)
}

func (e *Engine) maxAttempts(taskType string) int {
	if reg, ok := e.registry.lookup(taskType); ok {
		return reg.opts.MaxAttempts
	}
	return 1
}

package riskcontrol

import (
	"context"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
)

const (
	defaultWorkerCount = 4
	workerQueueCap     = 4096
)

// WorkerTask is one observe-mode detection job: config is captured at
// enqueue time so every task in flight is evaluated against a consistent
// snapshot even if an admin edits the config mid-flight.
type WorkerTask struct {
	Config domain.RiskControlConfig
	Input  CheckInput
}

// Worker runs observe-mode moderation checks off the request path: the
// pipeline step enqueues and returns immediately, and a small pool of
// goroutines drains the queue, calling the moderation API and persisting
// results. A full queue drops new tasks rather than blocking — moderation
// in observe mode is best-effort audit, never a source of request latency
// or backpressure.
type Worker struct {
	checker *Checker
	ch      chan WorkerTask
	logger  *zap.Logger
}

func NewWorker(checker *Checker, logger *zap.Logger) *Worker {
	return &Worker{checker: checker, ch: make(chan WorkerTask, workerQueueCap), logger: logger}
}

// Start launches workerCount goroutines draining the queue until ctx is
// cancelled (server shutdown). workerCount <= 0 uses defaultWorkerCount.
func (w *Worker) Start(ctx context.Context, workerCount int) {
	if workerCount <= 0 {
		workerCount = defaultWorkerCount
	}
	for range workerCount {
		go w.run(ctx)
	}
}

func (w *Worker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-w.ch:
			if !ok {
				return
			}
			w.process(ctx, task)
		}
	}
}

func (w *Worker) process(ctx context.Context, task WorkerTask) {
	det := w.checker.Detect(ctx, task.Config, task.Input.Text)
	w.checker.Record(ctx, task.Config, task.Input, det, domain.RiskControlModeObserve)
}

// Submit enqueues a task without blocking the caller.
func (w *Worker) Submit(task WorkerTask) {
	select {
	case w.ch <- task:
	default:
		if w.logger != nil {
			w.logger.Warn("risk_control: async moderation queue full, dropping task")
		}
	}
}

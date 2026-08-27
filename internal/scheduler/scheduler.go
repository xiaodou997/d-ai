package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	domain "xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/invariants"
	"xiaodou/dai/internal/billing/ledger"
)

// jwtKeyRetirer 定义 JWT 密钥退役接口，避免循环依赖
type jwtKeyRetirer interface {
	RetireExpiredGraceKeys(ctx context.Context) error
}

// paymentSweeper 定义支付兜底扫描接口，避免循环依赖（实现见 internal/payment/service）。
type paymentSweeper interface {
	SweepOnce(ctx context.Context) error
}

type paymentOrderCleaner interface {
	CleanupClosedOrders(ctx context.Context) error
}

const (
	TaskLotExpiry             = "lot_expiry"
	TaskBillingReconciliation = "billing_reconciliation"
	TaskJWTKeyRetire          = "jwt_key_retire"
	TaskPaymentSweep          = "payment_sweep"
	TaskPaymentCleanup        = "payment_cleanup"

	// Payment and ledger workers must not keep a database session (or an
	// upstream request) forever. The advisory lock is released in a fresh
	// short-lived context even when the operation context has timed out.
	schedulerTaskTimeout  = 5 * time.Minute
	advisoryUnlockTimeout = 5 * time.Second
)

var (
	schedulerTaskRuns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dai_scheduler_task_runs_total",
		Help: "Scheduler task executions by task name and outcome.",
	}, []string{"task", "outcome"})
	schedulerTaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "dai_scheduler_task_duration_seconds",
		Help: "Scheduler task execution duration in seconds.",
	}, []string{"task"})
	schedulerTaskRunning = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dai_scheduler_task_running",
		Help: "One while a scheduler task is executing.",
	}, []string{"task"})
	schedulerTaskFailures = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dai_scheduler_task_consecutive_failures",
		Help: "Consecutive failures observed for a scheduler task.",
	}, []string{"task"})
	schedulerTaskSkips = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dai_scheduler_task_skips_total",
		Help: "Scheduler task executions skipped, grouped by bounded reason.",
	}, []string{"task", "reason"})
)

// TaskSnapshot is the operational state of one scheduler task. It is safe to
// expose directly from the management health endpoint.
type TaskSnapshot struct {
	Running             bool       `json:"running"`
	LastStartedAt       *time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt      *time.Time `json:"last_finished_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastError           string     `json:"last_error,omitempty"`
	SkipCount           int        `json:"skip_count"`
	LastSkippedAt       *time.Time `json:"last_skipped_at,omitempty"`
	LastSkipReason      string     `json:"last_skip_reason,omitempty"`
}

// HealthSnapshot is the scheduler-level health projection. A task failure is
// observable here while HTTP readiness remains governed by infrastructure
// probes; the next scheduled cycle is the retry mechanism.
type HealthSnapshot struct {
	Started bool                    `json:"started"`
	Stopped bool                    `json:"stopped"`
	Tasks   map[string]TaskSnapshot `json:"tasks"`
}

type taskSkippedError struct{ reason string }

func (e *taskSkippedError) Error() string { return "scheduler task skipped: " + e.reason }

// Scheduler 定时任务调度器
type Scheduler struct {
	pool           *pgxpool.Pool
	keyRetirer     jwtKeyRetirer
	paymentSweeper paymentSweeper
	paymentCleaner paymentOrderCleaner
	logger         *zap.Logger
	stopChan       chan struct{}
	lifecycleMu    sync.Mutex
	stopMu         sync.Mutex
	started        bool
	stopped        bool
	stopClosed     bool
	workerCtx      context.Context
	workerCancel   context.CancelFunc
	workers        sync.WaitGroup
	taskMu         sync.RWMutex
	tasks          map[string]TaskSnapshot
}

// NewScheduler 创建调度器
func NewScheduler(pool *pgxpool.Pool, keyRetirer jwtKeyRetirer, paymentSweeper paymentSweeper, logger *zap.Logger) *Scheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	cleaner, _ := paymentSweeper.(paymentOrderCleaner)
	return &Scheduler{
		pool:           pool,
		keyRetirer:     keyRetirer,
		paymentSweeper: paymentSweeper,
		paymentCleaner: cleaner,
		logger:         logger,
		stopChan:       make(chan struct{}),
		tasks: map[string]TaskSnapshot{
			TaskLotExpiry: {}, TaskJWTKeyRetire: {},
			TaskBillingReconciliation: {},
			TaskPaymentSweep:          {}, TaskPaymentCleanup: {},
		},
	}
}

// taskContext returns the scheduler-owned context for one task invocation.
// Manual RunAllTasks calls made before Start retain a bounded standalone
// context, while process-owned workers are cancelled as part of Stop.
func (s *Scheduler) taskContext() context.Context {
	if s == nil {
		return context.Background()
	}
	s.lifecycleMu.Lock()
	ctx := s.workerCtx
	s.lifecycleMu.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// Health returns a lock-safe snapshot for diagnostics and management probes.
func (s *Scheduler) Health() HealthSnapshot {
	s.lifecycleMu.Lock()
	started, stopped := s.started, s.stopped
	s.lifecycleMu.Unlock()
	s.taskMu.RLock()
	tasks := make(map[string]TaskSnapshot, len(s.tasks))
	for name, state := range s.tasks {
		tasks[name] = state
	}
	s.taskMu.RUnlock()
	return HealthSnapshot{Started: started, Stopped: stopped, Tasks: tasks}
}

func (s *Scheduler) runTask(name string, task func() error) {
	startedAt := time.Now()
	outcome := "success"
	defer func() {
		schedulerTaskRuns.WithLabelValues(name, outcome).Inc()
		schedulerTaskDuration.WithLabelValues(name).Observe(time.Since(startedAt).Seconds())
	}()

	s.taskStarted(name)
	err := task()
	var skipped *taskSkippedError
	if errors.As(err, &skipped) {
		outcome = "skipped"
		s.taskSkipped(name, skipped.reason)
		return
	}
	s.taskFinished(name, err)
	if err != nil {
		outcome = "failure"
		if s.logger != nil {
			s.logger.Error("scheduler task failed", zap.String("task", name), zap.Error(err))
		}
	}
}

func (s *Scheduler) taskStarted(name string) {
	now := time.Now().UTC()
	s.taskMu.Lock()
	state := s.tasks[name]
	state.Running = true
	state.LastStartedAt = &now
	s.tasks[name] = state
	s.taskMu.Unlock()
	schedulerTaskRunning.WithLabelValues(name).Inc()
}

func (s *Scheduler) taskFinished(name string, err error) {
	now := time.Now().UTC()
	s.taskMu.Lock()
	state := s.tasks[name]
	state.Running = false
	state.LastFinishedAt = &now
	if err == nil {
		state.LastSuccessAt = &now
		state.ConsecutiveFailures = 0
		state.LastError = ""
	} else {
		state.LastFailureAt = &now
		state.ConsecutiveFailures++
		state.LastError = err.Error()
	}
	s.tasks[name] = state
	s.taskMu.Unlock()
	schedulerTaskRunning.WithLabelValues(name).Dec()
	schedulerTaskFailures.WithLabelValues(name).Set(float64(state.ConsecutiveFailures))
}

func (s *Scheduler) taskSkipped(name, reason string) {
	now := time.Now().UTC()
	s.taskMu.Lock()
	state := s.tasks[name]
	state.Running = false
	state.LastFinishedAt = &now
	state.SkipCount++
	state.LastSkippedAt = &now
	state.LastSkipReason = reason
	s.tasks[name] = state
	s.taskMu.Unlock()
	schedulerTaskRunning.WithLabelValues(name).Dec()
	schedulerTaskSkips.WithLabelValues(name, reason).Inc()
	schedulerTaskFailures.WithLabelValues(name).Set(float64(state.ConsecutiveFailures))
}

// Start 启动定时任务
func (s *Scheduler) Start(ctxs ...context.Context) {
	if s == nil {
		return
	}
	ctx := context.Background()
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}
	s.lifecycleMu.Lock()
	if s.started || s.stopped {
		s.lifecycleMu.Unlock()
		return
	}
	s.started = true
	s.workerCtx, s.workerCancel = context.WithCancel(ctx)
	s.workers.Add(5)
	s.lifecycleMu.Unlock()

	s.logger.Info("Scheduler started")

	go func() { defer s.workers.Done(); s.runLotExpiryTask() }()
	go func() { defer s.workers.Done(); s.runBillingReconciliationTask() }()
	go func() { defer s.workers.Done(); s.runJWTKeyRetireTask() }()
	go func() { defer s.workers.Done(); s.runPaymentSweepTask() }()
	go func() { defer s.workers.Done(); s.runPaymentCleanupTask() }()
}

// ==================== 资金不变量对账 ====================

// The checker is a full read-only scan, so it runs less often than the hot
// path and is protected by a transaction-scoped advisory lock. Every replica
// can schedule it, but only one replica performs the expensive snapshot.
const billingReconciliationInterval = 15 * time.Minute

func (s *Scheduler) runBillingReconciliationTask() {
	s.runTask(TaskBillingReconciliation, s.reconcileBilling)
	ctx := s.taskContext()
	ticker := time.NewTicker(billingReconciliationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTask(TaskBillingReconciliation, s.reconcileBilling)
		}
	}
}

func (s *Scheduler) reconcileBilling() error {
	if s.pool == nil {
		return &taskSkippedError{reason: "billing_reconciliation_not_configured"}
	}
	ctx, cancel := context.WithTimeout(s.taskContext(), schedulerTaskTimeout)
	defer cancel()

	// RepeatableRead makes all seven invariant queries observe one MVCC
	// snapshot. The xact-scoped lock uses the same transaction, so a second
	// replica can skip without holding a session lock across pool connections.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin billing reconciliation: %w", err)
	}
	defer tx.Rollback(ctx)

	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, "dai_scheduler_billing_reconciliation").Scan(&locked); err != nil {
		return fmt.Errorf("acquire billing reconciliation lock: %w", err)
	}
	if !locked {
		return &taskSkippedError{reason: "billing_reconciliation_lock_held"}
	}

	report, err := invariants.Check(ctx, tx)
	if err != nil {
		return fmt.Errorf("run billing reconciliation: %w", err)
	}
	if !report.Healthy() {
		publishBillingReconciliationMetrics(report, time.Now().UTC())
		return fmt.Errorf("billing reconciliation found %d invariant violations: %w", len(report.Violations), report.Err())
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit billing reconciliation snapshot: %w", err)
	}
	publishBillingReconciliationMetrics(report, time.Now().UTC())
	return nil
}

// Stop 停止定时任务
func (s *Scheduler) Stop(ctxs ...context.Context) error {
	if s == nil {
		return nil
	}
	ctx := context.Background()
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}
	s.stopMu.Lock()
	defer s.stopMu.Unlock()

	s.lifecycleMu.Lock()
	if !s.stopped {
		s.stopped = true
	}
	started := s.started
	cancel := s.workerCancel
	stopChan := s.stopChan
	s.lifecycleMu.Unlock()
	if !started {
		return nil
	}
	if cancel != nil {
		cancel()
	}
	if stopChan != nil {
		s.lifecycleMu.Lock()
		// Only the first Stop closes the channel; subsequent calls are retries
		// that continue waiting with a fresh caller deadline.
		firstStop := !s.stopClosed
		if firstStop {
			s.stopClosed = true
		}
		s.lifecycleMu.Unlock()
		if firstStop {
			close(stopChan)
		}
	}
	done := make(chan struct{})
	go func() {
		s.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		s.logger.Info("Scheduler stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ==================== 额度批次过期结算 ====================

// 过期不再是查询时的时间过滤，而是一次真实的余额扣减，所以它必须跑得足够勤：
// 一笔额度的过期时刻和余额反映出来的时刻之间就是这个间隔。
const lotExpiryInterval = 5 * time.Minute

// 一轮最多结算多少个批次。超出的部分下一轮继续，避免单次事务过大。
const lotExpiryBatch = 500

func (s *Scheduler) runLotExpiryTask() {
	ctx := s.taskContext()
	ticker := time.NewTicker(lotExpiryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTask(TaskLotExpiry, s.settleExpiredLots)
		}
	}
}

// settleExpiredLots removes the unspent remainder of every lot whose validity
// window has closed. Repeating is safe: expired_at is the idempotency anchor.
func (s *Scheduler) settleExpiredLots() error {
	ctx, cancel := context.WithTimeout(s.taskContext(), schedulerTaskTimeout)
	defer cancel()
	now := domain.NowUTC()

	total := 0
	for {
		settled, err := s.settleExpiredLotBatch(ctx, now)
		if err != nil {
			return fmt.Errorf("结算过期额度批次失败: %w", err)
		}
		total += settled
		if settled < lotExpiryBatch {
			break
		}
	}
	if total > 0 {
		s.logger.Info("[定时任务] 过期额度批次结算完成", zap.Int("lotCount", total))
	}
	return nil
}

func (s *Scheduler) settleExpiredLotBatch(ctx context.Context, now time.Time) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	settled, err := ledger.ExpireDueLots(ctx, tx, now, lotExpiryBatch)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return settled, nil
}

// ==================== JWT 密钥退役 ====================

func (s *Scheduler) runJWTKeyRetireTask() {
	ctx := s.taskContext()
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	select {
	case <-s.stopChan:
		return
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTask(TaskJWTKeyRetire, s.retireExpiredGraceKeys)
		}
	}
}

func (s *Scheduler) retireExpiredGraceKeys() error {
	if s.keyRetirer == nil {
		return &taskSkippedError{reason: "key_retire_not_configured"}
	}
	ctx, cancel := context.WithTimeout(s.taskContext(), schedulerTaskTimeout)
	defer cancel()
	return s.keyRetirer.RetireExpiredGraceKeys(ctx)
}

// ==================== 支付兜底 sweep（超时关单 + 在途补偿，设计文档 §4.5） ====================

func (s *Scheduler) runPaymentSweepTask() {
	s.runTask(TaskPaymentSweep, s.sweepPayments)
	ctx := s.taskContext()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTask(TaskPaymentSweep, s.sweepPayments)
		}
	}
}

func (s *Scheduler) sweepPayments() error {
	if s.paymentSweeper == nil {
		return &taskSkippedError{reason: "payment_sweep_not_configured"}
	}
	ctx, cancel := context.WithTimeout(s.taskContext(), schedulerTaskTimeout)
	defer cancel()
	return s.withAdvisoryLock(ctx, "dai_scheduler_payment", "payment_sweep_advisory_lock_held", s.paymentSweeper.SweepOnce)
}

const closedOrderCleanupInterval = 24 * time.Hour

func (s *Scheduler) runPaymentCleanupTask() {
	s.runTask(TaskPaymentCleanup, s.cleanupClosedOrders)
	ctx := s.taskContext()
	ticker := time.NewTicker(closedOrderCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTask(TaskPaymentCleanup, s.cleanupClosedOrders)
		}
	}
}

func (s *Scheduler) cleanupClosedOrders() error {
	if s.paymentCleaner == nil {
		return &taskSkippedError{reason: "payment_cleanup_not_configured"}
	}
	ctx, cancel := context.WithTimeout(s.taskContext(), schedulerTaskTimeout)
	defer cancel()
	return s.withAdvisoryLock(ctx, "dai_scheduler_payment_cleanup", "payment_cleanup_advisory_lock_held", s.paymentCleaner.CleanupClosedOrders)
}

// withAdvisoryLock pins acquisition, execution, and release to one physical
// PostgreSQL connection. pg_advisory_lock is session-scoped; using pool.QueryRow
// for acquisition and pool.Exec for release can silently hand those operations
// to different sessions and leak the lock until the first session is recycled.
func (s *Scheduler) withAdvisoryLock(ctx context.Context, key, skipReason string, task func(context.Context) error) (err error) {
	if s.pool == nil {
		return errors.New("scheduler database pool is not configured")
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire scheduler database connection: %w", err)
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key).Scan(&locked); err != nil {
		return fmt.Errorf("acquire %s: %w", key, err)
	}
	if !locked {
		return &taskSkippedError{reason: skipReason}
	}

	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), advisoryUnlockTimeout)
		defer cancel()
		var unlocked bool
		unlockErr := conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1))`, key).Scan(&unlocked)
		if unlockErr == nil && !unlocked {
			unlockErr = errors.New("database reported advisory lock was not held")
		}
		if unlockErr != nil {
			unlockErr = fmt.Errorf("release %s: %w", key, unlockErr)
			if err == nil {
				err = unlockErr
			} else {
				err = errors.Join(err, unlockErr)
			}
		}
	}()

	return task(ctx)
}

// RunAllTasks 手动执行所有任务（用于测试）
func (s *Scheduler) RunAllTasks() {
	s.logger.Info("手动触发所有定时任务")
	s.runTask(TaskLotExpiry, s.settleExpiredLots)
	s.runTask(TaskBillingReconciliation, s.reconcileBilling)
	s.runTask(TaskPaymentSweep, s.sweepPayments)
	s.runTask(TaskPaymentCleanup, s.cleanupClosedOrders)
}

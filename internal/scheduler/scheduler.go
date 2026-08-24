package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	domain "xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/ledger"
)

// jwtKeyRetirer 定义 JWT 密钥退役接口，避免循环依赖
type jwtKeyRetirer interface {
	RetireExpiredGraceKeys() error
}

// paymentSweeper 定义支付兜底扫描接口，避免循环依赖（实现见 internal/payment/service）。
type paymentSweeper interface {
	SweepOnce(ctx context.Context) error
}

type paymentOrderCleaner interface {
	CleanupClosedOrders(ctx context.Context) error
}

const (
	TaskLotExpiry      = "lot_expiry"
	TaskJWTKeyRetire   = "jwt_key_retire"
	TaskPaymentSweep   = "payment_sweep"
	TaskPaymentCleanup = "payment_cleanup"
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
	started        bool
	stopped        bool
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
			TaskPaymentSweep: {}, TaskPaymentCleanup: {},
		},
	}
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
	s.taskStarted(name)
	err := task()
	var skipped *taskSkippedError
	if errors.As(err, &skipped) {
		s.taskSkipped(name, skipped.reason)
		return
	}
	s.taskFinished(name, err)
	if err != nil && s.logger != nil {
		s.logger.Error("scheduler task failed", zap.String("task", name), zap.Error(err))
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
}

// Start 启动定时任务
func (s *Scheduler) Start() {
	s.lifecycleMu.Lock()
	if s.started || s.stopped {
		s.lifecycleMu.Unlock()
		return
	}
	s.started = true
	s.workers.Add(4)
	s.lifecycleMu.Unlock()

	s.logger.Info("Scheduler started")

	go func() { defer s.workers.Done(); s.runLotExpiryTask() }()
	go func() { defer s.workers.Done(); s.runJWTKeyRetireTask() }()
	go func() { defer s.workers.Done(); s.runPaymentSweepTask() }()
	go func() { defer s.workers.Done(); s.runPaymentCleanupTask() }()
}

// Stop 停止定时任务
func (s *Scheduler) Stop() {
	s.lifecycleMu.Lock()
	if s.stopped {
		s.lifecycleMu.Unlock()
		return
	}
	s.stopped = true
	started := s.started
	close(s.stopChan)
	s.lifecycleMu.Unlock()
	if started {
		s.workers.Wait()
	}
	s.logger.Info("Scheduler stopped")
}

// ==================== 额度批次过期结算 ====================

// 过期不再是查询时的时间过滤，而是一次真实的余额扣减，所以它必须跑得足够勤：
// 一笔额度的过期时刻和余额反映出来的时刻之间就是这个间隔。
const lotExpiryInterval = 5 * time.Minute

// 一轮最多结算多少个批次。超出的部分下一轮继续，避免单次事务过大。
const lotExpiryBatch = 500

func (s *Scheduler) runLotExpiryTask() {
	ticker := time.NewTicker(lotExpiryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.runTask(TaskLotExpiry, s.settleExpiredLots)
		}
	}
}

// settleExpiredLots removes the unspent remainder of every lot whose validity
// window has closed. Repeating is safe: expired_at is the idempotency anchor.
func (s *Scheduler) settleExpiredLots() error {
	ctx := context.Background()
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
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	select {
	case <-s.stopChan:
		return
	case <-timer.C:
	}

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
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
	if err := s.keyRetirer.RetireExpiredGraceKeys(); err != nil {
		return err
	}
	return nil
}

// ==================== 支付兜底 sweep（超时关单 + 在途补偿，设计文档 §4.5） ====================

func (s *Scheduler) runPaymentSweepTask() {
	s.runTask(TaskPaymentSweep, s.sweepPayments)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
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
	ctx := context.Background()

	var locked bool
	if err := s.pool.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('dai_scheduler_payment'))`).Scan(&locked); err != nil {
		return fmt.Errorf("acquire payment sweep advisory lock: %w", err)
	}
	if !locked {
		return &taskSkippedError{reason: "payment_sweep_advisory_lock_held"}
	}
	defer s.pool.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('dai_scheduler_payment'))`) //nolint

	return s.paymentSweeper.SweepOnce(ctx)
}

const closedOrderCleanupInterval = 24 * time.Hour

func (s *Scheduler) runPaymentCleanupTask() {
	s.runTask(TaskPaymentCleanup, s.cleanupClosedOrders)
	ticker := time.NewTicker(closedOrderCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
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
	ctx := context.Background()
	var locked bool
	if err := s.pool.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('dai_scheduler_payment_cleanup'))`).Scan(&locked); err != nil {
		return fmt.Errorf("acquire payment cleanup advisory lock: %w", err)
	}
	if !locked {
		return &taskSkippedError{reason: "payment_cleanup_advisory_lock_held"}
	}
	defer s.pool.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('dai_scheduler_payment_cleanup'))`) //nolint
	return s.paymentCleaner.CleanupClosedOrders(ctx)
}

// RunAllTasks 手动执行所有任务（用于测试）
func (s *Scheduler) RunAllTasks() {
	s.logger.Info("手动触发所有定时任务")
	s.runTask(TaskLotExpiry, s.settleExpiredLots)
	s.runTask(TaskPaymentSweep, s.sweepPayments)
	s.runTask(TaskPaymentCleanup, s.cleanupClosedOrders)
}

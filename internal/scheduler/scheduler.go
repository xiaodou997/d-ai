package scheduler

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	domain "xiaodou/dai/internal/billing"
)

// jwtKeyRetirer 定义 JWT 密钥退役接口，避免循环依赖
type jwtKeyRetirer interface {
	RetireExpiredGraceKeys() error
}

// paymentSweeper 定义支付兜底扫描接口，避免循环依赖（实现见 internal/payment/service）。
type paymentSweeper interface {
	SweepOnce(ctx context.Context)
}

// Scheduler 定时任务调度器
type Scheduler struct {
	pool           *pgxpool.Pool
	keyRetirer     jwtKeyRetirer
	paymentSweeper paymentSweeper
	logger         *zap.Logger
	stopChan       chan struct{}
}

// NewScheduler 创建调度器
func NewScheduler(pool *pgxpool.Pool, keyRetirer jwtKeyRetirer, paymentSweeper paymentSweeper, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		pool:           pool,
		keyRetirer:     keyRetirer,
		paymentSweeper: paymentSweeper,
		logger:         logger,
		stopChan:       make(chan struct{}),
	}
}

// Start 启动定时任务
func (s *Scheduler) Start() {
	s.logger.Info("Scheduler started")

	go s.runPackageExpiryTask()
	go s.runDepletedPackageTask()
	go s.runJWTKeyRetireTask()
	go s.runPaymentSweepTask()
}

// Stop 停止定时任务
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.logger.Info("Scheduler stopped")
}

// ==================== 资源包过期清理 ====================

func (s *Scheduler) runPackageExpiryTask() {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 1, 0, 0, 0, now.Location())

		select {
		case <-s.stopChan:
			return
		case <-time.After(next.Sub(now)):
			s.cleanupExpiredPackages()
		}
	}
}

func (s *Scheduler) cleanupExpiredPackages() {
	s.logger.Info("[定时任务] 开始清理过期额度包")

	ctx := context.Background()
	now := domain.NowUTC()

	result, err := s.pool.Exec(ctx, `
		UPDATE bill_credit_packages
		SET status = 'expired', updated_at = now()
		WHERE status = 'available'
		      AND expires_at IS NOT NULL
		      AND expires_at < $1
	`, now)
	if err != nil {
		s.logger.Error("[定时任务] 标记过期额度包失败", zap.Error(err))
		return
	}

	affected := result.RowsAffected()
	if affected == 0 {
		s.logger.Info("[定时任务] 没有需要清理的过期额度包")
		return
	}
	s.logger.Info("[定时任务] 过期额度包清理完成", zap.Int64("packageCount", affected))
}

// ==================== 耗尽资源包清理 ====================

func (s *Scheduler) runDepletedPackageTask() {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 1, 30, 0, 0, now.Location())

		select {
		case <-s.stopChan:
			return
		case <-time.After(next.Sub(now)):
			s.cleanupDepletedPackages()
		}
	}
}

func (s *Scheduler) cleanupDepletedPackages() {
	s.logger.Info("[定时任务] 开始清理耗尽额度包")

	ctx := context.Background()

	var depletedCount int
	s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bill_credit_packages
		WHERE status = 'available' AND remaining_credits <= 0
	`).Scan(&depletedCount)

	if depletedCount == 0 {
		s.logger.Info("[定时任务] 没有需要清理的耗尽额度包")
		return
	}

	now := domain.NowUTC()

	result, err := s.pool.Exec(ctx, `
		UPDATE bill_credit_packages
		SET status = 'depleted', updated_at = $1
		WHERE status = 'available' AND remaining_credits <= 0
	`, now)
	if err != nil {
		s.logger.Error("[定时任务] 清理耗尽额度包失败", zap.Error(err))
		return
	}

	s.logger.Info("[定时任务] 耗尽额度包清理完成",
		zap.Int64("depletedCount", result.RowsAffected()),
	)
}

// ==================== JWT 密钥退役 ====================

func (s *Scheduler) runJWTKeyRetireTask() {
	time.Sleep(5 * time.Minute)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.retireExpiredGraceKeys()
		}
	}
}

func (s *Scheduler) retireExpiredGraceKeys() {
	s.logger.Info("[定时任务] 检查 JWT 宽限期密钥退役")
	if s.keyRetirer == nil {
		return
	}
	if err := s.keyRetirer.RetireExpiredGraceKeys(); err != nil {
		s.logger.Error("[定时任务] JWT 密钥退役失败", zap.Error(err))
	} else {
		s.logger.Info("[定时任务] JWT 密钥退役检查完成")
	}
}

// ==================== 支付兜底 sweep（超时关单 + 在途补偿，设计文档 §4.5） ====================

func (s *Scheduler) runPaymentSweepTask() {
	s.sweepPayments()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.sweepPayments()
		}
	}
}

func (s *Scheduler) sweepPayments() {
	if s.paymentSweeper == nil {
		return
	}
	ctx := context.Background()

	var locked bool
	if err := s.pool.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('dai_scheduler_payment'))`).Scan(&locked); err != nil || !locked {
		return
	}
	defer s.pool.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('dai_scheduler_payment'))`) //nolint

	s.paymentSweeper.SweepOnce(ctx)
}

// RunAllTasks 手动执行所有任务（用于测试）
func (s *Scheduler) RunAllTasks() {
	s.logger.Info("手动触发所有定时任务")
	s.cleanupExpiredPackages()
	s.cleanupDepletedPackages()
	s.sweepPayments()
}

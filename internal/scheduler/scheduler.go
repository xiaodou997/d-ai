package scheduler

import (
	"context"
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

	go s.runLotExpiryTask()
	go s.runJWTKeyRetireTask()
	go s.runPaymentSweepTask()
}

// Stop 停止定时任务
func (s *Scheduler) Stop() {
	close(s.stopChan)
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
			s.settleExpiredLots()
		}
	}
}

// settleExpiredLots removes the unspent remainder of every lot whose validity
// window has closed. Repeating is safe: expired_at is the idempotency anchor.
func (s *Scheduler) settleExpiredLots() {
	ctx := context.Background()
	now := domain.NowUTC()

	total := 0
	for {
		settled, err := s.settleExpiredLotBatch(ctx, now)
		if err != nil {
			s.logger.Error("[定时任务] 结算过期额度批次失败", zap.Error(err))
			return
		}
		total += settled
		if settled < lotExpiryBatch {
			break
		}
	}
	if total > 0 {
		s.logger.Info("[定时任务] 过期额度批次结算完成", zap.Int("lotCount", total))
	}
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
	s.settleExpiredLots()
	s.sweepPayments()
}

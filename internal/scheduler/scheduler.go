package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	domain "xiaodou/dai/internal/billing"
	billingpg "xiaodou/dai/internal/billing/pg"
)

const (
	serviceInstanceRetention       = 24 * time.Hour
	serviceInstanceCleanupInterval = time.Hour
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
	pool                  *pgxpool.Pool
	keyRetirer            jwtKeyRetirer
	paymentSweeper        paymentSweeper
	logger                *zap.Logger
	stopChan              chan struct{}
	preAuthTimeoutMinutes int
	creditLeaseReaper     interface {
		ReapExpired(context.Context, int) (int, error)
	}
}

// NewScheduler 创建调度器
func NewScheduler(pool *pgxpool.Pool, keyRetirer jwtKeyRetirer, paymentSweeper paymentSweeper, logger *zap.Logger, preAuthTimeoutMinutes int) *Scheduler {
	if preAuthTimeoutMinutes <= 0 {
		preAuthTimeoutMinutes = 30
	}
	return &Scheduler{
		pool:                  pool,
		keyRetirer:            keyRetirer,
		paymentSweeper:        paymentSweeper,
		logger:                logger,
		stopChan:              make(chan struct{}),
		preAuthTimeoutMinutes: preAuthTimeoutMinutes,
	}
}

// Start 启动定时任务
func (s *Scheduler) Start() {
	s.logger.Info("Scheduler started")

	go s.runPreAuthTimeoutTask()
	go s.runPreAuthStatisticsTask()
	go s.runPackageExpiryTask()
	go s.runDepletedPackageTask()
	go s.runJWTKeyRetireTask()
	go s.runExpiredAuthCodeTask()
	go s.runServiceInstanceCleanupTask()
	go s.runPaymentSweepTask()
	go s.runCreditLeaseReaperTask()
}

func (s *Scheduler) WithCreditLeaseReaper(reaper interface {
	ReapExpired(context.Context, int) (int, error)
}) {
	s.creditLeaseReaper = reaper
}

func (s *Scheduler) runCreditLeaseReaperTask() {
	if s.creditLeaseReaper == nil {
		return
	}
	run := func() {
		released, err := s.creditLeaseReaper.ReapExpired(context.Background(), 200)
		if err != nil {
			s.logger.Error("credit lease reaper failed", zap.Error(err))
			return
		}
		if released > 0 {
			s.logger.Info("credit lease escrow released", zap.Int("count", released))
		}
	}
	run()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			run()
		}
	}
}

// Stop 停止定时任务
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.logger.Info("Scheduler stopped")
}

// ==================== 预授权超时释放 ====================

func (s *Scheduler) runPreAuthTimeoutTask() {
	s.releaseTimeoutPreAuth()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.releaseTimeoutPreAuth()
		}
	}
}

func (s *Scheduler) releaseTimeoutPreAuth() {
	ctx := context.Background()

	var locked bool
	if err := s.pool.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('urm_scheduler_preauth'))`).Scan(&locked); err != nil || !locked {
		s.logger.Info("[定时任务] 预授权释放任务已被其他实例持有，跳过")
		return
	}
	defer s.pool.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('urm_scheduler_preauth'))`) //nolint

	s.logger.Info("[定时任务] 开始释放超时预授权")

	cutoff := domain.NowUTC().Add(-time.Duration(s.preAuthTimeoutMinutes) * time.Minute)

	rows, err := s.pool.Query(ctx, `
		SELECT event_id,
		       COALESCE(tenant_id, ''),
		       COALESCE(user_id, ''),
		       tenant_credits,
		       user_credits
		FROM bill_events
		WHERE status = 'pending' AND created_at < $1
	`, cutoff)
	if err != nil {
		s.logger.Error("[定时任务] 查询超时预授权失败", zap.Error(err))
		return
	}
	defer rows.Close()

	type preAuthRow struct {
		eventID    string
		tenantID   string
		userID     string
		tenantCred *int64
		userCred   *int64
	}
	var events []preAuthRow

	for rows.Next() {
		var r preAuthRow
		if err := rows.Scan(&r.eventID, &r.tenantID, &r.userID, &r.tenantCred, &r.userCred); err != nil {
			continue
		}
		events = append(events, r)
	}

	if len(events) == 0 {
		s.logger.Info("[定时任务] 没有需要释放的超时预授权")
		return
	}

	s.logger.Info("[定时任务] 发现超时预授权", zap.Int("count", len(events)))

	successCount, failCount := 0, 0
	for _, ev := range events {
		if s.releaseSinglePreAuth(ctx, ev.eventID, ev.tenantID, ev.userID, ev.tenantCred, ev.userCred) {
			successCount++
		} else {
			failCount++
		}
	}

	s.logger.Info("[定时任务] 预授权释放完成",
		zap.Int("success", successCount),
		zap.Int("fail", failCount),
	)
}

func (s *Scheduler) releaseSinglePreAuth(ctx context.Context, eventID, tenantID, userID string, tenantCred, userCred *int64) bool {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.logger.Error("[定时任务] 开启事务失败", zap.Error(err))
		return false
	}
	defer tx.Rollback(ctx)

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM bill_events WHERE event_id=$1 FOR UPDATE`, eventID).Scan(&status); err != nil || status != "pending" {
		return false
	}

	now := domain.NowUTC()

	if tenantCred != nil && *tenantCred > 0 && tenantID != "" {
		if err := billingpg.ReduceTenantFrozen(ctx, tx, tenantID, *tenantCred); err != nil {
			s.logger.Error("[定时任务] 释放租户冻结额失败",
				zap.String("eventId", eventID), zap.Error(err))
			return false
		}
	}

	if userCred != nil && *userCred > 0 && userID != "" {
		if err := billingpg.ReduceUserFrozen(ctx, tx, userID, *userCred); err != nil {
			s.logger.Error("[定时任务] 释放用户冻结额失败",
				zap.String("eventId", eventID), zap.Error(err))
			return false
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE bill_events
		SET status = 'released', terminal_note = '预授权超时自动释放', finished_at = $1
		WHERE event_id = $2 AND status = 'pending'
	`, now, eventID)
	if err != nil {
		s.logger.Error("[定时任务] 更新事件状态失败",
			zap.String("eventId", eventID), zap.Error(err))
		return false
	}

	opJSON := fmt.Sprintf(`{"action":"auto_released","note":"预授权超时自动释放","at":%d}`, now.UnixMilli())
	_, err = tx.Exec(ctx, `
		UPDATE bill_events
		SET metadata = jsonb_set(
			COALESCE(metadata, '{}'),
			'{ops}',
			COALESCE(metadata->'ops', '[]'::jsonb) || $1::jsonb,
			true
		)
		WHERE event_id = $2
	`, opJSON, eventID)
	if err != nil {
		s.logger.Error("[定时任务] 写 auto_released op 失败",
			zap.String("eventId", eventID), zap.Error(err))
		return false
	}

	if err := tx.Commit(ctx); err != nil {
		return false
	}

	s.logger.Info("[预授权自动释放]",
		zap.String("eventId", eventID),
		zap.String("tenantId", tenantID),
		zap.String("userId", userID),
	)

	return true
}

// ==================== 预授权监控统计 ====================

func (s *Scheduler) runPreAuthStatisticsTask() {
	time.Sleep(30 * time.Minute)

	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.reportPreAuthStatistics()
		}
	}
}

func (s *Scheduler) reportPreAuthStatistics() {
	s.logger.Info("[监控统计] 开始统计预授权情况")

	ctx := context.Background()

	var activePreAuthCount int
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_events WHERE status = 'pending'`).Scan(&activePreAuthCount)

	s.logger.Info("[监控统计] 预授权状态", zap.Int("activeCount", activePreAuthCount))

	if activePreAuthCount > 1000 {
		s.logger.Warn("[监控告警] 进行中的预授权数量异常，请关注业务系统",
			zap.Int("count", activePreAuthCount),
		)
	}
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
	s.logger.Info("[定时任务] 开始清理过期积分包")

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
		s.logger.Error("[定时任务] 标记过期积分包失败", zap.Error(err))
		return
	}

	affected := result.RowsAffected()
	if affected == 0 {
		s.logger.Info("[定时任务] 没有需要清理的过期积分包")
		return
	}
	s.logger.Info("[定时任务] 过期积分包清理完成", zap.Int64("packageCount", affected))
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
	s.logger.Info("[定时任务] 开始清理耗尽积分包")

	ctx := context.Background()

	var depletedCount int
	s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bill_credit_packages
		WHERE status = 'available' AND remaining_credits <= 0
	`).Scan(&depletedCount)

	if depletedCount == 0 {
		s.logger.Info("[定时任务] 没有需要清理的耗尽积分包")
		return
	}

	now := domain.NowUTC()

	result, err := s.pool.Exec(ctx, `
		UPDATE bill_credit_packages
		SET status = 'depleted', updated_at = $1
		WHERE status = 'available' AND remaining_credits <= 0
	`, now)
	if err != nil {
		s.logger.Error("[定时任务] 清理耗尽积分包失败", zap.Error(err))
		return
	}

	s.logger.Info("[定时任务] 耗尽积分包清理完成",
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

// ==================== 过期 auth code 清理 ====================

func (s *Scheduler) runExpiredAuthCodeTask() {
	s.cleanupExpiredAuthCodes()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.cleanupExpiredAuthCodes()
		}
	}
}

func (s *Scheduler) cleanupExpiredAuthCodes() {
	ctx := context.Background()
	result, err := s.pool.Exec(ctx, `DELETE FROM auth_oauth_codes WHERE expires_at < now()`)
	if err != nil {
		s.logger.Error("[定时任务] 清理过期 auth code 失败", zap.Error(err))
		return
	}
	if affected := result.RowsAffected(); affected > 0 {
		s.logger.Info("[定时任务] 过期 auth code 清理完成", zap.Int64("count", affected))
	}
}

// ==================== 服务实例历史清理 ====================

func (s *Scheduler) runServiceInstanceCleanupTask() {
	s.cleanupStaleServiceInstances()

	ticker := time.NewTicker(serviceInstanceCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.cleanupStaleServiceInstances()
		}
	}
}

func (s *Scheduler) cleanupStaleServiceInstances() {
	cutoff := time.Now().UTC().Add(-serviceInstanceRetention)
	count, locked, err := s.cleanupServiceInstancesBefore(context.Background(), cutoff)
	if err != nil {
		s.logger.Error("[定时任务] 清理过期服务实例失败", zap.Error(err))
		return
	}
	if !locked {
		return
	}
	if count > 0 {
		s.logger.Info("[定时任务] 过期服务实例清理完成", zap.Int64("count", count))
	}
}

func (s *Scheduler) cleanupServiceInstancesBefore(ctx context.Context, cutoff time.Time) (int64, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)

	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext('urm_scheduler_service_instances'))`).Scan(&locked); err != nil {
		return 0, false, err
	}
	if !locked {
		return 0, false, nil
	}
	result, err := tx.Exec(ctx, `DELETE FROM gov_service_instances WHERE last_seen < $1`, cutoff)
	if err != nil {
		return 0, true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, true, err
	}
	return result.RowsAffected(), true, nil
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
	if err := s.pool.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('urm_scheduler_payment'))`).Scan(&locked); err != nil || !locked {
		return
	}
	defer s.pool.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('urm_scheduler_payment'))`) //nolint

	s.paymentSweeper.SweepOnce(ctx)
}

// RunAllTasks 手动执行所有任务（用于测试）
func (s *Scheduler) RunAllTasks() {
	s.logger.Info("手动触发所有定时任务")
	s.releaseTimeoutPreAuth()
	s.reportPreAuthStatistics()
	s.cleanupExpiredPackages()
	s.cleanupDepletedPackages()
	s.cleanupExpiredAuthCodes()
	s.cleanupStaleServiceInstances()
	s.sweepPayments()
}

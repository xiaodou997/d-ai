package subscription

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
)

// janitor 参数（沿用 ledger.Worker 的 ticker 模式；过期 UPDATE 幂等、卡单重放靠计费
// 幂等键，无需 advisory lock，多实例安全）。
const (
	janitorInterval = 60 * time.Second
	reconcileCutoff = time.Minute // 订单停滞多久后开始补偿
	reconcileLimit  = 32          // 单轮最多补偿多少单
)

// RunJanitor 后台卫生循环（阻塞，调用方以 go s.RunJanitor(ctx) 启动）：
//
//	a) 批量过期到期的 active 订阅（懒惰判定的兜底）
//	b) 卡单补偿：created/deducting 停滞超 reconcileCutoff 的订单，用同一幂等键重放
//	   strict debit → 成功则 FinalizeOrder 推进到 paid；明确余额不足则置 failed。
func (s *Service) RunJanitor(ctx context.Context) {
	t := time.NewTicker(janitorInterval)
	defer t.Stop()
	s.logger.Info("subscription janitor started", zap.Duration("interval", janitorInterval))
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("subscription janitor stopped")
			return
		case <-t.C:
			s.sweepOnce(ctx)
		}
	}
}

func (s *Service) sweepOnce(ctx context.Context) {
	// a) 批量过期
	if n, err := s.repo.ExpireDue(ctx); err != nil {
		s.logger.Warn("janitor: expire due subscriptions failed", zap.Error(err))
	} else if n > 0 {
		s.logger.Info("janitor: expired subscriptions", zap.Int64("count", n))
	}

	// b) 卡单补偿
	cutoff := time.Now().Add(-reconcileCutoff)
	orders, err := s.repo.ListReconcileOrders(ctx, cutoff, reconcileLimit)
	if err != nil {
		s.logger.Warn("janitor: list reconcile orders failed", zap.Error(err))
		return
	}
	for i := range orders {
		s.reconcileOrder(ctx, &orders[i])
	}
}

// reconcileOrder 补偿单个卡单：向前推进到 paid，绝不反向退款。
func (s *Service) reconcileOrder(ctx context.Context, order *Order) {
	priceMicro, ok := domain.CreditsToMicro(order.PriceCredits)
	if !ok {
		if _, ferr := s.repo.MarkOrderFailed(ctx, order.ID, "invalid_price"); ferr != nil {
			s.logger.Warn("janitor: mark invalid-price order failed",
				zap.String("order", order.OrderNo), zap.Error(ferr))
		}
		return
	}
	// 同一幂等键重放：已扣则返回同 event 不重扣，未扣则重试。
	resp, err := s.purchaser.DebitStrict(ctx, DebitRequest{
		IdempotencyKey: "ai-sub-" + order.OrderNo,
		TenantID:       order.TenantID,
		UserID:         order.UserID,
		Description:    "AI订阅套餐购买: " + order.PlanNameSnapshot,
		UserMicro:      priceMicro,
	})
	if err != nil {
		if errors.Is(err, ErrInsufficientBalance) {
			if _, ferr := s.repo.MarkOrderFailed(ctx, order.ID, "insufficient_balance"); ferr != nil {
				s.logger.Warn("janitor: mark stuck order failed",
					zap.String("order", order.OrderNo), zap.Error(ferr))
			}
			return
		}
		s.logger.Warn("janitor: consume retry failed, will retry next tick",
			zap.String("order", order.OrderNo), zap.Error(err))
		return
	}
	if _, err := s.repo.FinalizeOrder(ctx, order, resp.AuthorizationID); err != nil {
		s.logger.Warn("janitor: finalize retry failed, will retry next tick",
			zap.String("order", order.OrderNo), zap.Error(err))
	}
}

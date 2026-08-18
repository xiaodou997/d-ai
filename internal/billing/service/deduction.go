package service

import (
	"context"
	"encoding/json"
	"fmt"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/ledger"
	shared "xiaodou/dai/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// BatchOpError 批量操作单条失败记录
type BatchOpError struct {
	EventID string `json:"eventId"`
	Reason  string `json:"reason"`
}

// BatchOpResult 批量操作结果
type BatchOpResult struct {
	Succeeded          []string       `json:"succeeded"`
	Failed             []BatchOpError `json:"failed"`
	TotalTenantCredits int64          `json:"totalTenantCredits"`
	TotalUserCredits   int64          `json:"totalUserCredits"`
}

// DeductionService 账务运营操作：退款与充值撤销。
//
// 运行时的 AI 扣费不走这里 —— 它由 billing/outbox 消费者直接调用 ledger。
// 本类型只承载需要人工授权的、有审计意义的账务动作。
type DeductionService struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewDeductionService(pool *pgxpool.Pool, logger *zap.Logger) *DeductionService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DeductionService{pool: pool, logger: logger}
}

// Refund 全额退款（仅平台管理员可操作）。
//
// 退款就是把钱加回账户。账户余额是有符号的，所以「欠费的账户退款只清欠、不退现」
// 这条规则不需要任何代码来实现 —— 加法本身就是这个语义。
func (s *DeductionService) Refund(eventID, reason, operatorID string) error {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var tenantID, userID string
	var tenantCredits, userCredits *int64
	var status string
	err = tx.QueryRow(ctx, `
		SELECT tenant_id, COALESCE(user_id, ''), tenant_credits, user_credits, status
		FROM bill_events WHERE event_id = $1
	`, eventID).Scan(&tenantID, &userID, &tenantCredits, &userCredits, &status)
	if err != nil {
		return shared.ErrTransactionNotFound
	}
	if status != billing.EventStatusSucceeded {
		return fmt.Errorf("event not refundable (status=%s)", status)
	}

	now := billing.NowUTC()

	if tenantCredits != nil && *tenantCredits > 0 {
		if _, err := ledger.Grant(ctx, tx,
			ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID},
			*tenantCredits, nil, billing.PackageSourceRefund, ""); err != nil {
			return fmt.Errorf("退回租户余额失败：%w", err)
		}
	}
	if userCredits != nil && *userCredits > 0 && userID != "" {
		if _, err := ledger.Grant(ctx, tx,
			ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID},
			*userCredits, nil, billing.PackageSourceRefund, ""); err != nil {
			return fmt.Errorf("退回用户余额失败：%w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE bill_events
		SET status = 'refunded',
		    terminal_note = $1,
		    finished_at = $2
		WHERE event_id = $3
	`, reason, now, eventID); err != nil {
		return err
	}

	if err := appendOpTx(ctx, tx, eventID, map[string]any{
		"action":      "refunded",
		"operator_id": operatorID,
		"reason":      reason,
		"at":          now.UnixMilli(),
	}); err != nil {
		return fmt.Errorf("写 refunded op 失败: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.logger.Info("Refund completed", zap.String("event_id", eventID), zap.String("operator_id", operatorID))
	return nil
}

// appendOpTx 在退款事务内追加一条账务操作记录到 bill_events.metadata。
func appendOpTx(ctx context.Context, tx pgx.Tx, eventID string, op map[string]any) error {
	opJSON, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("序列化账务操作失败: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE bill_events
		SET metadata = jsonb_set(
			COALESCE(metadata, '{}'),
			'{ops}',
			COALESCE(metadata->'ops', '[]'::jsonb) || $1::jsonb,
			true
		)
		WHERE event_id = $2
	`, string(opJSON), eventID)
	return err
}

// ReverseResult 充值撤销结果
type ReverseResult struct {
	OrderID           string
	PaymentOrderID    string
	FulfillmentStatus string
	BalanceLotID      string
	OriginalCredits   int64
	ReversedCredits   int64
	LostCredits       int64
	IsPartial         bool
}

// ReverseOrder 撤销充值订单。
// 权限校验由 Handler 层完成（管理员撤 platform_to_tenant，租户用户撤 tenant_to_user）
func (s *DeductionService) ReverseOrder(orderID, reason, operatorID string) (*ReverseResult, error) {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status, orderType, paymentOrderID string
	var creditAmount int64
	err = tx.QueryRow(ctx, `
		SELECT status, order_type, COALESCE(payment_order_id, ''), credit_amount
		FROM bill_recharge_orders WHERE order_id = $1 FOR UPDATE
	`, orderID).Scan(&status, &orderType, &paymentOrderID, &creditAmount)
	if err != nil {
		return nil, shared.ErrRechargeNotFound
	}
	if status == billing.OrderStatusReversed {
		return nil, shared.ErrRechargeAlreadyReversed
	}
	if status != billing.OrderStatusActive {
		return nil, shared.ErrRechargeNotReversible
	}
	// 用户在线支付形成的租户收入不是目标用户的额度包，不跟随额度撤回。
	if orderType == billing.OrderTypeUserTopupIncome {
		return nil, shared.ErrRechargeNotReversible
	}
	// Online recharge is reversed only by the completed-refund workflow, which
	// also reverses tenant income and writes the cash-ledger correction.
	if paymentOrderID != "" {
		return nil, shared.ErrRechargeNotReversible
	}

	revocation, err := ledger.RevokeOrderLots(ctx, tx, orderID)
	if err != nil {
		return nil, fmt.Errorf("撤销额度包失败: %w", err)
	}
	// 充值额度已全部消耗（或额度包已过期离场），无可撤销余额
	if revocation.ReclaimedMicro == 0 {
		return nil, shared.ErrRechargeCreditsExhausted
	}
	lostMicro := creditAmount - revocation.ReclaimedMicro

	now := billing.NowUTC()
	if _, err := tx.Exec(ctx, `
		UPDATE bill_recharge_orders
		SET status = 'reversed', reversed_at = $1, reversed_by = $2, reversal_reason = $3,
		    reversed_amount_micro = $4, lost_amount_micro = $5
		WHERE order_id = $6
	`, now, operatorID, reason, revocation.ReclaimedMicro, lostMicro, orderID); err != nil {
		return nil, fmt.Errorf("更新充值订单状态失败: %w", err)
	}
	fulfillmentStatus := "reversed"
	if lostMicro > 0 {
		fulfillmentStatus = "partially_reversed"
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	lotID := ""
	if len(revocation.LotIDs) > 0 {
		lotID = revocation.LotIDs[0]
	}

	s.logger.Info("Recharge order reversed",
		zap.String("orderId", orderID),
		zap.String("operator_id", operatorID),
		zap.Int64("reversedCredits", revocation.ReclaimedMicro),
		zap.Int64("lostCredits", lostMicro),
		zap.Bool("isPartial", lostMicro > 0),
	)

	return &ReverseResult{
		OrderID:           orderID,
		PaymentOrderID:    paymentOrderID,
		FulfillmentStatus: fulfillmentStatus,
		BalanceLotID:      lotID,
		OriginalCredits:   creditAmount,
		ReversedCredits:   revocation.ReclaimedMicro,
		LostCredits:       lostMicro,
		IsPartial:         lostMicro > 0,
	}, nil
}

func (s *DeductionService) BatchRefund(eventIDs []string, reason, operatorID string) BatchOpResult {
	if len(eventIDs) > 100 {
		eventIDs = eventIDs[:100]
	}

	result := BatchOpResult{
		Succeeded: make([]string, 0),
		Failed:    make([]BatchOpError, 0),
	}

	for _, eventID := range eventIDs {
		// 读取已扣额用于汇总（Refund 内部会校验状态）
		var tenantCredits, userCredits *int64
		_ = s.pool.QueryRow(context.Background(), `
			SELECT tenant_credits, user_credits FROM bill_events WHERE event_id = $1
		`, eventID).Scan(&tenantCredits, &userCredits)

		if err := s.Refund(eventID, reason, operatorID); err != nil {
			result.Failed = append(result.Failed, BatchOpError{EventID: eventID, Reason: err.Error()})
			continue
		}

		result.Succeeded = append(result.Succeeded, eventID)
		if tenantCredits != nil {
			result.TotalTenantCredits += *tenantCredits
		}
		if userCredits != nil {
			result.TotalUserCredits += *userCredits
		}
	}

	return result
}

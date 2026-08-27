package service

import (
	"context"
	"fmt"
	"time"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/audit"
	"xiaodou/dai/internal/billing/ledger"
	shared "xiaodou/dai/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// BatchOpError 批量操作单条失败记录
type BatchOpError struct {
	RequestID string `json:"requestId"`
	Reason    string `json:"reason"`
}

// BatchOpResult 批量操作结果
type BatchOpResult struct {
	Succeeded          []string       `json:"succeeded"`
	Failed             []BatchOpError `json:"failed"`
	TotalTenantCredits int64          `json:"totalTenantCredits"`
	TotalUserCredits   int64          `json:"totalUserCredits"`
}

// DeductionService 运营资金操作：AI 使用退款与充值撤销。
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

// RefundUsage 全额退回一条已结算的 AI 使用记录（仅平台管理员可操作）。
//
// 退款就是把钱加回账户。账户余额是有符号的，所以「欠费的账户退款只清欠、不退现」
// 这条规则不需要任何代码来实现 —— 加法本身就是这个语义。
func (s *DeductionService) RefundUsage(ctx context.Context, requestID, reason, operatorID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var tenantID, userID string
	var tenantCredits, userCredits int64
	var billingStatus, refundStatus string
	err = tx.QueryRow(ctx, `
		SELECT tenant_id, COALESCE(user_id, ''), tenant_payable, user_charged,
		       billing_status, refund_status
		FROM ai_usage_logs
		WHERE request_id = $1
		FOR UPDATE
	`, requestID).Scan(&tenantID, &userID, &tenantCredits, &userCredits, &billingStatus, &refundStatus)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return shared.ErrUsageNotFound
	}
	if billingStatus != "settled" {
		return fmt.Errorf("usage not refundable (billing_status=%s)", billingStatus)
	}
	if refundStatus != "none" {
		return fmt.Errorf("usage already refunded")
	}
	if tenantCredits <= 0 && userCredits <= 0 {
		return fmt.Errorf("usage has no refundable charge")
	}
	beforeState, err := audit.Snapshot(map[string]any{
		"request_id": requestID, "tenant_id": tenantID, "user_id": userID,
		"billing_status": billingStatus, "refund_status": refundStatus,
		"tenant_payable": tenantCredits, "user_charged": userCredits,
	})
	if err != nil {
		return err
	}

	now := billing.NowUTC()

	if tenantCredits > 0 {
		if _, err := ledger.Grant(ctx, tx,
			ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID},
			tenantCredits, nil, billing.PackageSourceRefund, ""); err != nil {
			return fmt.Errorf("退回租户余额失败：%w", err)
		}
	}
	if userCredits > 0 && userID != "" {
		if _, err := ledger.Grant(ctx, tx,
			ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID},
			userCredits, nil, billing.PackageSourceRefund, ""); err != nil {
			return fmt.Errorf("退回用户余额失败：%w", err)
		}
	}
	if userCredits > 0 && userID == "" {
		return fmt.Errorf("user charge has no user id")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE ai_usage_logs
		SET refund_status = 'refunded',
		    refund_reason = NULLIF($1, ''),
		    refund_operator_id = NULLIF($2, ''),
		    refunded_at = $3
		WHERE request_id = $4
	`, reason, operatorID, now, requestID); err != nil {
		return err
	}
	afterState, err := audit.Snapshot(map[string]any{
		"request_id": requestID, "tenant_id": tenantID, "user_id": userID,
		"billing_status": billingStatus, "refund_status": "refunded",
		"tenant_payable": tenantCredits, "user_charged": userCredits,
		"refund_reason": reason, "refund_operator_id": operatorID,
	})
	if err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Event{
		RepairID: audit.NewRepairID(), Action: "usage_refund",
		IdempotencyKey: "usage-refund:" + requestID,
		TargetType:     "ai_usage_logs", TargetID: requestID,
		OperatorID: operatorID, Reason: reason,
		BeforeState: beforeState, AfterState: afterState,
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.logger.Info("Usage refund completed", zap.String("request_id", requestID), zap.String("operator_id", operatorID))
	return nil
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
//
// This is the unrestricted port used by platform operators and the completed
// payment-refund workflow. Tenant-scoped callers must use ReverseTenantOrder
// so the scope check runs under the same row lock as the state transition.
func (s *DeductionService) ReverseOrder(ctx context.Context, orderID, reason, operatorID string) (*ReverseResult, error) {
	return s.reverseOrder(ctx, orderID, "", reason, operatorID)
}

// ReverseTenantOrder 撤销本租户的一次性用户充值。
//
// tenantID is deliberately part of the service command rather than a
// preflight query in HTTP. The order row is selected FOR UPDATE before its
// tenant and order type are checked, so an order cannot change scope between
// authorization and reversal.
func (s *DeductionService) ReverseTenantOrder(ctx context.Context, orderID, tenantID, reason, operatorID string) (*ReverseResult, error) {
	if tenantID == "" {
		return nil, shared.ErrForbidden
	}
	return s.reverseOrder(ctx, orderID, tenantID, reason, operatorID)
}

func (s *DeductionService) reverseOrder(ctx context.Context, orderID, tenantID, reason, operatorID string) (*ReverseResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var orderTenantID, status, orderType, paymentOrderID string
	var creditAmount, reversedAmount, lostAmount int64
	var reversedAt *time.Time
	var reversedBy, reversalReason string
	err = tx.QueryRow(ctx, `
		SELECT tenant_id, status, order_type, COALESCE(payment_order_id, ''), credit_amount,
		       reversed_amount_micro, lost_amount_micro, reversed_at,
		       COALESCE(reversed_by, ''), COALESCE(reversal_reason, '')
		FROM bill_recharge_orders WHERE order_id = $1 FOR UPDATE
	`, orderID).Scan(&orderTenantID, &status, &orderType, &paymentOrderID, &creditAmount,
		&reversedAmount, &lostAmount, &reversedAt, &reversedBy, &reversalReason)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, shared.ErrRechargeNotFound
	}
	if tenantID != "" && (orderTenantID != tenantID || orderType != billing.OrderTypeTenantToUser) {
		return nil, shared.ErrForbidden
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
	beforeState, err := audit.Snapshot(map[string]any{
		"order_id": orderID, "tenant_id": orderTenantID, "order_type": orderType,
		"payment_order_id": paymentOrderID, "status": status,
		"credit_amount": creditAmount, "reversed_amount_micro": reversedAmount,
		"lost_amount_micro": lostAmount, "reversed_at": reversedAt,
		"reversed_by": reversedBy, "reversal_reason": reversalReason,
	})
	if err != nil {
		return nil, err
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
	afterState, err := audit.Snapshot(map[string]any{
		"order_id": orderID, "tenant_id": orderTenantID, "order_type": orderType,
		"payment_order_id": paymentOrderID, "status": billing.OrderStatusReversed,
		"credit_amount": creditAmount, "reversed_amount_micro": revocation.ReclaimedMicro,
		"lost_amount_micro": lostMicro, "fulfillment_status": fulfillmentStatus,
		"reversed_by": operatorID, "reversal_reason": reason,
	})
	if err != nil {
		return nil, err
	}
	if err := audit.Append(ctx, tx, audit.Event{
		RepairID: audit.NewRepairID(), Action: "recharge_reversal",
		IdempotencyKey: "recharge-reversal:" + orderID,
		TargetType:     "bill_recharge_orders", TargetID: orderID,
		OperatorID: operatorID, Reason: reason,
		BeforeState: beforeState, AfterState: afterState,
	}); err != nil {
		return nil, err
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

func (s *DeductionService) BatchRefundUsage(ctx context.Context, requestIDs []string, reason, operatorID string) BatchOpResult {
	if len(requestIDs) > 100 {
		requestIDs = requestIDs[:100]
	}

	result := BatchOpResult{
		Succeeded: make([]string, 0),
		Failed:    make([]BatchOpError, 0),
	}

	for i, requestID := range requestIDs {
		if err := ctx.Err(); err != nil {
			for _, pendingID := range requestIDs[i:] {
				result.Failed = append(result.Failed, BatchOpError{RequestID: pendingID, Reason: err.Error()})
			}
			break
		}
		// 读取原始金额用于汇总（RefundUsage 内部会校验状态）。
		var tenantCredits, userCredits int64
		if err := s.pool.QueryRow(ctx, `
			SELECT tenant_payable, user_charged FROM ai_usage_logs WHERE request_id = $1
		`, requestID).Scan(&tenantCredits, &userCredits); err != nil && ctx.Err() != nil {
			for _, pendingID := range requestIDs[i:] {
				result.Failed = append(result.Failed, BatchOpError{RequestID: pendingID, Reason: ctx.Err().Error()})
			}
			break
		}

		if err := s.RefundUsage(ctx, requestID, reason, operatorID); err != nil {
			result.Failed = append(result.Failed, BatchOpError{RequestID: requestID, Reason: err.Error()})
			if ctx.Err() != nil {
				for _, pendingID := range requestIDs[i+1:] {
					result.Failed = append(result.Failed, BatchOpError{RequestID: pendingID, Reason: ctx.Err().Error()})
				}
				break
			}
			continue
		}

		result.Succeeded = append(result.Succeeded, requestID)
		result.TotalTenantCredits += tenantCredits
		result.TotalUserCredits += userCredits
	}

	return result
}

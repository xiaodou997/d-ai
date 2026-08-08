package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/pg"
	shared "xiaodou/dai/internal/domain"

	"github.com/google/uuid"
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

const (
	accountStateOK        = "OK"
	accountStateOverdraft = "OVERDRAFT"
	accountStateExhausted = "EXHAUSTED"
)

type settlementMetadata struct {
	Mode               string `json:"mode,omitempty"`
	TenantDeducted     int64  `json:"tenantDeducted,omitempty"`
	UserDeducted       int64  `json:"userDeducted,omitempty"`
	TenantOverdraftAdd int64  `json:"tenantOverdraftAdd,omitempty"`
	UserOverdraftAdd   int64  `json:"userOverdraftAdd,omitempty"`
	AccountState       string `json:"accountState,omitempty"`
	AllowFurtherUse    bool   `json:"allowFurtherUsage"`
}

// DeductionService 计费服务
type DeductionService struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewDeductionService 创建进程内直接扣费服务。
func NewDeductionService(pool *pgxpool.Pool, logger *zap.Logger) *DeductionService {
	s := &DeductionService{
		pool:   pool,
		logger: logger,
	}
	return s
}

// ConsumeParams 单阶段聚合扣款请求。
//
// 适用场景：AI 业务域已在本地完成精细计费（如分账层聚合多次
// 微小消费），仅在达到阈值时通过进程内计费服务把 micro-USD 金额一次性扣掉。失败时不需要
// 反向补偿——单 SQL 事务，原子。
//
//   - 直接按额度包 FIFO 扣减，不创建冻结或预扣状态
//   - 允许透支时，额度不足的尾差直接转入 current_overdraft，扣费仍然成功
//   - 严格扣费调用方可以拒绝已有透支；AI 完成态扣费明确允许透支
type ConsumeParams struct {
	IdempotencyKey string
	ClientID       string
	TenantID       string
	UserID         string
	Description    string
	TenantAmount   int64
	UserAmount     int64
	// DisallowOverdraft 为 true 时，余额不足不转透支，整单回滚返回 ErrInsufficientBalance。
	// 适用于需要严格余额校验的管理或后台操作。
	DisallowOverdraft bool
	// AllowOverdraft 必须显式声明。只有已经完成工作的计费尾差可以启用。
	AllowOverdraft bool
}

// ConsumeResult 单阶段扣款结果
type ConsumeResult struct {
	EventID            string
	TenantDeducted     int64 // 从租户余额实际扣减的部分
	UserDeducted       int64 // 从用户余额实际扣减的部分
	TenantOverdraftAdd int64 // 计入租户欠费的部分
	UserOverdraftAdd   int64 // 计入用户欠费的部分
	AccountState       string
	AllowFurtherUsage  bool
}

// Consume 单阶段幂等扣款。金额扣减、扣款事件和调用方事务以外的场景使用此入口。
func (s *DeductionService) Consume(params ConsumeParams) (*ConsumeResult, error) {
	if err := validateConsumeParams(params); err != nil {
		return nil, err
	}

	txCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(txCtx)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败：%w", err)
	}
	defer tx.Rollback(txCtx)

	result, err := s.ConsumeTx(txCtx, tx, params)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(txCtx); err != nil {
		return nil, fmt.Errorf("提交事务失败：%w", err)
	}
	return result, nil
}

// ConsumeTx 在调用方事务内完成一次直接扣费。
//
// AI 使用记录应与这里产生的 bill_events 同事务提交：request_id 负责业务幂等，
// idempotency_key 负责账务幂等；没有冻结、预扣或异步扣款阶段。允许透支时，
// 余额不足的尾差直接进入 current_overdraft。
func (s *DeductionService) ConsumeTx(ctx context.Context, tx pgx.Tx, params ConsumeParams) (*ConsumeResult, error) {
	if err := validateConsumeParams(params); err != nil {
		return nil, err
	}

	var tenantStatus string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(status, 'active') FROM iam_tenants WHERE tenant_id = $1 FOR UPDATE
	`, params.TenantID).Scan(&tenantStatus); err != nil {
		return nil, fmt.Errorf("tenant %q not found", params.TenantID)
	}
	if tenantStatus != "active" {
		return nil, shared.ErrTenantSuspended
	}

	if params.UserID != "" {
		var accountID string
		if err := tx.QueryRow(ctx, `
			SELECT user_id FROM iam_accounts
			WHERE user_id = $1 AND tenant_id = $2 AND user_type = 4 AND status = 'active'
			LIMIT 1 FOR UPDATE
		`, params.UserID, params.TenantID).Scan(&accountID); err != nil {
			return nil, fmt.Errorf("user %q does not belong to tenant %q", params.UserID, params.TenantID)
		}
	}

	// The lookup is inside the same transaction as the debit. This also makes
	// recovery safe when the caller retries after an unknown commit result.
	var existingEventID, existingTenantID, existingUserID, existingClientID, existingStatus, existingMetadata string
	var existingTenant, existingUser *int64
	err := tx.QueryRow(ctx, `
		SELECT event_id, tenant_id, COALESCE(user_id, ''), COALESCE(client_id, ''),
		       tenant_credits, user_credits, status, COALESCE(metadata::text, '{}')
		FROM bill_events WHERE idempotency_key = $1 FOR UPDATE
	`, params.IdempotencyKey).Scan(&existingEventID, &existingTenantID, &existingUserID, &existingClientID,
		&existingTenant, &existingUser, &existingStatus, &existingMetadata)
	if err == nil {
		existingTenantAmount, existingUserAmount := int64(0), int64(0)
		if existingTenant != nil {
			existingTenantAmount = *existingTenant
		}
		if existingUser != nil {
			existingUserAmount = *existingUser
		}
		if existingStatus != billing.EventStatusSucceeded || existingClientID != params.ClientID ||
			existingTenantID != params.TenantID || existingUserID != params.UserID ||
			existingTenantAmount != params.TenantAmount || existingUserAmount != params.UserAmount {
			return nil, fmt.Errorf("idempotency key already belongs to a different debit request")
		}
		return &ConsumeResult{
			EventID:            existingEventID,
			TenantDeducted:     metadataTenantDeducted(existingMetadata, existingTenantAmount),
			UserDeducted:       metadataUserDeducted(existingMetadata, existingUserAmount),
			TenantOverdraftAdd: metadataTenantOverdraft(existingMetadata),
			UserOverdraftAdd:   metadataUserOverdraft(existingMetadata),
			AccountState:       metadataAccountState(existingMetadata),
			AllowFurtherUsage:  metadataAllowFurtherUsage(existingMetadata),
		}, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("查询扣款幂等记录失败：%w", err)
	}

	now := billing.NowUTC()
	result := &ConsumeResult{}
	allowOverdraft := consumeAllowsOverdraft(params)

	if params.TenantAmount > 0 {
		_, currentOverdraft, err := pg.GetTenantOverdraft(ctx, tx, params.TenantID)
		if err != nil {
			return nil, err
		}
		if !allowOverdraft && currentOverdraft > 0 {
			return nil, shared.ErrTenantInOverdraft
		}
		shortfall, err := pg.DeductFIFOPartial(ctx, tx, billing.PackageTypeTenant, params.TenantID, "", params.TenantAmount, now)
		if err != nil {
			return nil, fmt.Errorf("租户 FIFO 扣减失败：%w", err)
		}
		result.TenantDeducted = params.TenantAmount - shortfall
		if shortfall > 0 {
			if !allowOverdraft {
				return nil, shared.ErrInsufficientBalance
			}
			if err := pg.IncreaseTenantOverdraft(ctx, tx, params.TenantID, shortfall); err != nil {
				return nil, err
			}
			result.TenantOverdraftAdd = shortfall
		}
	}

	if params.UserAmount > 0 {
		_, currentOverdraft, err := pg.GetUserOverdraft(ctx, tx, params.UserID)
		if err != nil {
			return nil, err
		}
		if !allowOverdraft && currentOverdraft > 0 {
			return nil, shared.ErrUserInOverdraft
		}
		shortfall, err := pg.DeductFIFOPartial(ctx, tx, billing.PackageTypeUser, params.TenantID, params.UserID, params.UserAmount, now)
		if err != nil {
			return nil, fmt.Errorf("用户 FIFO 扣减失败：%w", err)
		}
		result.UserDeducted = params.UserAmount - shortfall
		if shortfall > 0 {
			if !allowOverdraft {
				return nil, shared.ErrInsufficientBalance
			}
			if err := pg.IncreaseUserOverdraft(ctx, tx, params.UserID, shortfall); err != nil {
				return nil, err
			}
			result.UserOverdraftAdd = shortfall
		}
	}

	result.EventID = "EV_" + uuid.New().String()[:24]
	result.AccountState, result.AllowFurtherUsage, err = directEventState(ctx, tx, params.TenantID, params.UserID,
		params.TenantAmount > 0, params.UserAmount > 0)
	if err != nil {
		return nil, err
	}

	var tenantCreditsVal, userCreditsVal any
	if params.TenantAmount > 0 {
		tenantCreditsVal = params.TenantAmount
	}
	if params.UserAmount > 0 {
		userCreditsVal = params.UserAmount
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_events
		(event_id, idempotency_key, tenant_id, user_id, description, client_id,
		 event_type, tenant_credits, user_credits, status, metadata, created_at, finished_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''),
		        'charge', $7, $8, 'succeeded', $9::jsonb, $10, $10)
	`, result.EventID, params.IdempotencyKey, params.TenantID, params.UserID,
		params.Description, params.ClientID, tenantCreditsVal, userCreditsVal,
		encodeSettlementMetadata(settlementMetadata{
			Mode:               modeString(allowOverdraft),
			TenantDeducted:     result.TenantDeducted,
			UserDeducted:       result.UserDeducted,
			TenantOverdraftAdd: result.TenantOverdraftAdd,
			UserOverdraftAdd:   result.UserOverdraftAdd,
			AccountState:       result.AccountState,
			AllowFurtherUse:    result.AllowFurtherUsage,
		}), now); err != nil {
		return nil, fmt.Errorf("记录扣款事件失败：%w", err)
	}
	return result, nil
}

func validateConsumeParams(params ConsumeParams) error {
	if params.IdempotencyKey == "" {
		return fmt.Errorf("idempotencyKey is required")
	}
	if params.TenantID == "" {
		return fmt.Errorf("tenantId is required")
	}
	if params.TenantAmount < 0 || params.UserAmount < 0 {
		return shared.ErrInvalidAmount
	}
	if params.TenantAmount == 0 && params.UserAmount == 0 {
		return fmt.Errorf("at least one of tenantAmount or userAmount must be > 0")
	}
	if params.UserAmount > 0 && params.UserID == "" {
		return fmt.Errorf("userId is required when userAmount > 0")
	}
	return nil
}

// directEventState only evaluates the live quota lots and overdraft state. New
// direct charges never create reservations, and historical holds must not
// change the meaning of a new usage record. Overdraft is intentionally
// considered usable by this path.
func directEventState(ctx context.Context, tx pgx.Tx, tenantID, userID string, includeTenant, includeUser bool) (string, bool, error) {
	state := accountStateOK
	for _, account := range []struct {
		packageType string
		id          string
		include     bool
	}{
		{billing.PackageTypeTenant, tenantID, includeTenant},
		{billing.PackageTypeUser, userID, includeUser},
	} {
		if !account.include {
			continue
		}
		var debt int64
		if account.packageType == billing.PackageTypeTenant {
			if err := tx.QueryRow(ctx, `SELECT COALESCE(current_overdraft, 0) FROM iam_tenants WHERE tenant_id=$1`, account.id).Scan(&debt); err != nil {
				return "", false, err
			}
		} else if err := tx.QueryRow(ctx, `SELECT COALESCE(current_overdraft, 0) FROM iam_accounts WHERE user_id=$1 AND user_type=4`, account.id).Scan(&debt); err != nil {
			return "", false, err
		}
		var remaining int64
		if account.packageType == billing.PackageTypeTenant {
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE(SUM(remaining_credits), 0) FROM bill_credit_packages
				WHERE package_type='tenant' AND tenant_id=$1 AND status='available'
				  AND (expires_at IS NULL OR expires_at > $2)
			`, account.id, billing.NowUTC()).Scan(&remaining); err != nil {
				return "", false, err
			}
		} else if err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(remaining_credits), 0) FROM bill_credit_packages
			WHERE package_type='user' AND user_id=$1 AND status='available'
			  AND (expires_at IS NULL OR expires_at > $2)
		`, account.id, billing.NowUTC()).Scan(&remaining); err != nil {
			return "", false, err
		}
		if debt > 0 {
			state = accountStateOverdraft
		} else if remaining <= 0 && state == accountStateOK {
			state = accountStateExhausted
		}
	}
	return state, true, nil
}

// Refund 全额退款（仅平台管理员可操作）
// 退款策略：新建一个永久额度包（source=REFUND），原消费事件标记为 refunded
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

	// 退款清欠策略：先抵扣 current_overdraft（清欠），剩余金额才进入新的退款额度包。
	// 过欠账户会被完整抵扣完债务后才退现金，体现"退款仅清欠（过欠不退现）"原则。
	if tenantCredits != nil && *tenantCredits > 0 {
		cleared, err := pg.DecreaseTenantOverdraft(ctx, tx, tenantID, *tenantCredits)
		if err != nil {
			return fmt.Errorf("抵扣租户透支失败：%w", err)
		}
		leftover := *tenantCredits - cleared
		if leftover > 0 {
			packageID := "PKG_" + uuid.New().String()[:24]
			if err := pg.CreateRefundPackage(ctx, tx, packageID, billing.PackageTypeTenant, tenantID, "", leftover, now); err != nil {
				return fmt.Errorf("退回租户余额失败：%w", err)
			}
		}
	}
	if userCredits != nil && *userCredits > 0 && userID != "" {
		cleared, err := pg.DecreaseUserOverdraft(ctx, tx, userID, *userCredits)
		if err != nil {
			return fmt.Errorf("抵扣用户透支失败：%w", err)
		}
		leftover := *userCredits - cleared
		if leftover > 0 {
			packageID := "PKG_" + uuid.New().String()[:24]
			if err := pg.CreateRefundPackage(ctx, tx, packageID, billing.PackageTypeUser, tenantID, userID, leftover, now); err != nil {
				return fmt.Errorf("退回用户余额失败：%w", err)
			}
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE bill_events
		SET status = 'refunded',
		    terminal_note = $1,
		    finished_at = $2
		WHERE event_id = $3
	`, reason, now, eventID)
	if err != nil {
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
	OrderID         string
	PackageID       string
	OriginalCredits int64
	ReversedCredits int64
	LostCredits     int64
	PackageStatus   string
	IsPartial       bool
}

// ReverseOrder 撤销充值订单
// 权限校验由 Handler 层完成（管理员撤 platform_to_tenant，租户用户撤 tenant_to_user）
func (s *DeductionService) ReverseOrder(orderID, reason, operatorID string) (*ReverseResult, error) {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. 查找并锁定充值订单
	var status, orderType string
	err = tx.QueryRow(ctx, `
		SELECT status, order_type FROM bill_recharge_orders WHERE order_id = $1 FOR UPDATE
	`, orderID).Scan(&status, &orderType)
	if err != nil {
		return nil, shared.ErrRechargeNotFound
	}
	if status == billing.OrderStatusReversed {
		return nil, shared.ErrRechargeAlreadyReversed
	}
	if status != billing.OrderStatusActive {
		return nil, shared.ErrRechargeNotReversible
	}
	// 在线充值（微信支付）不可撤销，权限校验之外再兜底一道
	if slices.Contains(billing.OnlineOrderTypes, orderType) {
		return nil, shared.ErrRechargeNotReversible
	}

	// 2. 查找关联额度包并加锁
	var packageID, pkgStatus string
	var totalCredits, remainingCredits int64
	err = tx.QueryRow(ctx, `
		SELECT package_id, status, total_credits, remaining_credits
		FROM bill_credit_packages WHERE recharge_order_id = $1 FOR UPDATE
	`, orderID).Scan(&packageID, &pkgStatus, &totalCredits, &remainingCredits)
	if err != nil {
		return nil, fmt.Errorf("未找到关联额度包: %w", err)
	}

	// 3. 充值余额已全部消耗，拒绝撤销
	if remainingCredits == 0 {
		return nil, shared.ErrRechargeCreditsExhausted
	}

	// 4. 计算撤销数据
	lostCredits := totalCredits - remainingCredits
	isPartial := lostCredits > 0
	fullRevoke := !isPartial

	// 5. 撤销额度包
	if err := pg.RevokeCreditPackage(ctx, tx, packageID, fullRevoke); err != nil {
		return nil, fmt.Errorf("撤销额度包失败: %w", err)
	}

	// 6. 更新充值订单状态
	now := billing.NowUTC()
	_, err = tx.Exec(ctx, `
		UPDATE bill_recharge_orders
		SET status = 'reversed', reversed_at = $1, reversed_by = $2, reversal_reason = $3
		WHERE order_id = $4
	`, now, operatorID, reason, orderID)
	if err != nil {
		return nil, fmt.Errorf("更新充值订单状态失败: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	pkgFinalStatus := billing.PackageStatusDepleted
	if fullRevoke {
		pkgFinalStatus = billing.PackageStatusRevoked
	}

	s.logger.Info("Recharge order reversed",
		zap.String("orderId", orderID),
		zap.String("operator_id", operatorID),
		zap.Int64("reversedCredits", remainingCredits),
		zap.Int64("lostCredits", lostCredits),
		zap.Bool("isPartial", isPartial),
	)

	return &ReverseResult{
		OrderID:         orderID,
		PackageID:       packageID,
		OriginalCredits: totalCredits,
		ReversedCredits: remainingCredits,
		LostCredits:     lostCredits,
		PackageStatus:   pkgFinalStatus,
		IsPartial:       isPartial,
	}, nil
}

func modeString(allowOverdraft bool) string {
	if allowOverdraft {
		return "flex"
	}
	return "strict"
}

func consumeAllowsOverdraft(params ConsumeParams) bool {
	if params.DisallowOverdraft {
		return false
	}
	return params.AllowOverdraft
}

func encodeSettlementMetadata(meta settlementMetadata) string {
	b, err := json.Marshal(meta)
	if err != nil {
		return `{}`
	}
	return string(b)
}

func decodeSettlementMetadata(raw string) settlementMetadata {
	if raw == "" {
		return settlementMetadata{}
	}
	var meta settlementMetadata
	_ = json.Unmarshal([]byte(raw), &meta)
	return meta
}

func metadataAccountState(raw string) string {
	meta := decodeSettlementMetadata(raw)
	if meta.AccountState == "" {
		return accountStateOK
	}
	return meta.AccountState
}

func metadataAllowFurtherUsage(raw string) bool {
	meta := decodeSettlementMetadata(raw)
	if meta.AccountState == "" && !meta.AllowFurtherUse {
		return true
	}
	return meta.AllowFurtherUse
}

func metadataTenantDeducted(raw string, fallback int64) int64 {
	meta := decodeSettlementMetadata(raw)
	if meta.TenantDeducted == 0 && fallback > 0 {
		return fallback
	}
	return meta.TenantDeducted
}

func metadataUserDeducted(raw string, fallback int64) int64 {
	meta := decodeSettlementMetadata(raw)
	if meta.UserDeducted == 0 && fallback > 0 {
		return fallback
	}
	return meta.UserDeducted
}

func metadataTenantOverdraft(raw string) int64 {
	return decodeSettlementMetadata(raw).TenantOverdraftAdd
}

func metadataUserOverdraft(raw string) int64 {
	return decodeSettlementMetadata(raw).UserOverdraftAdd
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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/pg"
	"xiaodou/dai/internal/cache"
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
	FrozenTenant       int64  `json:"frozenTenant,omitempty"`
	FrozenUser         int64  `json:"frozenUser,omitempty"`
	TenantDeducted     int64  `json:"tenantDeducted,omitempty"`
	UserDeducted       int64  `json:"userDeducted,omitempty"`
	TenantOverdraftAdd int64  `json:"tenantOverdraftAdd,omitempty"`
	UserOverdraftAdd   int64  `json:"userOverdraftAdd,omitempty"`
	AccountState       string `json:"accountState,omitempty"`
	AllowFurtherUse    bool   `json:"allowFurtherUsage"`
}

// DeductionService 计费服务
type DeductionService struct {
	pool      *pgxpool.Pool
	redis     *cache.RedisService
	eventRepo billing.EventRepository
	logger    *zap.Logger
}

// WithEventRepo 设置事件仓储
func WithEventRepo(repo billing.EventRepository) func(*DeductionService) {
	return func(s *DeductionService) {
		s.eventRepo = repo
	}
}

// NewDeductionService 创建计费服务（依赖注入）
func NewDeductionService(pool *pgxpool.Pool, redis *cache.RedisService, logger *zap.Logger, opts ...func(*DeductionService)) *DeductionService {
	s := &DeductionService{
		pool:   pool,
		redis:  redis,
		logger: logger,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.eventRepo == nil {
		s.eventRepo = pg.NewEventRepository(pool)
	}
	return s
}

// FreezeParams 双账户预授权冻结请求
type FreezeParams struct {
	IdempotencyKey string
	ClientID       string
	TenantID       string
	UserID         string
	Description    string
	TenantAmount   int64
	UserAmount     int64
	AllowOverdraft bool
}

// FreezeResult 预授权冻结结果
type FreezeResult struct {
	EventID           string
	FrozenTenant      int64
	FrozenUser        int64
	AccountState      string
	AllowFurtherUsage bool
	Status            string
}

// ConfirmParams 确认预授权请求。EventID / IdempotencyKey 至少需要一个。
type ConfirmParams struct {
	EventID            string
	IdempotencyKey     string
	ClientID           string
	ActualTenantAmount int64
	ActualUserAmount   int64
	AllowOverdraft     bool
	// AllowReleased is reserved for the controlled V2-to-V3 cutover. A
	// released authorization has no escrow left, but its completed usage must
	// still be chargeable exactly once.
	AllowReleased bool
}

// ConfirmResult 预授权确认结果。
type ConfirmResult struct {
	EventID            string
	TenantDeducted     int64
	UserDeducted       int64
	TenantOverdraftAdd int64
	UserOverdraftAdd   int64
	AccountState       string
	AllowFurtherUsage  bool
	Status             string
}

// CancelParams 取消预授权请求。EventID / IdempotencyKey 至少需要一个。
type CancelParams struct {
	EventID        string
	IdempotencyKey string
	ClientID       string
}

// CancelResult 取消预授权结果。
type CancelResult struct {
	EventID           string
	AccountState      string
	AllowFurtherUsage bool
	Status            string
}

// Freeze 双账户预授权冻结
func (s *DeductionService) Freeze(params FreezeParams) (*FreezeResult, error) {
	if params.TenantID == "" {
		return nil, fmt.Errorf("tenantId is required")
	}
	if params.TenantAmount < 0 || params.UserAmount < 0 {
		return nil, shared.ErrInvalidAmount
	}
	if params.TenantAmount == 0 && params.UserAmount == 0 {
		return nil, fmt.Errorf("at least one of tenantAmount or userAmount must be > 0")
	}
	if params.UserAmount > 0 && params.UserID == "" {
		return nil, fmt.Errorf("userId is required when userAmount > 0")
	}

	ctx := context.Background()

	// 校验租户状态
	var tenantStatus string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(status, 'active') FROM iam_tenants WHERE tenant_id = $1`, params.TenantID).Scan(&tenantStatus); err != nil {
		return nil, fmt.Errorf("tenant %q not found", params.TenantID)
	}
	if tenantStatus != "active" {
		return nil, shared.ErrTenantSuspended
	}

	if params.UserID != "" {
		var count int
		err := s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM iam_users WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
		`, params.UserID, params.TenantID).Scan(&count)
		if err != nil || count == 0 {
			return nil, fmt.Errorf("user %q does not belong to tenant %q", params.UserID, params.TenantID)
		}
	}

	// 幂等检查
	if params.IdempotencyKey != "" {
		existing, err := s.eventRepo.FindByIdempotencyKey(params.IdempotencyKey)
		if err == nil && existing != nil {
			frozenTenant, frozenUser := int64(0), int64(0)
			if existing.TenantCredits != nil {
				frozenTenant = *existing.TenantCredits
			}
			if existing.UserCredits != nil {
				frozenUser = *existing.UserCredits
			}
			meta := decodeSettlementMetadata(existing.Metadata)
			if meta.FrozenTenant > 0 {
				frozenTenant = meta.FrozenTenant
			}
			if meta.FrozenUser > 0 {
				frozenUser = meta.FrozenUser
			}
			if existing.ClientID != params.ClientID || existing.TenantID != params.TenantID ||
				existing.UserID != params.UserID || frozenTenant != params.TenantAmount || frozenUser != params.UserAmount {
				return nil, fmt.Errorf("idempotency key already belongs to a different authorization request")
			}
			return &FreezeResult{
				EventID:           existing.EventID,
				FrozenTenant:      frozenTenant,
				FrozenUser:        frozenUser,
				AccountState:      metadataAccountState(existing.Metadata),
				AllowFurtherUsage: metadataAllowFurtherUsage(existing.Metadata),
				Status:            "SUCCESS",
			}, nil
		}
	}

	txCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := s.pool.Begin(txCtx)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败：%w", err)
	}
	defer tx.Rollback(txCtx)

	now := billing.NowUTC()
	var tenantState, userState string
	grantedTenant := params.TenantAmount
	grantedUser := params.UserAmount

	if params.TenantAmount > 0 {
		available, _, err := pg.GetTenantAvailableBalance(txCtx, tx, params.TenantID, now)
		if err != nil {
			return nil, fmt.Errorf("查询租户余额失败：%w", err)
		}
		limit, current, err := pg.GetTenantOverdraft(txCtx, tx, params.TenantID)
		if err != nil {
			return nil, err
		}
		tenantState = classifyAccountState(available, limit, current)
		if params.AllowOverdraft {
			if current > 0 {
				return nil, shared.ErrTenantInOverdraft
			}
			if available < grantedTenant {
				grantedTenant = available
			}
			if grantedTenant <= 0 {
				return nil, shared.ErrTenantInsufficientBalance
			}
		} else if available < params.TenantAmount {
			return nil, shared.ErrTenantInsufficientBalance
		}
		if err := pg.AddTenantFrozen(txCtx, tx, params.TenantID, grantedTenant); err != nil {
			return nil, fmt.Errorf("冻结租户积分失败：%w", err)
		}
	}

	if params.UserAmount > 0 {
		available, _, err := pg.GetUserAvailableBalance(txCtx, tx, params.UserID, now)
		if err != nil {
			return nil, fmt.Errorf("查询用户余额失败：%w", err)
		}
		limit, current, err := pg.GetUserOverdraft(txCtx, tx, params.UserID)
		if err != nil {
			return nil, err
		}
		userState = classifyAccountState(available, limit, current)
		if params.AllowOverdraft {
			if current > 0 {
				return nil, shared.ErrUserInOverdraft
			}
			if available < grantedUser {
				grantedUser = available
			}
			if grantedUser <= 0 {
				return nil, shared.ErrUserInsufficientBalance
			}
		} else if available < params.UserAmount {
			return nil, shared.ErrUserInsufficientBalance
		}
		if err := pg.AddUserFrozen(txCtx, tx, params.UserID, grantedUser); err != nil {
			return nil, fmt.Errorf("冻结用户积分失败：%w", err)
		}
	}

	eventID := "EV_" + uuid.New().String()[:24]

	var tenantCreditsVal, userCreditsVal any
	if params.TenantAmount > 0 {
		tenantCreditsVal = grantedTenant
	}
	if params.UserAmount > 0 {
		userCreditsVal = grantedUser
	}

	_, err = tx.Exec(txCtx, `
		INSERT INTO bill_events
		(event_id, idempotency_key, tenant_id, user_id, description, client_id,
		 tenant_credits, user_credits, status, metadata, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), $7, $8, 'pending', $9::jsonb, $10)
	`, eventID, nullableStr(params.IdempotencyKey), params.TenantID, params.UserID,
		params.Description, params.ClientID,
		tenantCreditsVal, userCreditsVal, encodeSettlementMetadata(settlementMetadata{
			Mode:            modeString(params.AllowOverdraft),
			FrozenTenant:    grantedTenant,
			FrozenUser:      grantedUser,
			AccountState:    mergeAccountStates(tenantState, userState),
			AllowFurtherUse: mergeAccountStates(tenantState, userState) != accountStateExhausted,
		}), now)
	if err != nil {
		return nil, fmt.Errorf("记录预授权事件失败：%w", err)
	}

	accountState, allowFurtherUsage, err := currentEventState(txCtx, tx, params.TenantID, params.UserID, grantedTenant > 0, grantedUser > 0)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(txCtx); err != nil {
		return nil, fmt.Errorf("提交事务失败：%w", err)
	}

	s.logger.Info("预授权冻结成功",
		zap.String("event_id", eventID),
		zap.String("tenant_id", params.TenantID),
		zap.String("user_id", params.UserID),
		zap.Int64("frozen_tenant", grantedTenant),
		zap.Int64("frozen_user", grantedUser),
	)

	return &FreezeResult{
		EventID:           eventID,
		FrozenTenant:      grantedTenant,
		FrozenUser:        grantedUser,
		AccountState:      accountState,
		AllowFurtherUsage: allowFurtherUsage,
		Status:            "SUCCESS",
	}, nil
}

// Confirm 确认预授权（FIFO 扣减实际量，解冻原冻结额）。allowOverdraft=true 时，
// actual 可以超过原冻结额，超出的 shortfall 会直接计入 overdraft。
func (s *DeductionService) Confirm(params ConfirmParams) (*ConfirmResult, error) {
	ctx := context.Background()
	if params.EventID == "" && params.IdempotencyKey == "" {
		return nil, fmt.Errorf("eventId or idempotencyKey is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var tenantID, userID, eventClientID string
	var frozenTenant, frozenUser *int64
	var status, metadata string
	eventID, err := resolveEventID(ctx, tx, params.EventID, params.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(ctx, `
		SELECT tenant_id, COALESCE(user_id, ''), COALESCE(client_id, ''),
		       tenant_credits, user_credits, status, COALESCE(metadata::text, '{}')
		FROM bill_events WHERE event_id = $1 FOR UPDATE
	`, eventID).Scan(&tenantID, &userID, &eventClientID, &frozenTenant, &frozenUser, &status, &metadata)
	if err != nil {
		return nil, shared.ErrTransactionNotFound
	}
	if params.ClientID != "" && eventClientID != params.ClientID {
		return nil, shared.ErrForbidden
	}

	frozenTenantVal := int64(0)
	if frozenTenant != nil {
		frozenTenantVal = *frozenTenant
	}
	frozenUserVal := int64(0)
	if frozenUser != nil {
		frozenUserVal = *frozenUser
	}

	if status == billing.EventStatusSucceeded {
		if !sameAmounts(frozenTenantVal, params.ActualTenantAmount, frozenUserVal, params.ActualUserAmount) {
			return nil, fmt.Errorf("event already succeeded with a different amount")
		}
		return &ConfirmResult{
			EventID:            eventID,
			TenantDeducted:     metadataTenantDeducted(metadata, frozenTenantVal),
			UserDeducted:       metadataUserDeducted(metadata, frozenUserVal),
			TenantOverdraftAdd: metadataTenantOverdraft(metadata),
			UserOverdraftAdd:   metadataUserOverdraft(metadata),
			AccountState:       metadataAccountState(metadata),
			AllowFurtherUsage:  metadataAllowFurtherUsage(metadata),
			Status:             "SUCCESS",
		}, nil
	}
	escrowAlreadyReleased := status == billing.EventStatusReleased
	if status != billing.EventStatusPending && !(params.AllowReleased && escrowAlreadyReleased) {
		return nil, fmt.Errorf("event not pending (status=%s)", status)
	}

	if params.ActualTenantAmount < 0 || params.ActualUserAmount < 0 {
		return nil, shared.ErrInvalidAmount
	}
	if !params.AllowOverdraft {
		if params.ActualTenantAmount > frozenTenantVal {
			return nil, fmt.Errorf("actualTenantAmount(%d) exceeds frozen tenant amount(%d)", params.ActualTenantAmount, frozenTenantVal)
		}
		if params.ActualUserAmount > frozenUserVal {
			return nil, fmt.Errorf("actualUserAmount(%d) exceeds frozen user amount(%d)", params.ActualUserAmount, frozenUserVal)
		}
	}

	now := billing.NowUTC()
	result := &ConfirmResult{EventID: eventID}

	if frozenTenantVal > 0 || params.ActualTenantAmount > 0 {
		if params.ActualTenantAmount > 0 {
			if params.AllowOverdraft {
				ownedFrozen := frozenTenantVal
				if escrowAlreadyReleased {
					ownedFrozen = 0
				}
				shortfall, err := pg.DeductFIFOPartialPreservingFrozen(
					ctx, tx, billing.PackageTypeTenant, tenantID, "",
					params.ActualTenantAmount, ownedFrozen, now)
				if err != nil {
					return nil, fmt.Errorf("租户 FIFO 扣减失败：%w", err)
				}
				result.TenantDeducted = params.ActualTenantAmount - shortfall
				if shortfall > 0 {
					if err := pg.IncreaseTenantOverdraft(ctx, tx, tenantID, shortfall); err != nil {
						return nil, err
					}
					result.TenantOverdraftAdd = shortfall
				}
			} else if err := pg.DeductFIFO(ctx, tx, billing.PackageTypeTenant, tenantID, "", params.ActualTenantAmount, now); err != nil {
				return nil, fmt.Errorf("租户 FIFO 扣减失败：%w", err)
			}
			if !params.AllowOverdraft {
				result.TenantDeducted = params.ActualTenantAmount
			}
		}
		if !escrowAlreadyReleased && frozenTenantVal > 0 {
			if err := pg.ReduceTenantFrozen(ctx, tx, tenantID, frozenTenantVal); err != nil {
				return nil, fmt.Errorf("减少租户冻结额失败：%w", err)
			}
		}
	}

	if frozenUserVal > 0 || params.ActualUserAmount > 0 {
		if params.ActualUserAmount > 0 {
			if params.AllowOverdraft {
				ownedFrozen := frozenUserVal
				if escrowAlreadyReleased {
					ownedFrozen = 0
				}
				shortfall, err := pg.DeductFIFOPartialPreservingFrozen(
					ctx, tx, billing.PackageTypeUser, tenantID, userID,
					params.ActualUserAmount, ownedFrozen, now)
				if err != nil {
					return nil, fmt.Errorf("用户 FIFO 扣减失败：%w", err)
				}
				result.UserDeducted = params.ActualUserAmount - shortfall
				if shortfall > 0 {
					if err := pg.IncreaseUserOverdraft(ctx, tx, userID, shortfall); err != nil {
						return nil, err
					}
					result.UserOverdraftAdd = shortfall
				}
			} else if err := pg.DeductFIFO(ctx, tx, billing.PackageTypeUser, tenantID, userID, params.ActualUserAmount, now); err != nil {
				return nil, fmt.Errorf("用户 FIFO 扣减失败：%w", err)
			}
			if !params.AllowOverdraft {
				result.UserDeducted = params.ActualUserAmount
			}
		}
		if !escrowAlreadyReleased && frozenUserVal > 0 {
			if err := pg.ReduceUserFrozen(ctx, tx, userID, frozenUserVal); err != nil {
				return nil, fmt.Errorf("减少用户冻结额失败：%w", err)
			}
		}
	}

	var actualTenantVal, actualUserVal any
	if params.ActualTenantAmount > 0 {
		actualTenantVal = params.ActualTenantAmount
	}
	if params.ActualUserAmount > 0 {
		actualUserVal = params.ActualUserAmount
	}
	accountState, allowFurtherUsage, err := currentEventState(ctx, tx, tenantID, userID, params.ActualTenantAmount > 0 || frozenTenantVal > 0, params.ActualUserAmount > 0 || frozenUserVal > 0)
	if err != nil {
		return nil, err
	}
	result.AccountState = accountState
	result.AllowFurtherUsage = allowFurtherUsage
	_, err = tx.Exec(ctx, `
		UPDATE bill_events
		SET tenant_credits = $1,
		    user_credits = $2,
		    status = 'succeeded',
		    metadata = COALESCE(metadata, '{}'::jsonb) || $3::jsonb,
		    finished_at = $4
		WHERE event_id = $5
	`, actualTenantVal, actualUserVal, encodeSettlementMetadata(settlementMetadata{
		Mode:               modeString(params.AllowOverdraft),
		TenantDeducted:     result.TenantDeducted,
		UserDeducted:       result.UserDeducted,
		TenantOverdraftAdd: result.TenantOverdraftAdd,
		UserOverdraftAdd:   result.UserOverdraftAdd,
		AccountState:       result.AccountState,
		AllowFurtherUse:    result.AllowFurtherUsage,
		FrozenTenant:       frozenTenantVal,
		FrozenUser:         frozenUserVal,
	}), now, eventID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.logger.Info("Confirm completed",
		zap.String("event_id", eventID),
		zap.Int64("actualTenant", params.ActualTenantAmount),
		zap.Int64("actualUser", params.ActualUserAmount),
		zap.String("account_state", result.AccountState),
	)
	result.Status = "SUCCESS"
	return result, nil
}

// Cancel 取消预授权（释放双账户冻结额）
func (s *DeductionService) Cancel(params CancelParams) (*CancelResult, error) {
	ctx := context.Background()
	if params.EventID == "" && params.IdempotencyKey == "" {
		return nil, fmt.Errorf("eventId or idempotencyKey is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var tenantID, userID, eventClientID string
	var frozenTenant, frozenUser *int64
	var status string
	eventID, err := resolveEventID(ctx, tx, params.EventID, params.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(ctx, `
		SELECT tenant_id, COALESCE(user_id, ''), COALESCE(client_id, ''),
		       tenant_credits, user_credits, status
		FROM bill_events WHERE event_id = $1 FOR UPDATE
	`, eventID).Scan(&tenantID, &userID, &eventClientID, &frozenTenant, &frozenUser, &status)
	if err != nil {
		return nil, shared.ErrTransactionNotFound
	}
	if params.ClientID != "" && eventClientID != params.ClientID {
		return nil, shared.ErrForbidden
	}

	if status == billing.EventStatusCancelled {
		return &CancelResult{
			EventID:           eventID,
			AccountState:      accountStateOK,
			AllowFurtherUsage: true,
			Status:            "SUCCESS",
		}, nil
	}
	if status != billing.EventStatusPending {
		return nil, fmt.Errorf("event not pending (status=%s)", status)
	}

	if frozenTenant != nil && *frozenTenant > 0 {
		if err := pg.ReduceTenantFrozen(ctx, tx, tenantID, *frozenTenant); err != nil {
			return nil, fmt.Errorf("释放租户冻结额失败：%w", err)
		}
	}
	if frozenUser != nil && *frozenUser > 0 && userID != "" {
		if err := pg.ReduceUserFrozen(ctx, tx, userID, *frozenUser); err != nil {
			return nil, fmt.Errorf("释放用户冻结额失败：%w", err)
		}
	}

	now := billing.NowUTC()
	accountState, allowFurtherUsage, err := currentEventState(ctx, tx, tenantID, userID, frozenTenant != nil && *frozenTenant > 0, frozenUser != nil && *frozenUser > 0)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE bill_events
		SET status = 'cancelled',
		    terminal_note = 'Cancelled by caller',
		    metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb,
		    finished_at = $2
		WHERE event_id = $3
	`, encodeSettlementMetadata(settlementMetadata{
		AccountState:    accountState,
		AllowFurtherUse: allowFurtherUsage,
	}), now, eventID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.logger.Info("Cancel completed", zap.String("event_id", eventID))
	return &CancelResult{
		EventID:           eventID,
		AccountState:      accountState,
		AllowFurtherUsage: allowFurtherUsage,
		Status:            "SUCCESS",
	}, nil
}

// ConsumeParams 单阶段聚合扣款请求（不经过 Freeze/Confirm 两阶段）。
//
// 适用场景：AI 业务域已在本地完成精细计费（如分账层聚合多次
// 微小消费），仅在达到阈值时通过进程内计费服务把整数积分一次性扣掉。失败时不需要
// 反向 Cancel —— 单 SQL 事务，原子。
//
// 与 Freeze/Confirm 不同：
//   - 不冻结、不分阶段，直接扣减积分包 FIFO
//   - 仅完成态尾差可显式转入 current_overdraft；账户一旦欠费，后续准入停止
//   - 账户已透支（current_overdraft > 0）则直接拒绝，必须充值清欠后才能继续
type ConsumeParams struct {
	IdempotencyKey string
	ClientID       string
	TenantID       string
	UserID         string
	Description    string
	TenantAmount   int64
	UserAmount     int64
	// DisallowOverdraft 为 true 时，余额不足不转透支，整单回滚返回 ErrInsufficientBalance。
	// 仅为 V1 调用方保留；V2 严格扣款不允许欠费。
	DisallowOverdraft bool
	// AllowOverdraft 必须显式声明。只有已经完成工作的结算尾差可以启用。
	AllowOverdraft bool
}

// ConsumeResult 单阶段扣款结果
type ConsumeResult struct {
	EventID            string
	TenantDeducted     int64 // 从租户积分包实际扣减的部分
	UserDeducted       int64 // 从用户积分包实际扣减的部分
	TenantOverdraftAdd int64 // 计入租户欠费的部分
	UserOverdraftAdd   int64 // 计入用户欠费的部分
	AccountState       string
	AllowFurtherUsage  bool
}

// Consume 单阶段幂等扣款
func (s *DeductionService) Consume(params ConsumeParams) (*ConsumeResult, error) {
	if params.IdempotencyKey == "" {
		return nil, fmt.Errorf("idempotencyKey is required")
	}
	if params.TenantID == "" {
		return nil, fmt.Errorf("tenantId is required")
	}
	if params.TenantAmount < 0 || params.UserAmount < 0 {
		return nil, shared.ErrInvalidAmount
	}
	if params.TenantAmount == 0 && params.UserAmount == 0 {
		return nil, fmt.Errorf("at least one of tenantAmount or userAmount must be > 0")
	}
	if params.UserAmount > 0 && params.UserID == "" {
		return nil, fmt.Errorf("userId is required when userAmount > 0")
	}

	ctx := context.Background()

	// 校验租户状态
	var tenantStatus string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(status, 'active') FROM iam_tenants WHERE tenant_id = $1`, params.TenantID).Scan(&tenantStatus); err != nil {
		return nil, fmt.Errorf("tenant %q not found", params.TenantID)
	}
	if tenantStatus != "active" {
		return nil, shared.ErrTenantSuspended
	}

	// 用户归属校验
	if params.UserID != "" {
		var count int
		if err := s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM iam_users WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
		`, params.UserID, params.TenantID).Scan(&count); err != nil || count == 0 {
			return nil, fmt.Errorf("user %q does not belong to tenant %q", params.UserID, params.TenantID)
		}
	}

	// 幂等检查（事务外快速短路；事务内仍会有唯一约束兜底）
	if existing, err := s.eventRepo.FindByIdempotencyKey(params.IdempotencyKey); err == nil && existing != nil {
		tenantDeducted, userDeducted := int64(0), int64(0)
		if existing.TenantCredits != nil {
			tenantDeducted = *existing.TenantCredits
		}
		if existing.UserCredits != nil {
			userDeducted = *existing.UserCredits
		}
		if existing.ClientID != params.ClientID || existing.TenantID != params.TenantID ||
			existing.UserID != params.UserID || tenantDeducted != params.TenantAmount || userDeducted != params.UserAmount {
			return nil, fmt.Errorf("idempotency key already belongs to a different debit request")
		}
		return &ConsumeResult{
			EventID:            existing.EventID,
			TenantDeducted:     metadataTenantDeducted(existing.Metadata, tenantDeducted),
			UserDeducted:       metadataUserDeducted(existing.Metadata, userDeducted),
			TenantOverdraftAdd: metadataTenantOverdraft(existing.Metadata),
			UserOverdraftAdd:   metadataUserOverdraft(existing.Metadata),
			AccountState:       metadataAccountState(existing.Metadata),
			AllowFurtherUsage:  metadataAllowFurtherUsage(existing.Metadata),
		}, nil
	}

	txCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := s.pool.Begin(txCtx)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败：%w", err)
	}
	defer tx.Rollback(txCtx)

	now := billing.NowUTC()
	result := &ConsumeResult{}
	allowOverdraft := consumeAllowsOverdraft(params)

	// 租户扣减
	if params.TenantAmount > 0 {
		overdraftLimit, currentOverdraft, err := pg.GetTenantOverdraft(txCtx, tx, params.TenantID)
		if err != nil {
			return nil, err
		}
		if !allowOverdraft && currentOverdraft > 0 {
			return nil, shared.ErrTenantInOverdraft
		}
		shortfall, err := pg.DeductFIFOPartialPreservingFrozen(
			txCtx, tx, billing.PackageTypeTenant, params.TenantID, "",
			params.TenantAmount, 0, now)
		if err != nil {
			return nil, fmt.Errorf("租户 FIFO 扣减失败：%w", err)
		}
		result.TenantDeducted = params.TenantAmount - shortfall
		if shortfall > 0 {
			if !allowOverdraft {
				return nil, shared.ErrInsufficientBalance
			}
			if err := pg.IncreaseTenantOverdraft(txCtx, tx, params.TenantID, shortfall); err != nil {
				return nil, err
			}
			result.TenantOverdraftAdd = shortfall
		}
		_ = overdraftLimit
	}

	// 用户扣减
	if params.UserAmount > 0 {
		overdraftLimit, currentOverdraft, err := pg.GetUserOverdraft(txCtx, tx, params.UserID)
		if err != nil {
			return nil, err
		}
		if !allowOverdraft && currentOverdraft > 0 {
			return nil, shared.ErrUserInOverdraft
		}
		shortfall, err := pg.DeductFIFOPartialPreservingFrozen(
			txCtx, tx, billing.PackageTypeUser, params.TenantID, params.UserID,
			params.UserAmount, 0, now)
		if err != nil {
			return nil, fmt.Errorf("用户 FIFO 扣减失败：%w", err)
		}
		result.UserDeducted = params.UserAmount - shortfall
		if shortfall > 0 {
			if !allowOverdraft {
				return nil, shared.ErrInsufficientBalance
			}
			if err := pg.IncreaseUserOverdraft(txCtx, tx, params.UserID, shortfall); err != nil {
				return nil, err
			}
			result.UserOverdraftAdd = shortfall
		}
		_ = overdraftLimit
	}

	eventID := "EV_" + uuid.New().String()[:24]
	result.EventID = eventID

	var tenantCreditsVal, userCreditsVal any
	if params.TenantAmount > 0 {
		tenantCreditsVal = params.TenantAmount
	}
	if params.UserAmount > 0 {
		userCreditsVal = params.UserAmount
	}

	accountState, allowFurtherUsage, err := currentEventState(txCtx, tx, params.TenantID, params.UserID, params.TenantAmount > 0, params.UserAmount > 0)
	if err != nil {
		return nil, err
	}
	result.AccountState = accountState
	result.AllowFurtherUsage = allowFurtherUsage

	// 写入流水：status='succeeded'（单阶段一次扣完）。description 由调用方填写，
	// 推荐写明 "ai-gateway 聚合扣款（N次请求，明细见业务系统）" 等审计信息。
	if _, err := tx.Exec(txCtx, `
		INSERT INTO bill_events
		(event_id, idempotency_key, tenant_id, user_id, description, client_id,
		 event_type, tenant_credits, user_credits, status, metadata, created_at, finished_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''),
		        'charge', $7, $8, 'succeeded', $9::jsonb, $10, $10)
	`, eventID, params.IdempotencyKey, params.TenantID, params.UserID,
		params.Description, params.ClientID,
		tenantCreditsVal, userCreditsVal, encodeSettlementMetadata(settlementMetadata{
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

	if err := tx.Commit(txCtx); err != nil {
		return nil, fmt.Errorf("提交事务失败：%w", err)
	}

	s.logger.Info("Consume 扣款成功",
		zap.String("event_id", eventID),
		zap.String("tenant_id", params.TenantID),
		zap.String("user_id", params.UserID),
		zap.Int64("tenant_deducted", result.TenantDeducted),
		zap.Int64("user_deducted", result.UserDeducted),
		zap.Int64("tenant_overdraft_add", result.TenantOverdraftAdd),
		zap.Int64("user_overdraft_add", result.UserOverdraftAdd),
		zap.String("account_state", result.AccountState),
	)

	return result, nil
}

// Refund 全额退款（仅平台管理员可操作）
// 退款策略：新建一个永久积分包（source=REFUND），原消费事件标记为 refunded
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

	// 退款清欠策略：先抵扣 current_overdraft（清欠），剩余金额才进入新的退款积分包。
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
				return fmt.Errorf("退回租户积分失败：%w", err)
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
				return fmt.Errorf("退回用户积分失败：%w", err)
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
	// 在线充值（微信支付/现金购积分）不可撤销，权限校验之外再兜底一道
	if slices.Contains(billing.OnlineOrderTypes, orderType) {
		return nil, shared.ErrRechargeNotReversible
	}

	// 2. 查找关联积分包并加锁
	var packageID, pkgStatus string
	var totalCredits, remainingCredits int64
	err = tx.QueryRow(ctx, `
		SELECT package_id, status, total_credits, remaining_credits
		FROM bill_credit_packages WHERE recharge_order_id = $1 FOR UPDATE
	`, orderID).Scan(&packageID, &pkgStatus, &totalCredits, &remainingCredits)
	if err != nil {
		return nil, fmt.Errorf("未找到关联积分包: %w", err)
	}

	// 3. 积分已全部消耗，拒绝撤销
	if remainingCredits == 0 {
		return nil, shared.ErrRechargeCreditsExhausted
	}

	// 4. 计算撤销数据
	lostCredits := totalCredits - remainingCredits
	isPartial := lostCredits > 0
	fullRevoke := !isPartial

	// 5. 撤销积分包
	if err := pg.RevokeCreditPackage(ctx, tx, packageID, fullRevoke); err != nil {
		return nil, fmt.Errorf("撤销积分包失败: %w", err)
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

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
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

func classifyAccountState(available, limit, current int64) string {
	if current > 0 || available <= 0 {
		return accountStateExhausted
	}
	_ = limit // legacy column no longer participates in admission.
	return accountStateOK
}

func mergeAccountStates(states ...string) string {
	merged := accountStateOK
	for _, state := range states {
		switch state {
		case accountStateExhausted:
			return accountStateExhausted
		case accountStateOverdraft:
			merged = accountStateOverdraft
		}
	}
	return merged
}

func currentEventState(ctx context.Context, tx pgx.Tx, tenantID, userID string, includeTenant, includeUser bool) (string, bool, error) {
	var tenantState, userState string
	if includeTenant && tenantID != "" {
		available, _, err := pg.GetTenantAvailableBalance(ctx, tx, tenantID, billing.NowUTC())
		if err != nil {
			return "", false, fmt.Errorf("查询租户余额失败：%w", err)
		}
		limit, current, err := pg.GetTenantOverdraft(ctx, tx, tenantID)
		if err != nil {
			return "", false, err
		}
		tenantState = classifyAccountState(available, limit, current)
	}
	if includeUser && userID != "" {
		available, _, err := pg.GetUserAvailableBalance(ctx, tx, userID, billing.NowUTC())
		if err != nil {
			return "", false, fmt.Errorf("查询用户余额失败：%w", err)
		}
		limit, current, err := pg.GetUserOverdraft(ctx, tx, userID)
		if err != nil {
			return "", false, err
		}
		userState = classifyAccountState(available, limit, current)
	}
	state := mergeAccountStates(tenantState, userState)
	return state, state != accountStateExhausted, nil
}

func resolveEventID(ctx context.Context, tx pgx.Tx, eventID, idempotencyKey string) (string, error) {
	if eventID != "" {
		return eventID, nil
	}
	if idempotencyKey == "" {
		return "", shared.ErrTransactionNotFound
	}
	var resolved string
	if err := tx.QueryRow(ctx, `SELECT event_id FROM bill_events WHERE idempotency_key = $1`, idempotencyKey).Scan(&resolved); err != nil {
		return "", shared.ErrTransactionNotFound
	}
	return resolved, nil
}

func sameAmounts(tenantA, tenantB, userA, userB int64) bool {
	return tenantA == tenantB && userA == userB
}

// appendOpTx 在事务内追加一条 ops 记录到 bill_events.metadata
func appendOpTx(ctx context.Context, tx pgx.Tx, eventID string, op map[string]any) error {
	opJSON, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("序列化 op 失败: %w", err)
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

// AppendOp 在独立连接追加一条 ops 记录（非事务场景，如 scheduler）
func (s *DeductionService) AppendOp(ctx context.Context, eventID string, op map[string]any) error {
	opJSON, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("序列化 op 失败: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
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

// ManualConfirm 管理员手动确认已释放事件（released → succeeded）
// 金额由管理员自由决定，与原冻结额无关，只需账户余额充足
// 至少一个金额 > 0（否则应使用 AdminDismiss）
func (s *DeductionService) ManualConfirm(eventID string, actualTenant, actualUser int64, operatorID, note string) error {
	if actualTenant < 0 || actualUser < 0 {
		return shared.ErrInvalidAmount
	}
	if actualTenant == 0 && actualUser == 0 {
		return fmt.Errorf("金额不能全为零，如需免除扣费请使用 AdminDismiss")
	}

	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var tenantID, userID, status string
	err = tx.QueryRow(ctx, `
		SELECT tenant_id, COALESCE(user_id, ''), status
		FROM bill_events WHERE event_id = $1 FOR UPDATE
	`, eventID).Scan(&tenantID, &userID, &status)
	if err != nil {
		return shared.ErrTransactionNotFound
	}
	if status != billing.EventStatusReleased {
		return fmt.Errorf("只能对 released 状态的事件执行手动确认（当前状态: %s）", status)
	}

	now := billing.NowUTC()

	if actualTenant > 0 {
		available, _, err := pg.GetTenantAvailableBalance(ctx, tx, tenantID, now)
		if err != nil {
			return fmt.Errorf("查询租户余额失败: %w", err)
		}
		if available < actualTenant {
			return shared.ErrTenantInsufficientBalance
		}
		if err := pg.DeductFIFO(ctx, tx, billing.PackageTypeTenant, tenantID, "", actualTenant, now); err != nil {
			return fmt.Errorf("租户 FIFO 扣减失败: %w", err)
		}
	}

	if actualUser > 0 {
		if userID == "" {
			return fmt.Errorf("事件无关联用户，无法扣减用户积分")
		}
		available, _, err := pg.GetUserAvailableBalance(ctx, tx, userID, now)
		if err != nil {
			return fmt.Errorf("查询用户余额失败: %w", err)
		}
		if available < actualUser {
			return shared.ErrUserInsufficientBalance
		}
		if err := pg.DeductFIFO(ctx, tx, billing.PackageTypeUser, tenantID, userID, actualUser, now); err != nil {
			return fmt.Errorf("用户 FIFO 扣减失败: %w", err)
		}
	}

	var actualTenantVal, actualUserVal any
	if actualTenant > 0 {
		actualTenantVal = actualTenant
	}
	if actualUser > 0 {
		actualUserVal = actualUser
	}
	_, err = tx.Exec(ctx, `
		UPDATE bill_events
		SET status = 'succeeded',
		    tenant_credits = $1,
		    user_credits = $2,
		    finished_at = $3
		WHERE event_id = $4
	`, actualTenantVal, actualUserVal, now, eventID)
	if err != nil {
		return err
	}

	if err := appendOpTx(ctx, tx, eventID, map[string]any{
		"action":        "manual_confirm",
		"operator_id":   operatorID,
		"note":          note,
		"actual_tenant": actualTenant,
		"actual_user":   actualUser,
		"at":            now.UnixMilli(),
	}); err != nil {
		return fmt.Errorf("写 manual_confirm op 失败: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.logger.Info("ManualConfirm completed",
		zap.String("event_id", eventID),
		zap.String("operator_id", operatorID),
		zap.Int64("actualTenant", actualTenant),
		zap.Int64("actualUser", actualUser),
	)
	return nil
}

// AdminDismiss 管理员免除收费（released → cancelled）
// 不扣任何积分，语义是"确认不收费"
func (s *DeductionService) AdminDismiss(eventID, operatorID, note string) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT status FROM bill_events WHERE event_id = $1 FOR UPDATE
	`, eventID).Scan(&status)
	if err != nil {
		return shared.ErrTransactionNotFound
	}
	if status != billing.EventStatusReleased {
		return fmt.Errorf("只能对 released 状态的事件执行免除操作（当前状态: %s）", status)
	}

	now := billing.NowUTC()
	_, err = tx.Exec(ctx, `
		UPDATE bill_events
		SET status = 'cancelled',
		    terminal_note = '管理员确认免除扣费',
		    finished_at = $1
		WHERE event_id = $2
	`, now, eventID)
	if err != nil {
		return err
	}

	if err := appendOpTx(ctx, tx, eventID, map[string]any{
		"action":      "admin_dismissed",
		"operator_id": operatorID,
		"note":        note,
		"at":          now.UnixMilli(),
	}); err != nil {
		return fmt.Errorf("写 admin_dismissed op 失败: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.logger.Info("AdminDismiss completed",
		zap.String("event_id", eventID),
		zap.String("operator_id", operatorID),
	)
	return nil
}

// BatchConfirm 批量手动确认（使用原冻结额作为实际扣减额，max 100）
func (s *DeductionService) BatchConfirm(eventIDs []string, note, operatorID string) BatchOpResult {
	if len(eventIDs) > 100 {
		eventIDs = eventIDs[:100]
	}

	result := BatchOpResult{
		Succeeded: make([]string, 0),
		Failed:    make([]BatchOpError, 0),
	}

	for _, eventID := range eventIDs {
		// 读取原始冻结额
		var tenantCredits, userCredits *int64
		err := s.pool.QueryRow(context.Background(), `
			SELECT tenant_credits, user_credits
			FROM bill_events WHERE event_id = $1 AND status = 'released'
		`, eventID).Scan(&tenantCredits, &userCredits)
		if err != nil {
			result.Failed = append(result.Failed, BatchOpError{EventID: eventID, Reason: "事件不存在或状态非 released"})
			continue
		}

		actualTenant := int64(0)
		if tenantCredits != nil {
			actualTenant = *tenantCredits
		}
		actualUser := int64(0)
		if userCredits != nil {
			actualUser = *userCredits
		}

		if actualTenant == 0 && actualUser == 0 {
			result.Failed = append(result.Failed, BatchOpError{EventID: eventID, Reason: "原冻结额为零，请使用 AdminDismiss"})
			continue
		}

		if err := s.ManualConfirm(eventID, actualTenant, actualUser, operatorID, note); err != nil {
			result.Failed = append(result.Failed, BatchOpError{EventID: eventID, Reason: err.Error()})
			continue
		}

		result.Succeeded = append(result.Succeeded, eventID)
		result.TotalTenantCredits += actualTenant
		result.TotalUserCredits += actualUser
	}

	return result
}

// BatchRefund 批量退款（max 100）
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

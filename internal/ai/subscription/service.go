package subscription

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
)

const defaultMaxQueue = 2

// window 长度常量（与 DebitSubscription 的 interval 对齐）。
const (
	window5h = 5 * time.Hour
	window7d = 7 * 24 * time.Hour
)

// Service 订阅领域服务。热路径（ResolveForGate/Debit）低延迟；购买一致性靠
// 订单状态机 + janitor 幂等重放补偿。
type Service struct {
	repo      Repo
	purchaser Purchaser
	logger    *zap.Logger
	maxQueue  int
}

// NewService 构造订阅服务。maxQueue 为排队订阅上限（不含 active），默认 2。
func NewService(repo Repo, purchaser Purchaser, logger *zap.Logger, opts ...func(*Service)) *Service {
	s := &Service{repo: repo, purchaser: purchaser, logger: logger, maxQueue: defaultMaxQueue}
	for _, o := range opts {
		o(s)
	}
	if s.logger == nil {
		s.logger = zap.NewNop()
	}
	if s.maxQueue <= 0 {
		s.maxQueue = defaultMaxQueue
	}
	return s
}

// WithMaxQueue 覆盖排队上限。
func WithMaxQueue(n int) func(*Service) { return func(s *Service) { s.maxQueue = n } }

// ---- 热路径 ----

// ResolveForGate 每请求调用：懒惰过期/激活 + 三窗口余量判定。
func (s *Service) ResolveForGate(ctx context.Context, tenantID, userID string) (GateDecision, error) {
	active, err := s.resolveActive(ctx, tenantID, userID)
	if err != nil {
		return GateDecision{}, err
	}
	if active == nil {
		return GateDecision{Covered: false}, nil
	}
	return GateDecision{
		Covered:                    s.hasRemaining(active, time.Now()),
		SubscriptionID:             active.ID,
		GroupQuotaDebitMultipliers: active.GroupQuotaDebitMultipliers,
	}, nil
}

// DebitAdmitted records usage for a request admitted while the subscription
// was usable. Completion-time status changes do not alter the committed source.
func (s *Service) DebitAdmitted(ctx context.Context, subscriptionID string, userMicro int64) error {
	if userMicro <= 0 {
		return nil
	}
	_, err := s.repo.Debit(ctx, subscriptionID, userMicro)
	return err
}

// resolveActive 懒惰推进订阅状态机并返回当前 active 订阅（无则 nil）：
//   - active 已过期 ⇒ 置 expired（DB now() 权威判定）
//   - 无 active 且有 pending ⇒ 激活最早的一份（now() 激活）
//
// 供 ResolveForGate 与 CurrentSubscription 同源复用。
func (s *Service) resolveActive(ctx context.Context, tenantID, userID string) (*Subscription, error) {
	live, err := s.repo.GetLiveSubs(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	active, earliestPending := classifyLiveSubscriptions(live)

	// 懒惰过期：app 时钟仅作预筛，ExpireIfDue 用 DB now() 权威判定。
	if active != nil && active.ExpiresAt != nil && time.Now().After(*active.ExpiresAt) {
		expired, err := s.repo.ExpireIfDue(ctx, active.ID)
		if err != nil {
			return nil, err
		}
		if expired {
			active = nil
		} else {
			// Another gate may have expired the row and activated the next queued
			// subscription after our initial read. Re-read instead of admitting the
			// stale active object.
			refreshed, err := s.repo.GetLiveSubs(ctx, tenantID, userID)
			if err != nil {
				return nil, err
			}
			active, earliestPending = classifyLiveSubscriptions(refreshed)
		}
	}

	// 懒惰激活：无 active 且有排队 ⇒ 激活最早 pending。
	if active == nil && earliestPending != nil {
		activated, err := s.repo.Activate(ctx, earliestPending.ID)
		if err != nil {
			return nil, err
		}
		if activated {
			sub, err := s.repo.GetSubscription(ctx, earliestPending.ID)
			if err != nil {
				return nil, err
			}
			if sub.Status == SubActive {
				active = sub
			}
		} else {
			// Another request may have activated the same pending row after our
			// initial read. Re-read instead of incorrectly falling back to PAYG.
			refreshed, err := s.repo.GetLiveSubs(ctx, tenantID, userID)
			if err != nil {
				return nil, err
			}
			for i := range refreshed {
				if refreshed[i].Status == SubActive {
					active = &refreshed[i]
					break
				}
			}
		}
	}
	return active, nil
}

func classifyLiveSubscriptions(live []Subscription) (active, earliestPending *Subscription) {
	for i := range live {
		switch live[i].Status {
		case SubActive:
			active = &live[i]
		case SubPending:
			if earliestPending == nil {
				earliestPending = &live[i] // GetLiveSubs 按 created_at ASC，首个即最早
			}
		}
	}
	return active, earliestPending
}

// hasRemaining 三窗口余量判定：总额度与两个时间窗均需有余量。
func (s *Service) hasRemaining(sub *Subscription, now time.Time) bool {
	if sub.TotalLimitMicro-sub.TotalUsedMicro <= 0 {
		return false
	}
	return windowHasRemaining(sub.Window5hLimitMicro, sub.Win5hStart, sub.Win5hUsedMicro, window5h, now) &&
		windowHasRemaining(sub.Window7dLimitMicro, sub.Win7dStart, sub.Win7dUsedMicro, window7d, now)
}

// windowHasRemaining：limit 为 nil=不限；窗口未开或已翻转 ⇒ 满额；否则 limit-used>0。
func windowHasRemaining(limit *int64, start *time.Time, used int64, dur time.Duration, now time.Time) bool {
	if limit == nil {
		return true
	}
	if start == nil || now.After(start.Add(dur)) {
		return *limit > 0
	}
	return *limit-used > 0
}

// ---- 购买 ----

// Purchase 购买套餐。状态机见 docs/ai-subscription-design.md §6。
// 返回 (order, subscription, error)：
//   - 余额不足 ⇒ ErrInsufficientBalance（订单置 failed）
//   - 扣款未知态 / finalize 失败 ⇒ ErrOrderProcessing（订单停 deducting，janitor 补偿），subscription 为 nil
//   - 成功 ⇒ order.Status=paid + subscription
func (s *Service) Purchase(ctx context.Context, p PurchaseParams) (*Order, *Subscription, error) {
	if p.IdempotencyKey == "" {
		return nil, nil, errors.New("subscription: idempotency key is required")
	}
	orderNo := purchaseOrderNo(p)
	reservation, err := s.repo.ReservePurchase(ctx, orderNo, p, s.maxQueue)
	if err != nil {
		return nil, nil, err
	}
	order := reservation.Order
	if order == nil {
		return nil, nil, errors.New("subscription: purchase reservation missing order")
	}
	if reservation.Replayed {
		switch order.Status {
		case OrderPaid:
			sub, err := s.repo.GetSubscription(ctx, order.SubscriptionID)
			return order, sub, err
		case OrderCreated, OrderDeducting:
			return order, nil, ErrOrderProcessing
		case OrderFailed:
			if order.FailReason == "insufficient_balance" {
				return nil, nil, ErrInsufficientBalance
			}
			return order, nil, ErrOrderProcessing
		}
	}
	priceMicro, ok := domain.CreditsToMicro(order.PriceCredits)
	if !ok {
		return nil, nil, ErrPlanQuotaInvalid
	}
	if _, err := s.repo.MarkOrderDeducting(ctx, order.ID); err != nil {
		return nil, nil, err
	}

	resp, err := s.purchaser.DebitStrict(ctx, DebitRequest{
		IdempotencyKey: "ai-sub-" + order.OrderNo,
		TenantID:       p.TenantID,
		UserID:         p.UserID,
		Description:    "AI订阅套餐购买: " + order.PlanNameSnapshot,
		UserMicro:      priceMicro,
	})
	if err != nil {
		if errors.Is(err, ErrInsufficientBalance) {
			if _, ferr := s.repo.MarkOrderFailed(ctx, order.ID, "insufficient_balance"); ferr != nil {
				s.logger.Warn("mark order failed after insufficient balance", zap.String("order", order.OrderNo), zap.Error(ferr))
			}
			return nil, nil, ErrInsufficientBalance
		}
		// 未知错误：订单留在 deducting，janitor 用同一幂等键重放推进。
		s.logger.Warn("billing consume unknown error, order left deducting for janitor",
			zap.String("order", order.OrderNo), zap.Error(err))
		return order, nil, ErrOrderProcessing
	}

	sub, err := s.repo.FinalizeOrder(ctx, order, resp.AuthorizationID)
	if err != nil {
		// 已扣款但本地开通失败 ⇒ 留 deducting，janitor 重放 FinalizeOrder 推进到 paid。
		s.logger.Error("finalize order failed, order left deducting for janitor",
			zap.String("order", order.OrderNo), zap.String("authorization", resp.AuthorizationID), zap.Error(err))
		return order, nil, ErrOrderProcessing
	}
	paidOrder, err := s.repo.GetOrder(ctx, order.ID)
	if err != nil {
		return order, sub, nil
	}
	return paidOrder, sub, nil
}

func purchaseOrderNo(p PurchaseParams) string {
	sum := sha256.Sum256([]byte(p.TenantID + "\x00" + p.UserID + "\x00" + p.IdempotencyKey))
	return "SUBI" + hex.EncodeToString(sum[:16])
}

// ---- 查询 ----

// CurrentSubscription 当前生效订阅（懒惰触发激活/过期流转）；无则 (nil, nil)。
func (s *Service) CurrentSubscription(ctx context.Context, tenantID, userID string) (*Subscription, error) {
	return s.resolveActive(ctx, tenantID, userID)
}

func (s *Service) ListPlans(ctx context.Context, f PlanFilter) ([]Plan, int64, error) {
	f.Limit, f.Offset = clampPage(f.Limit, f.Offset)
	plans, total, err := s.repo.ListPlans(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	return plans, total, nil
}

// ListPlansForUser returns storefront plans with eligibility computed in one
// repository call using the same policy interface as purchase reservation.
func (s *Service) ListPlansForUser(ctx context.Context, f PlanFilter, userID string) ([]Plan, int64, error) {
	plans, total, err := s.ListPlans(ctx, f)
	if err != nil || len(plans) == 0 {
		return plans, total, err
	}
	decisions, err := s.repo.EvaluatePlansForUser(ctx, f.TenantID, userID, plans, s.maxQueue)
	if err != nil {
		return nil, 0, err
	}
	for i := range plans {
		decision := decisions[plans[i].ID]
		plans[i].PurchaseEligibility = &decision
	}
	return plans, total, nil
}

func (s *Service) ListSubscriptions(ctx context.Context, f SubFilter) ([]Subscription, int64, error) {
	f.Limit, f.Offset = clampPage(f.Limit, f.Offset)
	return s.repo.ListSubscriptions(ctx, f)
}

func (s *Service) ListOrders(ctx context.Context, f OrderFilter) ([]Order, int64, error) {
	f.Limit, f.Offset = clampPage(f.Limit, f.Offset)
	return s.repo.ListOrders(ctx, f)
}

func (s *Service) GetOrder(ctx context.Context, id string) (*Order, error) {
	return s.repo.GetOrder(ctx, id)
}

// GroupNames 批量解析分组名（订阅快照只存 group_id，transport 展示层补名）。
func (s *Service) GroupNames(ctx context.Context, ids []string) (map[string]string, error) {
	return s.repo.GroupNames(ctx, ids)
}

// ---- 套餐管理（租户）----

func (s *Service) CreatePlan(ctx context.Context, p CreatePlanParams) (*Plan, error) {
	p.PurchasePolicy = NormalizePurchasePolicy(p.PurchasePolicy)
	if err := ValidatePurchasePolicy(p.PurchasePolicy); err != nil {
		return nil, err
	}
	if err := validatePlanShape(p.DurationDays, p.PriceCredits, p.TotalLimitMicro, p.Window5hLimitMicro, p.Window7dLimitMicro); err != nil {
		return nil, err
	}
	if p.SaleLimit != nil && *p.SaleLimit < 1 {
		return nil, ErrPlanSaleLimitInvalid
	}
	if err := validatePlanGroups(p.Groups); err != nil {
		return nil, err
	}
	if err := s.ensureGroupsVisible(ctx, p.TenantID, p.Groups); err != nil {
		return nil, err
	}
	plan, err := s.repo.CreatePlan(ctx, p)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func (s *Service) UpdatePlan(ctx context.Context, p UpdatePlanParams) (bool, error) {
	if p.PurchasePolicy != nil {
		policy := NormalizePurchasePolicy(*p.PurchasePolicy)
		if err := ValidatePurchasePolicy(policy); err != nil {
			return false, err
		}
		p.PurchasePolicy = &policy
	}
	if err := validatePlanShape(p.DurationDays, p.PriceCredits, p.TotalLimitMicro, p.Window5hLimitMicro, p.Window7dLimitMicro); err != nil {
		return false, err
	}
	if p.SaleLimit != nil && *p.SaleLimit < 1 {
		return false, ErrPlanSaleLimitInvalid
	}
	if err := validatePlanGroups(p.Groups); err != nil {
		return false, err
	}
	if err := s.ensureGroupsVisible(ctx, p.TenantID, p.Groups); err != nil {
		return false, err
	}
	release, err := s.repo.AcquirePlanLock(ctx, p.ID)
	if err != nil {
		return false, err
	}
	defer release()
	current, err := s.repo.GetPlan(ctx, p.ID)
	if err != nil {
		return false, err
	}
	if current.TenantID != p.TenantID {
		return false, nil
	}
	if p.SaleLimit != nil && *p.SaleLimit < current.SoldCount+current.ReservedCount {
		return false, ErrPlanSaleLimitInvalid
	}
	p.SortOrder = current.SortOrder
	return s.repo.UpdatePlanByTenant(ctx, p)
}

// ReorderPlans replaces user-facing plan order in one operation. Duplicate or
// foreign IDs are rejected so partial client state cannot silently corrupt it.
func (s *Service) ReorderPlans(ctx context.Context, tenantID string, planIDs []string) error {
	if tenantID == "" || len(planIDs) == 0 || len(planIDs) > 2000 {
		return ErrPlanReorderInvalid
	}
	seen := make(map[string]struct{}, len(planIDs))
	for _, id := range planIDs {
		if id == "" {
			return ErrPlanReorderInvalid
		}
		if _, exists := seen[id]; exists {
			return ErrPlanReorderInvalid
		}
		seen[id] = struct{}{}
	}
	return s.repo.ReorderPlansByTenant(ctx, tenantID, planIDs)
}

// SetPlanStatus 租户上下架：目标态只能是 draft/on_sale/off_sale。上架要求套餐已绑 ≥1 分组。
func (s *Service) SetPlanStatus(ctx context.Context, id, tenantID, status string) (bool, error) {
	switch status {
	case PlanDraft, PlanOnSale, PlanOffSale:
	default:
		return false, ErrInvalidStatus
	}
	release, err := s.repo.AcquirePlanLock(ctx, id)
	if err != nil {
		return false, err
	}
	defer release()
	if status == PlanOnSale {
		plan, err := s.repo.GetPlan(ctx, id)
		if err != nil {
			return false, err
		}
		if plan.TenantID != tenantID {
			return false, nil // 非本租户 ⇒ 视为未命中（transport 映射 404）
		}
		if len(plan.Groups) == 0 {
			return false, ErrPlanNeedsGroups
		}
		if err := s.ensureGroupsVisible(ctx, tenantID, plan.Groups); err != nil {
			return false, err
		}
	}
	return s.repo.SetPlanStatusByTenant(ctx, id, tenantID, status)
}

func (s *Service) GetPlan(ctx context.Context, id string) (*Plan, error) {
	return s.repo.GetPlan(ctx, id)
}

func (s *Service) ListPurchasePolicyRevisions(ctx context.Context, planID string) ([]PurchasePolicyRevision, error) {
	return s.repo.ListPurchasePolicyRevisions(ctx, planID)
}

// ---- 校验/分页 helper ----

// validatePlanGroups 校验套餐分组绑定形状：≥1 个、扣额倍率>0、组内不重复。
func validatePlanGroups(groups []PlanGroup) error {
	if len(groups) == 0 {
		return ErrPlanNeedsGroups
	}
	seen := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		if g.GroupID == "" || g.QuotaDebitMultiplier <= 0 {
			return ErrPlanGroupsInvalid
		}
		if _, dup := seen[g.GroupID]; dup {
			return ErrPlanGroupsInvalid
		}
		seen[g.GroupID] = struct{}{}
	}
	return nil
}

// ensureGroupsVisible 校验每个绑定分组对该租户 active 且可见（public ∪ 租户绑定）。
func (s *Service) ensureGroupsVisible(ctx context.Context, tenantID string, groups []PlanGroup) error {
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.GroupID
	}
	valid, err := s.repo.ValidateGroupsForTenant(ctx, tenantID, ids)
	if err != nil {
		return err
	}
	if len(valid) != len(ids) { // 有分组不存在/未激活/对租户不可见
		return ErrPlanGroupsInvalid
	}
	return nil
}

func validatePlanShape(durationDays int32, price, totalLimit int64, w5h, w7d *int64) error {
	switch durationDays {
	case 1, 3, 7, 30:
	default:
		return ErrInvalidDuration
	}
	if price <= 0 || price > domain.MaxWholeCredits || totalLimit <= 0 {
		return ErrPlanQuotaInvalid
	}
	if (w5h != nil && (*w5h <= 0 || *w5h > totalLimit)) || (w7d != nil && (*w7d <= 0 || *w7d > totalLimit)) {
		return ErrPlanQuotaInvalid
	}
	if durationDays < 7 && w7d != nil {
		return ErrPlanWindow7dInvalid
	}
	return nil
}

func clampPage(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

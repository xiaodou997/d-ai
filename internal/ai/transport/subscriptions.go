package transport

// subscriptions.go holds the shared DTOs, view converters and error mapping for
// the AI subscription-plan endpoints. The three auth groups live in
// subscriptions_self.go (endUserAuth) and subscriptions_tenant.go
// (tenantUserAuth). See docs/ai-subscription-design.md §7.
//
// 额度单位一律为微积分（micro，1 积分 = 10000 微积分），API 直接透出 *_micro，前端
// 除以 10000 展示为积分（写入无精度损失）；价格为整数积分。

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/libs/go/httpx"
)

// 窗口长度（与 DebitSubscription / subscription.window* 对齐，仅用于展示层计算重置时刻）。
const (
	subWindow5h = 5 * time.Hour
	subWindow7d = 7 * 24 * time.Hour
)

// errInsufficientBalance 是本包的 402 模板（httpx 未预置 402）。
var errInsufficientBalance = httpx.New("insufficient_balance", http.StatusPaymentRequired, "Payment Required")

// errPlanNotAccessible 是购买时用户与套餐分组无交集的 409 模板。
var errPlanNotAccessible = httpx.New("plan_not_accessible", http.StatusConflict, "Conflict")

// mapSubscriptionError 把订阅领域错误映射为 HTTP 语义。ErrOrderProcessing 不在此处理
// （购买端点用 202 表达），落到 default 走通用映射。
func mapSubscriptionError(err error) error {
	var denied *subscription.PurchaseDeniedError
	if errors.As(err, &denied) {
		return purchaseDeniedAppError(denied.Decision).WithCause(err)
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, subscription.ErrInsufficientBalance):
		return errInsufficientBalance.WithDetail("用户积分不足，无法购买该套餐").WithCause(err)
	case errors.Is(err, subscription.ErrQueueFull):
		return httpx.ErrConflict.WithDetail("订阅排队已满（最多 1 个生效 + 2 个排队）").WithCause(err)
	case errors.Is(err, subscription.ErrPlanAlreadyQueued):
		return httpx.ErrConflict.WithDetail("该套餐已在待激活队列中，无需重复购买").WithCause(err)
	case errors.Is(err, subscription.ErrPlanNotOnSale):
		return httpx.ErrConflict.WithDetail("套餐未在售").WithCause(err)
	case errors.Is(err, subscription.ErrPlanForbidden):
		return httpx.ErrForbidden.WithDetail("套餐不属于当前租户").WithCause(err)
	case errors.Is(err, subscription.ErrPlanNotFound):
		return httpx.ErrNotFound.WithDetail("套餐不存在").WithCause(err)
	case errors.Is(err, subscription.ErrSubNotFound):
		return httpx.ErrNotFound.WithDetail("订阅不存在").WithCause(err)
	case errors.Is(err, subscription.ErrOrderNotFound):
		return httpx.ErrNotFound.WithDetail("订单不存在").WithCause(err)
	case errors.Is(err, subscription.ErrIdempotencyConflict):
		return httpx.ErrConflict.WithDetail("幂等键已用于另一笔订阅购买").WithCause(err)
	case errors.Is(err, subscription.ErrInvalidStatus):
		return httpx.ErrBadRequest.WithDetail("非法的状态流转").WithCause(err)
	case errors.Is(err, subscription.ErrInvalidDuration):
		return httpx.ErrBadRequest.WithDetail("有效期只能是 1/3/7/30 天").WithCause(err)
	case errors.Is(err, subscription.ErrPlanQuotaInvalid):
		return httpx.ErrValidation.
			WithDetail("售价或额度超出支持范围").
			WithFields(
				httpx.FieldError{Field: "price_credits", Message: "must be between 1 and 922337203685477"},
				httpx.FieldError{Field: "total_limit_micro", Message: "must be greater than 0"},
				httpx.FieldError{Field: "window_5h_limit_micro", Message: "must be positive and no greater than total_limit_micro"},
				httpx.FieldError{Field: "window_7d_limit_micro", Message: "must be positive and no greater than total_limit_micro"},
			).
			WithCause(err)
	case errors.Is(err, subscription.ErrPlanWindow7dInvalid):
		return httpx.ErrValidation.
			WithDetail("7 天窗口额度仅适用于 7 天及以上套餐").
			WithFields(
				httpx.FieldError{Field: "duration_days", Message: "must be at least 7 when window_7d_limit_micro is set"},
				httpx.FieldError{Field: "window_7d_limit_micro", Message: "only valid when duration_days >= 7"},
			).
			WithCause(err)
	case errors.Is(err, subscription.ErrPlanNeedsGroups):
		return httpx.ErrValidation.WithDetail("套餐必须绑定至少一个分组").WithCause(err)
	case errors.Is(err, subscription.ErrPlanGroupsInvalid):
		return httpx.ErrValidation.WithDetail("绑定分组含非法项（不存在/未激活/不属于当前租户/套餐扣额倍率需 > 0）").WithCause(err)
	case errors.Is(err, subscription.ErrPurchasePolicyInvalid):
		return httpx.ErrValidation.WithDetail("购买限制配置不完整或相互冲突").WithCause(err)
	case errors.Is(err, subscription.ErrPlanNotAccessible):
		return errPlanNotAccessible.WithDetail("你没有可访问该套餐分组的权限，无法购买").WithCause(err)
	case errors.Is(err, subscription.ErrPlanSoldOut):
		return httpx.ErrConflict.WithDetail("套餐已售罄").WithCause(err)
	case errors.Is(err, subscription.ErrPlanSaleLimitInvalid):
		return httpx.ErrValidation.WithDetail("销售数量必须大于 0，且不能低于已售和待支付数量").WithCause(err)
	case errors.Is(err, subscription.ErrPlanReorderInvalid):
		return httpx.ErrValidation.WithDetail("套餐排序数据无效或包含不属于当前租户的套餐").WithCause(err)
	default:
		return mapServiceError(err)
	}
}

func purchaseDeniedAppError(decision subscription.PurchaseDecision) *httpx.AppError {
	detail := map[subscription.PurchaseBlockReason]string{
		subscription.PurchaseOrderProcessing:      "该套餐已有购买订单正在处理",
		subscription.PurchasePlanAlreadyQueued:    "该套餐已在待激活队列中",
		subscription.PurchaseQueueFull:            "订阅排队已满（最多 1 个生效 + 2 个排队）",
		subscription.PurchaseAdvanceNotAllowed:    "当前套餐权益结束前不允许提前购买",
		subscription.PurchaseLifetimeLimitReached: "已达到该套餐的累计购买上限",
		subscription.PurchaseRollingLimitReached:  "尚未到该套餐的下一次可购买时间",
		subscription.PurchaseCalendarLimitReached: "已达到当前自然周期的购买上限",
	}[decision.PrimaryReason]
	if detail == "" {
		detail = "当前不满足套餐购买条件"
	}
	return httpx.New(string(decision.PrimaryReason), http.StatusConflict, "Conflict").
		WithDetail(detail).
		WithMeta(purchaseDecisionMeta(decision))
}

func purchaseDecisionMeta(decision subscription.PurchaseDecision) map[string]any {
	rules := make([]map[string]any, 0, len(decision.BlockingRules))
	for _, rule := range decision.BlockingRules {
		item := map[string]any{"reason": rule.Reason, "used": rule.Used}
		if rule.RetryAt != nil {
			item["retry_at"] = rule.RetryAt
		}
		if rule.Limit != nil {
			item["limit"] = *rule.Limit
		}
		rules = append(rules, item)
	}
	meta := map[string]any{"blocking_rules": rules}
	if decision.RetryAt != nil {
		meta["retry_at"] = decision.RetryAt
	}
	return meta
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// subPlanGroupDTO 套餐绑定的分组（含套餐扣额倍率）。
type subPlanGroupDTO struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	QuotaDebitMultiplier float64 `json:"quota_debit_multiplier"`
}

type subPurchasePolicyDTO struct {
	LifetimeMaxPurchases *int32 `json:"lifetime_max_purchases,omitempty"`
	PeriodType           string `json:"period_type" enum:"none,rolling,calendar"`
	PeriodMaxPurchases   *int32 `json:"period_max_purchases,omitempty"`
	RollingWindowHours   *int32 `json:"rolling_window_hours,omitempty"`
	CalendarUnit         string `json:"calendar_unit,omitempty" enum:"day,week,month"`
	CalendarTimezone     string `json:"calendar_timezone,omitempty"`
	AllowAdvancePurchase bool   `json:"allow_advance_purchase"`
	Version              int64  `json:"version"`
}

func purchasePolicyToDTO(p subscription.PurchasePolicy) subPurchasePolicyDTO {
	return subPurchasePolicyDTO{
		LifetimeMaxPurchases: p.LifetimeMaxPurchases,
		PeriodType:           p.PeriodType,
		PeriodMaxPurchases:   p.PeriodMaxPurchases,
		RollingWindowHours:   p.RollingWindowHours,
		CalendarUnit:         p.CalendarUnit,
		CalendarTimezone:     p.CalendarTimezone,
		AllowAdvancePurchase: p.AllowAdvancePurchase,
		Version:              p.Version,
	}
}

type subPurchaseRuleDecisionDTO struct {
	Reason  string     `json:"reason"`
	RetryAt *time.Time `json:"retry_at,omitempty"`
	Limit   *int32     `json:"limit,omitempty"`
	Used    int32      `json:"used"`
}

type subPurchaseEligibilityDTO struct {
	Allowed       bool                         `json:"allowed"`
	PrimaryReason string                       `json:"primary_reason,omitempty"`
	BlockingRules []subPurchaseRuleDecisionDTO `json:"blocking_rules"`
	RetryAt       *time.Time                   `json:"retry_at,omitempty"`
}

type subPurchasePolicyRevisionDTO struct {
	PlanID    string               `json:"plan_id"`
	Version   int64                `json:"version"`
	Policy    subPurchasePolicyDTO `json:"policy"`
	ChangedBy string               `json:"changed_by,omitempty"`
	ChangedAt time.Time            `json:"changed_at"`
}

func purchasePolicyRevisionToDTO(revision subscription.PurchasePolicyRevision) subPurchasePolicyRevisionDTO {
	return subPurchasePolicyRevisionDTO{
		PlanID: revision.PlanID, Version: revision.Version,
		Policy: purchasePolicyToDTO(revision.Policy), ChangedBy: revision.ChangedBy,
		ChangedAt: revision.ChangedAt,
	}
}

func purchaseEligibilityToDTO(decision *subscription.PurchaseDecision) *subPurchaseEligibilityDTO {
	if decision == nil {
		return nil
	}
	rules := make([]subPurchaseRuleDecisionDTO, 0, len(decision.BlockingRules))
	for _, rule := range decision.BlockingRules {
		rules = append(rules, subPurchaseRuleDecisionDTO{
			Reason: string(rule.Reason), RetryAt: rule.RetryAt, Limit: rule.Limit, Used: rule.Used,
		})
	}
	return &subPurchaseEligibilityDTO{
		Allowed: decision.Allowed, PrimaryReason: string(decision.PrimaryReason),
		BlockingRules: rules, RetryAt: decision.RetryAt,
	}
}

type subPlanDTO struct {
	ID                 string               `json:"id"`
	TenantID           string               `json:"tenant_id"`
	Name               string               `json:"name"`
	Description        string               `json:"description"`
	PriceCredits       int64                `json:"price_credits"`
	DurationDays       int32                `json:"duration_days"`
	TotalLimitMicro    int64                `json:"total_limit_micro"`
	Window5hLimitMicro *int64               `json:"window_5h_limit_micro,omitempty"`
	Window7dLimitMicro *int64               `json:"window_7d_limit_micro,omitempty"`
	Status             string               `json:"status"`
	SortOrder          int32                `json:"sort_order"`
	SaleLimit          *int32               `json:"sale_limit,omitempty"`
	SoldCount          int32                `json:"sold_count"`
	ReservedCount      int32                `json:"reserved_count"`
	AvailableCount     *int32               `json:"available_count,omitempty"`
	SoldOut            bool                 `json:"sold_out"`
	Groups             []subPlanGroupDTO    `json:"groups"`
	PurchasePolicy     subPurchasePolicyDTO `json:"purchase_policy"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

func subPlanToDTO(p subscription.Plan) subPlanDTO {
	groups := make([]subPlanGroupDTO, 0, len(p.Groups))
	for _, g := range p.Groups {
		groups = append(groups, subPlanGroupDTO{ID: g.GroupID, Name: g.Name, QuotaDebitMultiplier: g.QuotaDebitMultiplier})
	}
	return subPlanDTO{
		ID:                 p.ID,
		TenantID:           p.TenantID,
		Name:               p.Name,
		Description:        p.Description,
		PriceCredits:       p.PriceCredits,
		DurationDays:       p.DurationDays,
		TotalLimitMicro:    p.TotalLimitMicro,
		Window5hLimitMicro: p.Window5hLimitMicro,
		Window7dLimitMicro: p.Window7dLimitMicro,
		Status:             p.Status,
		SortOrder:          p.SortOrder,
		SaleLimit:          p.SaleLimit,
		SoldCount:          p.SoldCount,
		ReservedCount:      p.ReservedCount,
		AvailableCount:     planAvailableCount(p),
		SoldOut:            planSoldOut(p),
		Groups:             groups,
		PurchasePolicy:     purchasePolicyToDTO(p.PurchasePolicy),
		CreatedAt:          p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
	}
}

// subPublicPlanDTO omits tenant pricing internals from the customer marketplace.
type subPublicPlanDTO struct {
	ID                  string                     `json:"id"`
	Name                string                     `json:"name"`
	Description         string                     `json:"description"`
	PriceCredits        int64                      `json:"price_credits"`
	DurationDays        int32                      `json:"duration_days"`
	TotalLimitMicro     int64                      `json:"total_limit_micro"`
	Window5hLimitMicro  *int64                     `json:"window_5h_limit_micro,omitempty"`
	Window7dLimitMicro  *int64                     `json:"window_7d_limit_micro,omitempty"`
	SaleLimit           *int32                     `json:"sale_limit,omitempty"`
	SoldCount           int32                      `json:"sold_count"`
	AvailableCount      *int32                     `json:"available_count,omitempty"`
	SoldOut             bool                       `json:"sold_out"`
	Groups              []subPlanGroupDTO          `json:"groups"`
	PurchasePolicy      subPurchasePolicyDTO       `json:"purchase_policy"`
	PurchaseEligibility *subPurchaseEligibilityDTO `json:"purchase_eligibility,omitempty"`
}

func subPlanToPublicDTO(p subscription.Plan) subPublicPlanDTO {
	private := subPlanToDTO(p)
	return subPublicPlanDTO{
		ID: p.ID, Name: p.Name, Description: p.Description,
		PriceCredits: p.PriceCredits, DurationDays: p.DurationDays,
		TotalLimitMicro: p.TotalLimitMicro, Window5hLimitMicro: p.Window5hLimitMicro,
		Window7dLimitMicro: p.Window7dLimitMicro, Groups: private.Groups,
		SaleLimit: p.SaleLimit, SoldCount: p.SoldCount,
		AvailableCount: planAvailableCount(p), SoldOut: planSoldOut(p),
		PurchasePolicy:      private.PurchasePolicy,
		PurchaseEligibility: purchaseEligibilityToDTO(p.PurchaseEligibility),
	}
}

func planAvailableCount(p subscription.Plan) *int32 {
	if p.SaleLimit == nil {
		return nil
	}
	remaining := *p.SaleLimit - p.SoldCount - p.ReservedCount
	if remaining < 0 {
		remaining = 0
	}
	return &remaining
}

func planSoldOut(p subscription.Plan) bool {
	remaining := planAvailableCount(p)
	return remaining != nil && *remaining == 0
}

// subWindowDTO 单个时间窗的展示态（后端用服务器时间算好余量与重置时刻，避免前端时钟误差）。
type subWindowDTO struct {
	LimitMicro     *int64     `json:"limit_micro,omitempty"`     // nil = 该窗口不限
	UsedMicro      int64      `json:"used_micro"`                // 当前窗口已用（翻转后展示 0）
	RemainingMicro *int64     `json:"remaining_micro,omitempty"` // nil = 不限
	ResetAt        *time.Time `json:"reset_at,omitempty"`        // 当前窗口重置时刻；未开窗/不限为空
}

// computeWindow 用服务器时间算某窗口的展示态，与 service.windowHasRemaining 同源逻辑。
func computeWindow(limit *int64, start *time.Time, used int64, dur time.Duration, now time.Time) subWindowDTO {
	if limit == nil {
		return subWindowDTO{UsedMicro: used}
	}
	if start != nil && now.Before(start.Add(dur)) {
		rem := *limit - used
		if rem < 0 {
			rem = 0
		}
		reset := start.Add(dur)
		return subWindowDTO{LimitMicro: limit, UsedMicro: used, RemainingMicro: &rem, ResetAt: &reset}
	}
	// 未开窗或已翻转：展示满额、used 归 0。
	full := *limit
	return subWindowDTO{LimitMicro: limit, UsedMicro: 0, RemainingMicro: &full}
}

// subSubGroupDTO 覆盖订阅的分组套餐扣额倍率快照条目（名字由 groupNames 补，缺则为空）。
type subSubGroupDTO struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	QuotaDebitMultiplier float64 `json:"quota_debit_multiplier"`
}

type subscriptionDTO struct {
	ID              string           `json:"id"`
	TenantID        string           `json:"tenant_id"`
	UserID          string           `json:"user_id"`
	PlanID          string           `json:"plan_id"`
	OrderID         string           `json:"order_id"`
	PlanName        string           `json:"plan_name"`
	DurationDays    int32            `json:"duration_days"`
	Status          string           `json:"status"`
	ActivatedAt     *time.Time       `json:"activated_at,omitempty"`
	ExpiresAt       *time.Time       `json:"expires_at,omitempty"`
	TotalLimitMicro int64            `json:"total_limit_micro"`
	TotalUsedMicro  int64            `json:"total_used_micro"`
	TotalRemaining  int64            `json:"total_remaining_micro"`
	Window5h        subWindowDTO     `json:"window_5h"`
	Window7d        subWindowDTO     `json:"window_7d"`
	Groups          []subSubGroupDTO `json:"groups"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

func subscriptionToDTO(s subscription.Subscription, now time.Time, groupNames map[string]string) subscriptionDTO {
	totalRem := s.TotalLimitMicro - s.TotalUsedMicro
	if totalRem < 0 {
		totalRem = 0
	}
	groups := make([]subSubGroupDTO, 0, len(s.GroupQuotaDebitMultipliers))
	for gid, w := range s.GroupQuotaDebitMultipliers {
		groups = append(groups, subSubGroupDTO{ID: gid, Name: groupNames[gid], QuotaDebitMultiplier: w})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return subscriptionDTO{
		ID:              s.ID,
		TenantID:        s.TenantID,
		UserID:          s.UserID,
		PlanID:          s.PlanID,
		OrderID:         s.OrderID,
		PlanName:        s.PlanNameSnapshot,
		DurationDays:    s.DurationDays,
		Status:          s.Status,
		ActivatedAt:     s.ActivatedAt,
		ExpiresAt:       s.ExpiresAt,
		TotalLimitMicro: s.TotalLimitMicro,
		TotalUsedMicro:  s.TotalUsedMicro,
		TotalRemaining:  totalRem,
		Window5h:        computeWindow(s.Window5hLimitMicro, s.Win5hStart, s.Win5hUsedMicro, subWindow5h, now),
		Window7d:        computeWindow(s.Window7dLimitMicro, s.Win7dStart, s.Win7dUsedMicro, subWindow7d, now),
		Groups:          groups,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
}

// subscriptionsToDTO 批量转换：先汇总所有快照分组 id 一次解析名字，再逐条建 DTO。
func subscriptionsToDTO(ctx context.Context, d AIDeps, subs []subscription.Subscription, now time.Time) []subscriptionDTO {
	idset := make(map[string]struct{})
	for i := range subs {
		for gid := range subs[i].GroupQuotaDebitMultipliers {
			idset[gid] = struct{}{}
		}
	}
	names := resolveGroupNames(ctx, d, idset)
	items := make([]subscriptionDTO, 0, len(subs))
	for i := range subs {
		items = append(items, subscriptionToDTO(subs[i], now, names))
	}
	return items
}

// resolveGroupNames 批量解析分组名（空集或服务缺失时返回空 map，不阻断）。
func resolveGroupNames(ctx context.Context, d AIDeps, idset map[string]struct{}) map[string]string {
	if len(idset) == 0 || d.Subscriptions == nil {
		return map[string]string{}
	}
	ids := make([]string, 0, len(idset))
	for id := range idset {
		ids = append(ids, id)
	}
	names, err := d.Subscriptions.GroupNames(ctx, ids)
	if err != nil {
		return map[string]string{}
	}
	return names
}

type subOrderDTO struct {
	ID                    string               `json:"id"`
	OrderNo               string               `json:"order_no"`
	TenantID              string               `json:"tenant_id"`
	UserID                string               `json:"user_id"`
	PlanID                string               `json:"plan_id"`
	PlanName              string               `json:"plan_name"`
	PriceCredits          int64                `json:"price_credits"`
	Status                string               `json:"status"`
	BillingEventID        string               `json:"billing_event_id,omitempty"`
	SubscriptionID        string               `json:"subscription_id,omitempty"`
	FailReason            string               `json:"fail_reason,omitempty"`
	PurchasePolicyVersion int64                `json:"purchase_policy_version"`
	PurchasePolicy        subPurchasePolicyDTO `json:"purchase_policy"`
	PaidAt                *time.Time           `json:"paid_at,omitempty"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
}

func subOrderToDTO(o subscription.Order) subOrderDTO {
	return subOrderDTO{
		ID:                    o.ID,
		OrderNo:               o.OrderNo,
		TenantID:              o.TenantID,
		UserID:                o.UserID,
		PlanID:                o.PlanID,
		PlanName:              o.PlanNameSnapshot,
		PriceCredits:          o.PriceCredits,
		Status:                o.Status,
		BillingEventID:        o.BillingEventID,
		SubscriptionID:        o.SubscriptionID,
		FailReason:            o.FailReason,
		PurchasePolicyVersion: o.PurchasePolicyVersion,
		PurchasePolicy:        purchasePolicyToDTO(o.PurchasePolicySnapshot),
		PaidAt:                o.PaidAt,
		CreatedAt:             o.CreatedAt,
		UpdatedAt:             o.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// list outputs (huma needs concrete types)
// ---------------------------------------------------------------------------

type ControlPage[T any] struct {
	Items    []T                 `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	Size     int                 `json:"size"`
	Included IdentityIncludedDTO `json:"included"`
}

func NewControlPage[T any](items []T, total int64, page, size int, included IdentityIncludedDTO) ControlPage[T] {
	paged := httpx.NewPage(items, total, page, size)
	return ControlPage[T]{
		Items:    paged.Items,
		Total:    paged.Total,
		Page:     paged.Page,
		Size:     paged.Size,
		Included: included,
	}
}

type subPlanListOutput struct {
	Body ControlPage[subPlanDTO]
}

type subPublicPlanListOutput struct {
	Body ControlPage[subPublicPlanDTO]
}

type subscriptionListOutput struct {
	Body ControlPage[subscriptionDTO]
}

type subOrderListOutput struct {
	Body ControlPage[subOrderDTO]
}

type subPlanOutput struct {
	Body subPlanDTO
}

type subscriptionOutput struct {
	Body subscriptionDTO
}

type subscriptionNullableOutput struct {
	Body *subscriptionDTO
}

type subOrderOutput struct {
	Body subOrderDTO
}

type subPurchasePolicyRevisionListBody struct {
	Items []subPurchasePolicyRevisionDTO `json:"items"`
}

type subPurchasePolicyRevisionListOutput struct {
	Body subPurchasePolicyRevisionListBody
}

// pageParams normalises limit/offset into a (page, size) pair for httpx.Page.
func subPageMeta(limit, offset int32) (page, size int) {
	if limit <= 0 {
		limit = 20
	}
	size = int(limit)
	page = int(offset)/size + 1
	return page, size
}

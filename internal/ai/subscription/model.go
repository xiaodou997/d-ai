// Package subscription 实现 AI 订阅制套餐领域层：套餐定义、订阅实例、购买订单、
// 热路径额度判定与记账、后台卫生 janitor。方案见 docs/ai-subscription-design.md。
//
// 价格与额度单位统一为 micro-USD。套餐额度按零售基准价 × 命中分组套餐扣额倍率计量，
// 用户倍率不参与套餐计量。套餐必须绑定 ≥1 个分组，订阅覆盖期
// 硬限制路由到套餐分组交集。二期重构见 docs/ai-subscription-group-refactor.md。
package subscription

import (
	"context"
	"errors"
	"time"
)

// 计费来源常量（与 ai_usage_logs.billing_source / pipeline.Request 对齐）。
const (
	BillingSourcePayg         = "payg"
	BillingSourceSubscription = "subscription"
)

// 套餐 / 订阅 / 订单状态。
const (
	PlanDraft   = "draft"
	PlanOnSale  = "on_sale"
	PlanOffSale = "off_sale"

	SubPending   = "pending"
	SubActive    = "active"
	SubExpired   = "expired"
	SubCancelled = "cancelled"

	OrderCreated   = "created"
	OrderDeducting = "deducting"
	OrderPaid      = "paid"
	OrderFailed    = "failed"
)

// 领域错误。
var (
	ErrPlanNotFound  = errors.New("subscription: plan not found")
	ErrPlanNotOnSale = errors.New("subscription: plan not on sale")
	ErrPlanForbidden = errors.New("subscription: plan not owned by tenant")
	ErrQueueFull     = errors.New("subscription: subscription queue is full")
	// ErrPlanAlreadyQueued：待激活队列（pending 订阅 + 在途订单）里已有同一套餐；active 同套餐不拦（预购续期）。
	ErrPlanAlreadyQueued = errors.New("subscription: plan already queued")
	// ErrInsufficientBalance：购买时用户 USD 余额不足。
	ErrInsufficientBalance = errors.New("subscription: insufficient balance")
	// ErrOrderProcessing：扣款处于未知态，订单停在 deducting，交由 janitor 补偿。
	ErrOrderProcessing     = errors.New("subscription: order still processing")
	ErrOrderNotFound       = errors.New("subscription: order not found")
	ErrIdempotencyConflict = errors.New("subscription: idempotency key belongs to another purchase")
	ErrInvalidStatus       = errors.New("subscription: invalid status transition")
	ErrSubNotFound         = errors.New("subscription: subscription not found")
	ErrInvalidDuration     = errors.New("subscription: duration_days must be 1/3/7/30")
	// ErrPlanQuotaInvalid：套餐售价或总额度超出支持范围。
	ErrPlanQuotaInvalid = errors.New("subscription: price_micro_usd or total_limit_micro_usd is outside the supported range")
	// ErrPlanWindow7dInvalid：7 天窗口额度仅适用于 ≥7 天套餐。
	ErrPlanWindow7dInvalid = errors.New("subscription: window_7d_limit only valid for duration >= 7")
	// ErrPlanNeedsGroups：套餐未绑定任何分组（创建/编辑/上架都要求 ≥1 个）。
	ErrPlanNeedsGroups = errors.New("subscription: plan must bind at least one group")
	// ErrPlanGroupsInvalid：绑定分组含非法项（不存在/未激活/不属于租户/扣额倍率 <=0）。
	ErrPlanGroupsInvalid = errors.New("subscription: plan groups invalid (inactive, not owned by tenant, or bad debit multiplier)")
	// ErrPlanNotAccessible：用户可见分组与套餐分组无交集，无法购买。
	ErrPlanNotAccessible = errors.New("subscription: plan groups not accessible to user")
	// ErrPlanSoldOut：套餐仍处于上架状态，但限量份数已全部售出或被在途订单预占。
	ErrPlanSoldOut = errors.New("subscription: plan sold out")
	// ErrPlanSaleLimitInvalid：销售上限必须为正数，且不可低于已售与在途预占之和。
	ErrPlanSaleLimitInvalid = errors.New("subscription: sale limit is below committed inventory")
	ErrPlanReorderInvalid   = errors.New("subscription: reorder plan ids are invalid")
)

// PlanGroup 套餐-分组绑定。QuotaDebitMultiplier 为套餐扣额倍率（额度消耗 = 基准价 × QuotaDebitMultiplier）；
// 与按量计费倍率无关。Name 只在读出时填充供展示，写入时忽略。
type PlanGroup struct {
	GroupID              string
	Name                 string
	QuotaDebitMultiplier float64
	SortOrder            int32
}

// Plan 套餐定义。Window*LimitMicro 为 nil 表示该窗口不限（只受总限额约束）。
type Plan struct {
	ID                 string
	TenantID           string
	Name               string
	Description        string
	PriceMicroUSD      int64
	DurationDays       int32
	TotalLimitMicro    int64
	Window5hLimitMicro *int64
	Window7dLimitMicro *int64
	Status             string
	SortOrder          int32
	SaleLimit          *int32
	SoldCount          int32
	ReservedCount      int32
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	// Groups 套餐绑定的分组（含套餐扣额倍率）；读出时填充，写入走 CreatePlanParams/UpdatePlanParams。
	Groups []PlanGroup
	// PurchasePolicy controls per-user repeat purchases. Eligibility is evaluated
	// against the current policy and the complete paid-order history for this plan.
	PurchasePolicy PurchasePolicy
	// PurchaseEligibility is populated only for user-aware storefront reads.
	PurchaseEligibility *PurchaseDecision
}

// Order 购买订单（一致性锚点）。
type Order struct {
	ID                                 string
	OrderNo                            string
	TenantID                           string
	UserID                             string
	PlanID                             string
	PlanNameSnapshot                   string
	PriceMicroUSD                      int64
	DurationDaysSnapshot               int32
	TotalLimitMicroSnapshot            int64
	Window5hLimitMicroSnapshot         *int64
	Window7dLimitMicroSnapshot         *int64
	GroupQuotaDebitMultipliersSnapshot map[string]float64
	PurchasePolicyVersion              int64
	PurchasePolicySnapshot             PurchasePolicy
	InventoryReserved                  bool
	Status                             string
	DebitReference                     string
	DebitedAt                          *time.Time
	SubscriptionID                     string
	FailReason                         string
	PaidAt                             *time.Time
	CreatedAt                          time.Time
	UpdatedAt                          time.Time
}

// Subscription 订阅实例（含三窗口计数与快照）。
type Subscription struct {
	ID                 string
	TenantID           string
	UserID             string
	PlanID             string
	OrderID            string
	PlanNameSnapshot   string
	DurationDays       int32
	TotalLimitMicro    int64
	Window5hLimitMicro *int64
	Window7dLimitMicro *int64
	Status             string
	ActivatedAt        *time.Time
	ExpiresAt          *time.Time
	TotalUsedMicro     int64
	Win5hStart         *time.Time
	Win5hUsedMicro     int64
	Win7dStart         *time.Time
	Win7dUsedMicro     int64
	// GroupQuotaDebitMultipliers 售出时套餐分组套餐扣额倍率快照 {group_id: quota_debit_multiplier}；空=存量无快照订阅。
	GroupQuotaDebitMultipliers map[string]float64
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

// GateDecision 热路径 gate 判定结果。
type GateDecision struct {
	Covered        bool
	SubscriptionID string
	// GroupQuotaDebitMultipliers 覆盖订阅的分组套餐扣额倍率快照；gate 后写入 req 供路由交集与记账。
	GroupQuotaDebitMultipliers map[string]float64
}

// ---- 参数 ----

// CreatePlanParams 新建套餐（租户）。
type CreatePlanParams struct {
	TenantID           string
	Name               string
	Description        string
	PriceMicroUSD      int64
	DurationDays       int32
	TotalLimitMicro    int64
	Window5hLimitMicro *int64
	Window7dLimitMicro *int64
	SortOrder          int32
	SaleLimit          *int32
	CreatedBy          string
	Groups             []PlanGroup
	PurchasePolicy     PurchasePolicy
}

// UpdatePlanParams 编辑套餐（租户，改动只影响新购）。
type UpdatePlanParams struct {
	ID                 string
	TenantID           string
	Name               string
	Description        string
	PriceMicroUSD      int64
	DurationDays       int32
	TotalLimitMicro    int64
	Window5hLimitMicro *int64
	Window7dLimitMicro *int64
	SortOrder          int32
	SaleLimit          *int32
	Groups             []PlanGroup
	// PurchasePolicy is optional for backwards-compatible updates. Nil keeps the
	// current policy unchanged; a non-nil value replaces it and creates a revision.
	PurchasePolicy *PurchasePolicy
	UpdatedBy      string
}

// PlanFilter 套餐列表筛选。TenantID 为空 = 跨租户（admin 监管）；OnSaleOnly=用户商城。
type PlanFilter struct {
	TenantID   string
	Status     string
	OnSaleOnly bool
	Limit      int32
	Offset     int32
}

// SubFilter 订阅列表筛选。TenantID 为空 = 全局（admin）。
type SubFilter struct {
	TenantID string
	UserID   string
	Status   string
	Limit    int32
	Offset   int32
}

// OrderFilter 订单列表筛选。
type OrderFilter struct {
	TenantID string
	UserID   string
	Status   string
	Limit    int32
	Offset   int32
}

// PurchaseParams 购买请求。
type PurchaseParams struct {
	TenantID       string
	UserID         string
	PlanID         string
	IdempotencyKey string
}

// PurchaseReservation is the atomic result before the billing debit.
// Replayed means the idempotency key already owned the returned order.
type PurchaseReservation struct {
	Order    *Order
	Replayed bool
}

type DebitRequest struct {
	IdempotencyKey string
	TenantID       string
	UserID         string
	Description    string
	TenantMicro    int64
	UserMicro      int64
}

type DebitReceipt struct {
	AuthorizationID string
}

// Purchaser 是订阅购买需要的计费窄接口（strict：不足额整单失败，不许透支）。
type Purchaser interface {
	DebitStrict(ctx context.Context, req DebitRequest) (*DebitReceipt, error)
}

// Repo 是订阅领域的持久化接口，由 adapters/postgres 实现。
type Repo interface {
	// 套餐
	CreatePlan(ctx context.Context, p CreatePlanParams) (*Plan, error)
	GetPlan(ctx context.Context, id string) (*Plan, error)
	UpdatePlanByTenant(ctx context.Context, p UpdatePlanParams) (bool, error)
	SetPlanStatusByTenant(ctx context.Context, id, tenantID, status string) (bool, error)
	ReorderPlansByTenant(ctx context.Context, tenantID string, planIDs []string) error
	ListPlans(ctx context.Context, f PlanFilter) ([]Plan, int64, error)
	ListPurchasePolicyRevisions(ctx context.Context, planID string) ([]PurchasePolicyRevision, error)
	EvaluatePlansForUser(ctx context.Context, tenantID, userID string, plans []Plan, maxQueue int) (map[string]PurchaseDecision, error)
	// AcquirePlanLock 串行化同一套餐的编辑、状态变更与平台禁用。
	AcquirePlanLock(ctx context.Context, planID string) (func(), error)
	// ValidateGroupsForTenant 返回入参分组中「active 且对该租户可见」的子集（比对入参判非法）。
	ValidateGroupsForTenant(ctx context.Context, tenantID string, groupIDs []string) ([]string, error)
	// CountUserAccessiblePlanGroups 返回套餐分组 ∩ 用户可见分组的数量（>0 才可购买）。
	CountUserAccessiblePlanGroups(ctx context.Context, planID, tenantID, userID string) (int64, error)
	// GroupNames 批量取分组名（订阅快照只存 group_id，展示层补名）。
	GroupNames(ctx context.Context, ids []string) (map[string]string, error)

	// 订单
	GetOrder(ctx context.Context, id string) (*Order, error)
	ReservePurchase(ctx context.Context, orderNo string, p PurchaseParams, maxQueue int) (*PurchaseReservation, error)
	MarkOrderDeducting(ctx context.Context, id string) (bool, error)
	MarkOrderFailed(ctx context.Context, id, reason string) (bool, error)
	ListReconcileOrders(ctx context.Context, cutoff time.Time, limit int32) ([]Order, error)
	ListOrders(ctx context.Context, f OrderFilter) ([]Order, int64, error)

	// 订阅
	GetLiveSubs(ctx context.Context, tenantID, userID string) ([]Subscription, error)
	ExpireIfDue(ctx context.Context, id string) (bool, error)
	Activate(ctx context.Context, id string) (bool, error)
	Debit(ctx context.Context, id string, micro int64) (int64, error)
	ExpireDue(ctx context.Context) (int64, error)
	GetSubscription(ctx context.Context, id string) (*Subscription, error)
	ListSubscriptions(ctx context.Context, f SubFilter) ([]Subscription, int64, error)

	// FinalizeOrder 购买 finalize 事务（按用户 advisory 串行）：从订单不可变权益快照建订阅 →
	// 订单置 paid。幂等：订单已 paid 时返回既有订阅。
	FinalizeOrder(ctx context.Context, order *Order, debitReference string) (*Subscription, error)
}

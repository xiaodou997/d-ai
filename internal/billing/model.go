package billing

import (
	"time"

	"xiaodou/dai/internal/domain"
)

// PackageType identifies the owner of a persisted USD balance lot.
const (
	PackageTypeTenant = "tenant"
	PackageTypeUser   = "user"
)

// PackageSource identifies how a USD balance lot was funded.
const (
	PackageSourceAdminRecharge   = "ADMIN_RECHARGE"
	PackageSourceTenantRecharge  = "TENANT_RECHARGE"
	PackageSourceRefund          = "REFUND"
	PackageSourceOnlineTopup     = "ONLINE_TOPUP"
	PackageSourceUserTopupIncome = "USER_TOPUP_INCOME"
)

// PackageStatus is the lifecycle of a USD balance lot.
const (
	PackageStatusAvailable = "available"
	PackageStatusExpired   = "expired"
	PackageStatusDepleted  = "depleted"
	PackageStatusRevoked   = "revoked"
)

// EventStatus 消费事件状态
const (
	EventStatusPending   = "pending"
	EventStatusSucceeded = "succeeded"
	EventStatusCancelled = "cancelled"
	EventStatusReleased  = "released"
	EventStatusRefunded  = "refunded"
)

// EventType 消费事件类型
const (
	EventTypeCharge = "charge"
	EventTypeRefund = "refund"
)

// OrderType 充值订单类型
const (
	OrderTypePlatformToTenant  = "platform_to_tenant"
	OrderTypeTenantToUser      = "tenant_to_user"
	OrderTypeOnlineUserTopup   = "online_user_topup"   // 用户在线充值（微信支付）
	OrderTypeOnlineTenantTopup = "online_tenant_topup" // 租户在线充值（微信支付）
	OrderTypeUserTopupIncome   = "user_topup_income"   // 终端用户充值产生的租户 USD 收入
)

// OnlineOrderTypes 在线充值相关的 order_type（不可撤销、需纳入统计口径）。
var OnlineOrderTypes = []string{OrderTypeOnlineUserTopup, OrderTypeOnlineTenantTopup, OrderTypeUserTopupIncome}

// TenantRechargeOrderTypes 归入"租户充值"统计口径的 order_type。
var TenantRechargeOrderTypes = []string{OrderTypePlatformToTenant, OrderTypeOnlineTenantTopup, OrderTypeUserTopupIncome}

// UserRechargeOrderTypes 归入"用户充值"统计口径的 order_type。
var UserRechargeOrderTypes = []string{OrderTypeTenantToUser, OrderTypeOnlineUserTopup}

// OrderStatus 充值订单状态
const (
	OrderStatusActive   = "active"
	OrderStatusReversed = "reversed"
)

// BillingEvent 消费事件聚合根（对应 bill_events 表）
type BillingEvent struct {
	ID             int64
	EventID        string
	IdempotencyKey string
	TenantID       string
	UserID         string
	ClientID       string
	Description    string
	EventType      string  // charge / refund
	RefundOf       *string // refund 时指向原始 EventID
	TenantCredits  *int64  // legacy DB mapping; value is tenant micro-USD
	UserCredits    *int64  // legacy DB mapping; value is user micro-USD
	Status         string  // pending / succeeded / cancelled / released / refunded
	Metadata       string
	TerminalNote   string
	CreatedAt      time.Time
	FinishedAt     *time.Time
}

// RechargeOrder 充值订单（对应 bill_recharge_orders 表）
type RechargeOrder struct {
	ID             int64
	OrderID        string
	OrderType      string // platform_to_tenant / tenant_to_user
	TenantID       string
	UserID         *string
	CreditAmount   int64
	PaidAmount     int64
	PaymentRef     *string
	ExpiresAt      *time.Time
	OperatorID     string
	Note           *string
	Status         string // active / reversed
	ReversedAt     *time.Time
	ReversedBy     *string
	ReversalReason *string
	CreatedAt      time.Time
}

// NowUTC 获取当前 UTC 时间
func NowUTC() time.Time {
	return time.UnixMilli(domain.NowMillis()).UTC()
}

func (e *BillingEvent) IsPending() bool {
	return e.Status == EventStatusPending
}

func (e *BillingEvent) Confirm() {
	e.Status = EventStatusSucceeded
	now := NowUTC()
	e.FinishedAt = &now
}

func (e *BillingEvent) Cancel() {
	e.Status = EventStatusCancelled
	now := NowUTC()
	e.FinishedAt = &now
}

func (e *BillingEvent) AutoRelease() {
	e.Status = EventStatusReleased
	now := NowUTC()
	e.FinishedAt = &now
}

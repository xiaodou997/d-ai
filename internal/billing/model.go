package billing

import (
	"time"

	"xiaodou/dai/internal/domain"
)

// PackageType 积分包类型
const (
	PackageTypeTenant = "tenant"
	PackageTypeUser   = "user"
)

// PackageSource 积分包来源
const (
	PackageSourceAdminRecharge  = "ADMIN_RECHARGE"
	PackageSourceTenantRecharge = "TENANT_RECHARGE"
	PackageSourceRefund         = "REFUND"
	PackageSourceOnlineTopup    = "ONLINE_TOPUP"
	PackageSourceCashPurchase   = "CASH_PURCHASE"
)

// PackageStatus 积分包状态
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
	OrderTypeCashPurchase      = "cash_purchase"       // 租户现金余额购积分（内部划转）
)

// OnlineOrderTypes 在线充值相关的 order_type（不可撤销、需纳入统计口径）。
var OnlineOrderTypes = []string{OrderTypeOnlineUserTopup, OrderTypeOnlineTenantTopup, OrderTypeCashPurchase}

// TenantRechargeOrderTypes 归入"租户充值"统计口径的 order_type。
var TenantRechargeOrderTypes = []string{OrderTypePlatformToTenant, OrderTypeOnlineTenantTopup, OrderTypeCashPurchase}

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
	TenantCredits  *int64  // 租户积分（pending 阶段为冻结额，succeeded 后为实际扣减额）
	UserCredits    *int64  // 用户积分（同上）
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

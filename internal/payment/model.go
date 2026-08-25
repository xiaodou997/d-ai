// Package payment 实现微信支付在线充值：下单、回调核销、兜底 sweep、统一 USD 账户与提现，
// 以及管理端费率与微信商户配置。
package payment

import "time"

// 支付场景
const (
	SceneUserTopup   = "user_topup"
	SceneTenantTopup = "tenant_topup"
)

// Fulfillment status is deliberately separate from the provider payment
// status. A paid order remains paid even when its credited balance is revoked.
const (
	FulfillmentStatusPending           = "pending"
	FulfillmentStatusCredited          = "credited"
	FulfillmentStatusPartiallyReversed = "partially_reversed"
	FulfillmentStatusReversed          = "reversed"
)

const (
	RefundStatusNone     = "none"
	RefundStatusRefunded = "refunded"
	RefundMethodWechat   = "wechat"
	RefundMethodOffline  = "offline"
)

// 支付订单状态机：created -> paying -> paid（终态）；created/paying -> closed/expired（终态，
// 但 closed/expired 收到微信侧 SUCCESS 仍需补入账，见 Settle）。
const (
	OrderStatusCreated = "created"
	OrderStatusPaying  = "paying"
	OrderStatusPaid    = "paid"
	OrderStatusClosed  = "closed"
	OrderStatusExpired = "expired"
)

// 现金流水类型
const (
	CashTxnTopupIncome    = "topup_income"
	CashTxnRefundReversal = "refund_reversal"
	CashTxnConsumption    = "consumption"
	CashTxnWithdraw       = "withdraw"
	CashTxnAdjust         = "adjust"
)

// 提现历史状态兼容值。新提现由管理员直接创建并记为 paid，不再经过
// pending/approved/rejected/cancelled 申请审核状态机。
const (
	WithdrawalStatusPending   = "pending"
	WithdrawalStatusApproved  = "approved"
	WithdrawalStatusRejected  = "rejected"
	WithdrawalStatusPaid      = "paid"
	WithdrawalStatusCancelled = "cancelled"
)

// WithdrawalListParams is the application-facing filter for withdrawal
// queries. TenantID is empty for platform-admin views and scoped for tenant
// views.
type WithdrawalListParams struct {
	TenantID string
	Status   string
	Page     int
	Size     int
}

func ValidWithdrawalStatus(status string) bool {
	switch status {
	case WithdrawalStatusPending, WithdrawalStatusApproved, WithdrawalStatusRejected,
		WithdrawalStatusPaid, WithdrawalStatusCancelled:
		return true
	default:
		return false
	}
}

// Order 对应 pay_orders 表。
type Order struct {
	ID                     int64
	OrderID                string
	OutTradeNo             string
	Scene                  string
	TenantID               string
	TenantName             string // 列表查询时关联的当前租户名称
	UserID                 string // 为空表示 tenant_topup
	Username               string // 列表查询时关联的当前用户名
	TopupMode              string
	PackageID              string
	PackageName            string
	PackageBadge           string
	PaymentCurrency        string
	PaymentAmountMinor     int64
	LedgerCurrency         string
	GrossAmountMicroUSD    int64
	FeeRateBp              int
	FeeAmountMicroUSD      int64
	GiftAmountMicroUSD     int64
	CreditedAmountMicroUSD int64
	TenantIncomeMicroUSD   int64
	BalanceExpiresAt       *time.Time
	Channel                string
	CodeURL                string
	TransactionID          string
	Status                 string
	FulfillmentStatus      string
	RefundStatus           string
	PaidAt                 *time.Time
	ExpiresAt              time.Time
	BalanceOrderID         string
	FailNote               string
	SweepAttempts          int
	SweepNextAttemptAt     *time.Time
	SweepLastAttemptAt     *time.Time
	SweepLastError         string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Refund records a completed external refund. The original payment remains
// paid; this is the separate refund fact used for reconciliation and reversal.
type Refund struct {
	RefundID          string
	PaymentOrderID    string
	Method            string
	RefundReference   string
	ChannelRefundID   string
	RefundAmountMinor int64
	Status            string
	RefundedAt        time.Time
	Reason            string
	Note              string
	OperatorID        string
	CreatedAt         time.Time
}

type RefundReversalEffect struct {
	ReversalID                 string
	RefundID                   string
	RechargeOrderID            string
	AccountID                  string
	CreditAmountMicroUSD       int64
	AvailableReclaimedMicroUSD int64
	NonAvailableDebitMicroUSD  int64
	ExpiredAmountMicroUSD      int64
	AccountDebitMicroUSD       int64
	BalanceAfterMicroUSD       int64
}

// BalanceAccount is the tenant's single USD balance projection.
type BalanceAccount struct {
	TenantID        string
	BalanceMicroUSD int64
	UpdatedAt       time.Time
}

// CashLedgerEntry 对应 pay_cash_ledger 表。
type CashLedgerEntry struct {
	ID                   int64
	TxnID                string
	TenantID             string
	TxnType              string
	AmountMicroUSD       int64
	BalanceAfterMicroUSD int64
	RefType              string
	RefID                string
	OperatorID           string
	Note                 string
	CreatedAt            time.Time
}

// Withdrawal 对应 pay_withdrawals 表。
type Withdrawal struct {
	ID                   int64
	WithdrawalID         string
	TenantID             string
	AmountMicroUSD       int64
	FeeAmountMicroUSD    int64
	PayoutAmountMicroUSD int64
	AccountName          string
	BankName             string
	AccountNo            string
	ApplyNote            string
	Status               string
	AppliedBy            string
	ReviewedBy           string
	ReviewedAt           *time.Time
	ReviewNote           string
	PaidBy               string
	PaidAt               *time.Time
	PaymentRef           string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

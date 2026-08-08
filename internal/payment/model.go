// Package payment 实现微信支付在线充值：下单、回调核销、兜底 sweep、统一 USD 账户与提现，
// 以及管理端费率与微信商户配置。
package payment

import "time"

// 支付场景
const (
	SceneUserTopup   = "user_topup"
	SceneTenantTopup = "tenant_topup"
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
	CashTxnTopupIncome = "topup_income"
	CashTxnConsumption = "consumption"
	CashTxnWithdraw    = "withdraw"
	CashTxnAdjust      = "adjust"
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

// Order 对应 pay_orders 表。
type Order struct {
	ID                     int64
	OrderID                string
	OutTradeNo             string
	Scene                  string
	TenantID               string
	UserID                 string // 为空表示 tenant_topup
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
	PaidAt                 *time.Time
	ExpiresAt              time.Time
	BalanceOrderID         string
	FailNote               string
	CreatedAt              time.Time
	UpdatedAt              time.Time
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

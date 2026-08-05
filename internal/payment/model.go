// Package payment 实现微信支付在线充值：下单/回调核销/兜底 sweep、租户现金账户与提现、
// 管理端费率与微信商户配置。子包 payment/wechat 是与本包及 urm 业务解耦的自闭环模块
// （Gateway 接口 + wechatpay-go 真实实现 + mock 实现 + 商户配置读写），方便日后其他服务照抄参考。
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
	CashTxnBuyCredits  = "buy_credits"
	CashTxnWithdraw    = "withdraw"
	CashTxnAdjust      = "adjust"
)

// 提现状态机：pending -> approved -> paid；pending -> rejected；pending -> cancelled（仅申请人）。
const (
	WithdrawalStatusPending   = "pending"
	WithdrawalStatusApproved  = "approved"
	WithdrawalStatusRejected  = "rejected"
	WithdrawalStatusPaid      = "paid"
	WithdrawalStatusCancelled = "cancelled"
)

// Order 对应 pay_orders 表。
type Order struct {
	ID                int64
	OrderID           string
	OutTradeNo        string
	Scene             string
	TenantID          string
	UserID            string // 为空表示 tenant_topup
	TopupMode         string
	PackageID         string
	PackageName       string
	PackageBadge      string
	Amount            int64
	ExchangeRate      int64
	GrossCreditAmount int64
	FeeRateBp         int
	FeeCreditAmount   int64
	CreditAmount      int64
	FeeAmount         int64
	NetAmount         int64
	Channel           string
	CodeURL           string
	TransactionID     string
	Status            string
	PaidAt            *time.Time
	ExpiresAt         time.Time
	CreditOrderID     string
	FailNote          string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CashAccount 对应 pay_cash_accounts 表。
type CashAccount struct {
	TenantID  string
	Balance   int64
	Frozen    int64
	UpdatedAt time.Time
}

// Available 可用余额 = balance - frozen。
func (a CashAccount) Available() int64 {
	if v := a.Balance - a.Frozen; v > 0 {
		return v
	}
	return 0
}

// CashLedgerEntry 对应 pay_cash_ledger 表。
type CashLedgerEntry struct {
	ID           int64
	TxnID        string
	TenantID     string
	TxnType      string
	Amount       int64
	BalanceAfter int64
	RefType      string
	RefID        string
	OperatorID   string
	Note         string
	CreatedAt    time.Time
}

// Withdrawal 对应 pay_withdrawals 表。
type Withdrawal struct {
	ID           int64
	WithdrawalID string
	TenantID     string
	Amount       int64
	FeeAmount    int64
	PayoutAmount int64
	AccountName  string
	BankName     string
	AccountNo    string
	ApplyNote    string
	Status       string
	AppliedBy    string
	ReviewedBy   string
	ReviewedAt   *time.Time
	ReviewNote   string
	PaidBy       string
	PaidAt       *time.Time
	PaymentRef   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

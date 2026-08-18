package payment

// AdminRechargeOrder is the management projection of one recharge business
// action. Online orders are rooted at pay_orders; manual orders are rooted at
// bill_recharge_orders.
type AdminRechargeOrder struct {
	OrderID                string                 `json:"orderId"`
	BalanceOrderID         string                 `json:"balanceOrderId,omitempty"`
	Method                 string                 `json:"method"`
	TargetType             string                 `json:"targetType"`
	OrderType              string                 `json:"orderType"`
	TenantID               string                 `json:"tenantId"`
	TenantName             string                 `json:"tenantName"`
	UserID                 string                 `json:"userId,omitempty"`
	Username               string                 `json:"username,omitempty"`
	PaidAmountMinor        int64                  `json:"paidAmountMinor"`
	GrossAmountMicroUSD    int64                  `json:"grossAmountMicroUsd"`
	FeeAmountMicroUSD      int64                  `json:"feeAmountMicroUsd"`
	GiftAmountMicroUSD     int64                  `json:"giftAmountMicroUsd"`
	CreditedAmountMicroUSD int64                  `json:"creditedAmountMicroUsd"`
	TenantIncomeMicroUSD   int64                  `json:"tenantIncomeMicroUsd"`
	PaymentStatus          string                 `json:"paymentStatus"`
	FulfillmentStatus      string                 `json:"fulfillmentStatus"`
	OutTradeNo             string                 `json:"outTradeNo,omitempty"`
	TransactionID          string                 `json:"transactionId,omitempty"`
	TopupMode              string                 `json:"topupMode,omitempty"`
	PackageName            string                 `json:"packageName,omitempty"`
	Channel                string                 `json:"channel,omitempty"`
	Note                   string                 `json:"note,omitempty"`
	FailNote               string                 `json:"failNote,omitempty"`
	CreatedAt              int64                  `json:"createdAt"`
	PaidAt                 *int64                 `json:"paidAt,omitempty"`
	PaymentExpiresAt       *int64                 `json:"paymentExpiresAt,omitempty"`
	BalanceExpiresAt       *int64                 `json:"balanceExpiresAt,omitempty"`
	ReversedAt             *int64                 `json:"reversedAt,omitempty"`
	ReversedBy             string                 `json:"reversedBy,omitempty"`
	ReversalReason         string                 `json:"reversalReason,omitempty"`
	Credits                []RechargeCreditDetail `json:"credits,omitempty"`
}

type RechargeCreditDetail struct {
	BalanceOrderID          string `json:"balanceOrderId"`
	OrderType               string `json:"orderType"`
	Primary                 bool   `json:"primary"`
	CreditAmountMicroUSD    int64  `json:"creditAmountMicroUsd"`
	Status                  string `json:"status"`
	Note                    string `json:"note,omitempty"`
	BalanceExpiresAt        *int64 `json:"balanceExpiresAt,omitempty"`
	ReversedAt              *int64 `json:"reversedAt,omitempty"`
	ReversedBy              string `json:"reversedBy,omitempty"`
	ReversalReason          string `json:"reversalReason,omitempty"`
	ReversedAmountMicroUSD  int64  `json:"reversedAmountMicroUsd"`
	LostAmountMicroUSD      int64  `json:"lostAmountMicroUsd"`
	LotID                   string `json:"lotId,omitempty"`
	GrantedAmountMicroUSD   int64  `json:"grantedAmountMicroUsd"`
	ConsumedAmountMicroUSD  int64  `json:"consumedAmountMicroUsd"`
	RemainingAmountMicroUSD int64  `json:"remainingAmountMicroUsd"`
	LotStatus               string `json:"lotStatus"`
}

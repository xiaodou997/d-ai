package urm

type Response[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    T      `json:"data,omitempty"`
}

type FreezeRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
	TenantID       string `json:"tenantId"`
	UserID         string `json:"userId,omitempty"`
	Description    string `json:"description,omitempty"`
	TenantAmount   int64  `json:"tenantAmount"`
	UserAmount     int64  `json:"userAmount"`
}

type FreezeResponse struct {
	EventID      string `json:"eventId"`
	FrozenTenant int64  `json:"frozenTenant"`
	FrozenUser   int64  `json:"frozenUser"`
	Status       string `json:"status"`
}

type ConfirmRequest struct {
	EventID            string `json:"eventId"`
	ActualTenantAmount int64  `json:"actualTenantAmount"`
	ActualUserAmount   int64  `json:"actualUserAmount"`
}

type ConfirmResponse struct {
	EventID string `json:"eventId"`
	Status  string `json:"status"`
}

type BalanceResponse struct {
	PackageType      int    `json:"packageType"`
	CustomerID       string `json:"customerId"`
	TotalCredits     int64  `json:"totalCredits"`
	FrozenCredits    int64  `json:"frozenCredits"`
	AvailableCredits int64  `json:"availableCredits"`
	OverdraftLimit   int64  `json:"overdraftLimit"`
	CurrentOverdraft int64  `json:"currentOverdraft"`
}

// ConsumeRequest 单阶段聚合扣款请求（POST /internal/v1/settle/consume）。
// 适用于业务系统在本地完成精细计费后聚合扣款的场景。失败时不需要反向 Cancel。
type ConsumeRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
	TenantID       string `json:"tenantId"`
	UserID         string `json:"userId,omitempty"`
	Description    string `json:"description,omitempty"`
	TenantAmount   int64  `json:"tenantAmount"`
	UserAmount     int64  `json:"userAmount"`
}

// ConsumeResponse 单阶段扣款结果。Deducted 字段是实际从积分包扣的部分，
// OverdraftAdd 是计入透支额度的部分。两者之和等于请求的金额。
type ConsumeResponse struct {
	EventID            string `json:"eventId"`
	TenantDeducted     int64  `json:"tenantDeducted"`
	UserDeducted       int64  `json:"userDeducted"`
	TenantOverdraftAdd int64  `json:"tenantOverdraftAdd"`
	UserOverdraftAdd   int64  `json:"userOverdraftAdd"`
	Status             string `json:"status"`
}

type UserInfoResponse struct {
	Subject    string `json:"sub"`
	Username   string `json:"username"`
	UserType   int    `json:"userType"`
	TenantID   string `json:"tenantId"`
	TenantName string `json:"tenantName"`
	ClientID   string `json:"clientId"`
}

type TokenPairResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int64  `json:"expiresIn"`
}

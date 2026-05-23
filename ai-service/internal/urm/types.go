package urm

type Response[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    T      `json:"data,omitempty"`
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

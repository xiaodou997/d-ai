package urm

type Response[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    T      `json:"data,omitempty"`
}

type FreezeRequest struct {
	RequestID    string `json:"requestId"`
	TenantID     string `json:"tenantId"`
	CustomerID   string `json:"customerId,omitempty"`
	Description  string `json:"description,omitempty"`
	TenantAmount int64  `json:"tenantAmount"`
	UserAmount   int64  `json:"userAmount"`
}

type FreezeResponse struct {
	TransactionID string `json:"transactionId"`
	FrozenTenant  int64  `json:"frozenTenant"`
	FrozenUser    int64  `json:"frozenUser"`
	Status        string `json:"status"`
}

type ConfirmRequest struct {
	TransactionID      string `json:"transactionId"`
	ActualTenantAmount int64  `json:"actualTenantAmount"`
	ActualUserAmount   int64  `json:"actualUserAmount"`
}

type ConfirmResponse struct {
	TransactionID string `json:"transactionId"`
	Status        string `json:"status"`
}

type BalanceResponse struct {
	PackageType      int    `json:"packageType"`
	CustomerID       string `json:"customerId"`
	TotalCredits     int64  `json:"totalCredits"`
	FrozenCredits    int64  `json:"frozenCredits"`
	AvailableCredits int64  `json:"availableCredits"`
}

type UserInfoResponse struct {
	Subject     string   `json:"sub"`
	Username    string   `json:"username"`
	UserType    int      `json:"userType"`
	TenantID    string   `json:"tenantId"`
	TenantName  string   `json:"tenantName"`
	AllowedApps []string `json:"allowedApps"`
}

package platform

import (
	"errors"
	"net/http"
	"time"

	"xiaodou/dai/internal/auth"
)

var ErrEndUserNotFound = errors.New("end user not found")

// ErrInsufficientBalance 表示余额不足（或账户已透支）。
var ErrInsufficientBalance = errors.New("insufficient balance")

// Claims 直接复用统一认证域的 JWT claims。
type Claims = auth.Claims

// APIError 承载非 2xx 响应的结构化信息（HTTP 状态 + RFC7807 的 code）。
// 合并后 billingledger 的 classifyPortError 仍用它做错误分类。
// 进程内调用不会产生 APIError，但测试用例和错误分类逻辑仍引用此类型。
type APIError struct {
	Status int
	Code   string
	Body   string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return e.Code
	}
	return http.StatusText(e.Status)
}

func NewAPIError(status int, body []byte) *APIError {
	return &APIError{Status: status, Body: string(body)}
}

type AccountState string

const (
	AccountStateOK        AccountState = "OK"
	AccountStateOverdraft AccountState = "OVERDRAFT"
	AccountStateExhausted AccountState = "EXHAUSTED"
)

type LedgerDebitReceipt struct {
	AuthorizationID      string       `json:"authorization_id"`
	TenantDeductedMicro  int64        `json:"tenant_deducted_micro"`
	UserDeductedMicro    int64        `json:"user_deducted_micro"`
	TenantDebtAddedMicro int64        `json:"tenant_debt_added_micro"`
	UserDebtAddedMicro   int64        `json:"user_debt_added_micro"`
	AccountState         AccountState `json:"account_state"`
	AllowFurtherUsage    bool         `json:"allow_further_usage"`
}

type StrictDebitRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	TenantID       string `json:"tenant_id"`
	UserID         string `json:"user_id,omitempty"`
	Description    string `json:"description,omitempty"`
	TenantMicro    int64  `json:"tenant_micro,omitempty"`
	UserMicro      int64  `json:"user_micro,omitempty"`
}

type StrictDebitResponse = LedgerDebitReceipt
type AcquireCreditLeaseRequest struct {
	ClientWindowID       string `json:"client_window_id"`
	TenantID             string `json:"tenant_id"`
	UserID               string `json:"user_id,omitempty"`
	Description          string `json:"description,omitempty"`
	RequestedTenantMicro int64  `json:"requested_tenant_micro,omitempty"`
	RequestedUserMicro   int64  `json:"requested_user_micro,omitempty"`
	TTLSeconds           int64  `json:"ttl_seconds,omitempty"`
	GraceSeconds         int64  `json:"grace_seconds,omitempty"`
}

type RenewCreditLeaseRequest struct {
	Version      int64 `json:"version"`
	TTLSeconds   int64 `json:"ttl_seconds,omitempty"`
	GraceSeconds int64 `json:"grace_seconds,omitempty"`
}

type SettleCreditLeaseRequest struct {
	SettlementID      string `json:"settlement_id"`
	ActualTenantMicro int64  `json:"actual_tenant_micro,omitempty"`
	ActualUserMicro   int64  `json:"actual_user_micro,omitempty"`
}

type CreditLeaseResponse struct {
	LeaseID              string       `json:"lease_id"`
	ClientWindowID       string       `json:"client_window_id"`
	TenantID             string       `json:"tenant_id"`
	UserID               string       `json:"user_id,omitempty"`
	GrantedTenantMicro   int64        `json:"granted_tenant_micro"`
	GrantedUserMicro     int64        `json:"granted_user_micro"`
	EscrowState          string       `json:"escrow_state"`
	SettlementState      string       `json:"settlement_state"`
	Version              int64        `json:"version"`
	ExpiresAt            time.Time    `json:"expires_at"`
	GraceUntil           time.Time    `json:"grace_until"`
	SettlementID         string       `json:"settlement_id,omitempty"`
	ActualTenantMicro    *int64       `json:"actual_tenant_micro,omitempty"`
	ActualUserMicro      *int64       `json:"actual_user_micro,omitempty"`
	SettledEventID       string       `json:"settled_event_id,omitempty"`
	SettledAt            *time.Time   `json:"settled_at,omitempty"`
	TenantDeductedMicro  int64        `json:"tenant_deducted_micro"`
	UserDeductedMicro    int64        `json:"user_deducted_micro"`
	TenantDebtAddedMicro int64        `json:"tenant_debt_added_micro"`
	UserDebtAddedMicro   int64        `json:"user_debt_added_micro"`
	AccountState         AccountState `json:"account_state"`
	AllowFurtherUsage    bool         `json:"allow_further_usage"`
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

type EndUserRecord struct {
	UserID   string `json:"userId"`
	TenantID string `json:"tenantId"`
	Username string `json:"username"`
	Status   string `json:"status"`
}

type InternalUser struct {
	UserID      string  `json:"userId"`
	TenantID    string  `json:"tenantId"`
	Username    string  `json:"username"`
	Email       *string `json:"email,omitempty"`
	Nickname    *string `json:"nickname,omitempty"`
	Avatar      *string `json:"avatar,omitempty"`
	Status      int32   `json:"status"`
	CreatedTime *int64  `json:"createdTime,omitempty"`
}

type InternalTenant struct {
	TenantID   string  `json:"tenantId"`
	TenantName string  `json:"tenantName"`
	Status     *string `json:"status,omitempty"`
}

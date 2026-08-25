package ports

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrTenantUserNotFound means that the requested tenant user is missing or
	// is no longer a tenant user.
	ErrTenantUserNotFound = errors.New("tenant user not found")
	// ErrInvitationCodeTaken asks the application service to retry with a new
	// random code. The adapter translates the database unique violation.
	ErrInvitationCodeTaken = errors.New("invitation code taken")
	// ErrSelfServiceUnavailable means that a required tenant self-service
	// capability was not wired by the composition root.
	ErrSelfServiceUnavailable = errors.New("tenant self service unavailable")
)

// TenantUser is the safe projection returned by the tenant self-service
// account query. Password material and credential state never cross this port.
type TenantUser struct {
	UserID        string
	TenantID      string
	Username      string
	Email         string
	Phone         string
	Status        int
	LastLoginTime *int64
	CreatedTime   int64
}

// InviteCodeItem is the tenant-scoped invitation projection used by the
// self-service list endpoint.
type InviteCodeItem struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	TenantID    string `json:"tenantId"`
	CreatedBy   string `json:"createdBy"`
	Description string `json:"description"`
	MaxUses     int    `json:"maxUses"`
	UsedCount   int    `json:"usedCount"`
	Status      int    `json:"status"`
	ExpireTime  *int64 `json:"expireTime,omitempty"`
	CreatedTime int64  `json:"createdTime"`
	UpdatedTime int64  `json:"updatedTime"`
}

// TenantOverviewStats contains the aggregate tenant self-service metrics.
type TenantOverviewStats struct {
	EndUserCount             int64   `json:"endUserCount"`
	InviteCodeCount          int64   `json:"inviteCodeCount"`
	UserDeductionUSD         float64 `json:"userDeductionUsd"`
	UserTotalBalanceUSD      float64 `json:"userTotalBalanceUsd"`
	ActiveUserCount          int64   `json:"activeUserCount"`
	UserConsumptionCount     int64   `json:"userConsumptionCount"`
	SettlementIncomeMicroUSD int64   `json:"settlementIncomeMicroUsd"`
}

// ClientConsumptionItem is an application/client usage aggregate.
type ClientConsumptionItem struct {
	ClientID   string  `json:"clientId"`
	ClientName string  `json:"clientName"`
	AmountUSD  float64 `json:"amountUsd"`
	Percentage string  `json:"percentage"`
}

// UserConsumptionItem is a terminal-user usage aggregate.
type UserConsumptionItem struct {
	UserID           string  `json:"userId"`
	Username         string  `json:"username"`
	AmountUSD        float64 `json:"amountUsd"`
	TransactionCount int64   `json:"transactionCount"`
	Percentage       string  `json:"percentage"`
}

// InvitationCreateCommand contains the tenant-scoped creation input. Code
// generation and uniqueness retry are application concerns, not HTTP logic.
type InvitationCreateCommand struct {
	TenantID    string
	CreatedBy   string
	Description string
	MaxUses     int
	ExpireTime  *int64
}

// InvitationUpdateCommand contains the tenant-scoped mutable fields.
type InvitationUpdateCommand struct {
	ID          int64
	TenantID    string
	Status      int
	Description string
}

// TenantSelfReader exposes tenant-user and tenant analytics queries without
// leaking a database repository into HTTP.
type TenantSelfReader interface {
	GetByUserID(ctx context.Context, userID string) (*TenantUser, error)
	ListInvitationCodes(ctx context.Context, tenantID string, page, size int) ([]InviteCodeItem, int64, error)
	GetTenantOverviewStats(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time) (*TenantOverviewStats, error)
	GetClientConsumption(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time) ([]ClientConsumptionItem, error)
	GetUserConsumptionRanking(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time, limit int) ([]UserConsumptionItem, error)
}

// TenantInvitationWriter exposes tenant-scoped invitation mutations. The
// application service supplies the generated code and owns retry semantics.
type TenantInvitationWriter interface {
	CreateInvitationCode(ctx context.Context, code, tenantID, createdBy, description string, maxUses int, expireTime *int64) error
	UpdateInvitationCode(ctx context.Context, id int64, tenantID string, status int, description string) error
	DeleteInvitationCode(ctx context.Context, id int64, tenantID string) error
}

// TenantSelfService is the application boundary consumed by tenant HTTP
// handlers. Implementations may combine a reader and invitation writer while
// preserving separate ports at the persistence edge.
type TenantSelfService interface {
	GetByUserID(ctx context.Context, userID string) (*TenantUser, error)
	ListInvitationCodes(ctx context.Context, tenantID string, page, size int) ([]InviteCodeItem, int64, error)
	CreateInvitation(ctx context.Context, input InvitationCreateCommand) (InviteCodeItem, error)
	UpdateInvitation(ctx context.Context, input InvitationUpdateCommand) error
	DeleteInvitation(ctx context.Context, id int64, tenantID string) error
	GetTenantOverviewStats(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time) (*TenantOverviewStats, error)
	GetClientConsumption(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time) ([]ClientConsumptionItem, error)
	GetUserConsumptionRanking(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time, limit int) ([]UserConsumptionItem, error)
}

package ports

import (
	"context"
	"errors"
	"time"
)

var ErrAccountQueryUnavailable = errors.New("account query service unavailable")

// BalanceResponse is the account balance projection consumed by Portal.
type BalanceResponse struct {
	Currency                string              `json:"currency"`
	TotalUSD                float64             `json:"totalUsd"`
	UsedUSD                 float64             `json:"usedUsd"`
	RemainingUSD            float64             `json:"remainingUsd"`
	AvailableUSD            float64             `json:"availableUsd"`
	PermanentUSD            float64             `json:"permanentUsd"`
	TimedUSD                float64             `json:"timedUsd"`
	OutstandingDebtMicroUSD int64               `json:"outstandingDebtMicroUsd"`
	ServiceState            string              `json:"serviceState"`
	BalanceLots             []AccountBalanceLot `json:"balanceLots,omitempty"`
}

type AccountBalanceLot struct {
	BalanceLotID string     `json:"balanceLotId"`
	TotalUSD     float64    `json:"totalUsd"`
	RemainingUSD float64    `json:"remainingUsd"`
	CreatedAt    time.Time  `json:"createdAt"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	Source       string     `json:"source"`
}

type RechargeRecordRow struct {
	OrderID         string  `json:"orderId"`
	OrderType       string  `json:"orderType"`
	PaidAmountMinor int64   `json:"paidAmountMinor"`
	AmountUSD       float64 `json:"amountUsd"`
	Status          string  `json:"status"`
	Note            string  `json:"note"`
	UserID          string  `json:"userId"`
	Username        string  `json:"username"`
	TenantName      string  `json:"tenantName"`
	CreatedTime     *int64  `json:"createdTime"`
}

type AccountStatsResult struct {
	EndUserCount     int64   `json:"endUserCount"`
	InviteCodeCount  int64   `json:"inviteCodeCount"`
	UserDeductionUSD float64 `json:"userDeductionUsd"`
}

// RechargeRecordsQuery is the validated application query for recharge
// history. Order type scope is selected by the caller's authorization policy.
type RechargeRecordsQuery struct {
	TenantID   string
	UserID     string
	TenantName string
	Username   string
	OrderTypes []string
	TimeFrom   *time.Time
	TimeTo     *time.Time
	Page       int
	Size       int
}

// AccountQueryReader is the narrow application read port for account views.
type AccountQueryReader interface {
	GetTenantBalance(ctx context.Context, tenantID string, detail bool) (*BalanceResponse, error)
	GetUserBalance(ctx context.Context, userID string, detail bool) (*BalanceResponse, error)
	ListRechargeRecords(ctx context.Context, query RechargeRecordsQuery) ([]RechargeRecordRow, int64, error)
	GetAccountStats(ctx context.Context, tenantID string) (*AccountStatsResult, error)
}

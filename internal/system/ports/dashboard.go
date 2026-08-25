package ports

import (
	"context"
	"time"
)

type FailedTransactionAlert struct {
	RequestID       string
	SettlementError string
	BillingStatus   string
	CreatedAt       time.Time
}

type GlobalStats struct {
	Currency                string  `json:"currency"`
	TenantRechargePaidMinor int64   `json:"tenantRechargePaidMinor"`
	TenantRechargeAmountUSD float64 `json:"tenantRechargeAmountUsd"`
	ActiveTenants           int64   `json:"activeTenants"`
	TenantTotalBalanceUSD   float64 `json:"tenantTotalBalanceUsd"`
	UserRechargePaidMinor   int64   `json:"userRechargePaidMinor"`
	UserRechargeAmountUSD   float64 `json:"userRechargeAmountUsd"`
	NewUsers                int64   `json:"newUsers"`
	UserTotalBalanceUSD     float64 `json:"userTotalBalanceUsd"`
}

type ConsumptionTrendPoint struct {
	Day   time.Time
	Total float64
}

type ConsumptionTrendQuery struct {
	TimeFrom    *time.Time
	TimeTo      *time.Time
	TenantID    *string
	AccountType *int64
}

type ResourceStatistic struct {
	ClientID   string
	ClientName string
	Total      float64
}

type ResourceStatisticsQuery struct {
	TimeFrom *time.Time
	TimeTo   *time.Time
	TenantID *string
}

type AdminDashboardReader interface {
	ListFailedTransactionAlerts(ctx context.Context) ([]FailedTransactionAlert, error)
	GetGlobalStats(ctx context.Context, timeFrom, timeTo *time.Time) (GlobalStats, error)
	GetConsumptionTrend(ctx context.Context, query ConsumptionTrendQuery) ([]ConsumptionTrendPoint, error)
	GetResourceStatistics(ctx context.Context, query ResourceStatisticsQuery) ([]ResourceStatistic, error)
}

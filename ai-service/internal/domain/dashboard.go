package domain

import "time"

// DashboardSummary is the headline panel of the analytics dashboard. Cost
// fields are micro-credits (suffix Micro); the HTTP layer converts to display
// credits.
type DashboardSummary struct {
	TotalRequests          int64
	SuccessfulRequests     int64
	FailedRequests         int64
	TotalTokens            int64
	TotalPromptTokens      int64
	TotalCompletionTokens  int64
	TotalProviderCostMicro int64
	TotalPlatformCostMicro int64
	TotalUserCostMicro     int64
	AvgLatencyMs           float64
}

// DashboardTopModel is one row of the "top models by usage" widget.
type DashboardTopModel struct {
	ModelCode      string
	RequestCount   int64
	TotalTokens    int64
	TotalCostMicro int64
}

// DashboardTopTenant is one row of the "top tenants by usage" widget.
type DashboardTopTenant struct {
	TenantID       string
	RequestCount   int64
	TotalTokens    int64
	TotalCostMicro int64
}

// DashboardRecentError is one row of the "recent errors" widget.
type DashboardRecentError struct {
	RequestID     string
	ModelCode     string
	RequestStatus string
	ErrorCode     string
	ErrorMessage  string
	HTTPStatus    *int32
	CreatedAt     time.Time
}

// DashboardFilter scopes dashboard queries. Empty TenantID/UserID mean "all";
// Since is required by the underlying queries but nil maps to NULL.
type DashboardFilter struct {
	TenantID string
	UserID   string
	Since    *time.Time
}

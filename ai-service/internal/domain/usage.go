package domain

import "time"

// UsageLog is the management-domain view of a row in ai_usage_logs (the full
// admin projection). Cost fields are micro-credits (suffix Micro); the HTTP
// layer converts to display credits. Optional columns use Go zero values for
// strings and *int32 for nullable ints.
type UsageLog struct {
	ID                   string
	RequestID            string
	TraceID              string
	APIKeyID             string
	KeyOwnerType         string
	AuthMethod           string
	RequestSource        string
	TenantID             string
	UserID               string
	ExternalUserID       string
	ModelID              string
	ModelCode            string
	CapabilityType       string
	ModelRouteID         string
	UpstreamDeploymentID string
	EndpointID           string
	ProviderCode         string
	UpstreamModel        string
	ConversationID       string
	Stream               bool
	PromptTokens         int32
	CompletionTokens     int32
	TotalTokens          int32
	BillableUnitType     string
	BillableUnits        int64
	ProviderCostMicro    int64
	PlatformCostMicro    int64
	UserCostMicro        int64
	APIKeyQuotaCostMicro int64
	URMTransactionID     string
	BillingStatus        string
	RequestStatus        string
	HTTPStatus           *int32
	UpstreamStatus       *int32
	LatencyMs            *int32
	FirstTokenLatencyMs  *int32
	ErrorCode            string
	ErrorMessage         string
	UsageEstimated       bool
	TokenUsageSource     string
	CreatedAt            time.Time
}

// UsageStats is the aggregate panel shown alongside a usage-log page (computed
// over the same filter, not just the current page). TotalCostMicro is the sum
// of user_cost in micro-credits.
type UsageStats struct {
	TotalRequests  int64
	SuccessCount   int64
	FailedCount    int64
	TotalTokens    int64
	TotalCostMicro int64
	AvgLatencyMs   float64
}

// UsageSummaryRow aggregates usage per model. Cost fields are micro-credits.
type UsageSummaryRow struct {
	ModelCode              string
	RequestCount           int64
	TotalPromptTokens      int64
	TotalCompletionTokens  int64
	TotalTokens            int64
	TotalProviderCostMicro int64
	TotalPlatformCostMicro int64
	TotalUserCostMicro     int64
	TotalQuotaCostMicro    int64
}

// UsageUnitSummaryRow aggregates usage per billable-unit type.
type UsageUnitSummaryRow struct {
	BillableUnitType       string
	RequestCount           int64
	TotalBillableUnits     int64
	TotalProviderCostMicro int64
	TotalPlatformCostMicro int64
	TotalUserCostMicro     int64
}

// UserUsageSummary is the single-row summary for one tenant user.
type UserUsageSummary struct {
	RequestCount          int64
	SuccessRequests       int64
	FailedRequests        int64
	TotalTokens           int64
	TotalPromptTokens     int64
	TotalCompletionTokens int64
	TotalUserCostMicro    int64
	AvgLatencyMs          float64
}

// DailyTrendRow is one day of aggregated usage from ai_usage_rollups_hourly.
// Cost fields are micro-credits.
type DailyTrendRow struct {
	Date              string
	RequestCount      int64
	SuccessCount      int64
	FailedCount       int64
	TotalTokens       int64
	PromptTokens      int64
	CompletionTokens  int64
	ProviderCostMicro int64
	PlatformCostMicro int64
	UserCostMicro     int64
	AvgLatencyMs      int64
}

// UsageFilter scopes usage queries. Empty strings / nil times mean "no filter".
type UsageFilter struct {
	TenantID      string
	UserID        string
	ModelCode     string
	RequestStatus string
	RequestSource string
	DateFrom      *time.Time
	DateTo        *time.Time
}

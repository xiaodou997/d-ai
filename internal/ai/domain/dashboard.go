package domain

import "time"

// DashboardSummary is the headline panel of the analytics dashboard. Cost
// fields are micro-credits (suffix Micro); the HTTP layer converts to display
// credits.
type DashboardSummary struct {
	TotalRequests           int64
	SuccessfulRequests      int64
	FailedRequests          int64
	TotalTokens             int64
	TotalPromptTokens       int64
	TotalCompletionTokens   int64
	TotalCatalogBaseMicro   int64
	TotalTenantPayableMicro int64
	TotalRetailBaseMicro    int64
	TotalUserPayableMicro   int64
	TotalUserChargedMicro   int64
	AvgLatencyMs            float64
	AvgRequestTotalMs       float64
	AvgFirstResponseByteMs  float64
}

// DashboardTopModel is one row of the "top models by usage" widget.
type DashboardTopModel struct {
	ModelCode               string
	RequestCount            int64
	TotalTokens             int64
	TotalTenantPayableMicro int64
}

// DashboardTopTenant is one row of the "top tenants by usage" widget.
type DashboardTopTenant struct {
	TenantID                string
	RequestCount            int64
	TotalTokens             int64
	TotalTenantPayableMicro int64
}

// DashboardRecentError is one row of the "recent errors" widget.
type DashboardRecentError struct {
	RequestID                  string
	ModelCode                  string
	RequestedModel             string
	MatchedDispatchRuleSummary string
	ResolvedLogicalModel       string
	ResolvedProviderFamily     string
	ClientProtocol             string
	SelectedUpstreamProtocol   string
	UpstreamModel              string
	ProtocolConversionEnabled  bool
	RequestStatus              string
	ErrorCode                  string
	ErrorMessage               string
	HTTPStatus                 *int32
	CreatedAt                  time.Time
}

// DashboardFilter scopes dashboard queries. Empty TenantID/UserID mean "all";
// DateFrom/DateTo define an exact [start, end) window when provided.
type DashboardFilter struct {
	TenantID string
	UserID   string
	DateFrom *time.Time
	DateTo   *time.Time
}

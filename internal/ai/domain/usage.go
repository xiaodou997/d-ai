package domain

import "time"

// UsageLog is the management-domain view of a row in ai_usage_logs (the full
// admin projection). Cost fields are micro-USD (suffix Micro); the HTTP
// layer converts to display USD. Optional columns use Go zero values for
// strings and *int32 for nullable ints.
type UsageLog struct {
	ID                                 string
	RequestID                          string
	TraceID                            string
	APIKeyID                           string
	APIKeyName                         string
	APIKeyLastFour                     string
	KeyOwnerType                       string
	AuthMethod                         string
	RequestSource                      string
	TenantID                           string
	UserID                             string
	Username                           string
	ClientUserAgent                    string
	ExternalUserID                     string
	GroupID                            string
	GroupNameSnapshot                  string
	GroupDefaultUserMultiplierSnapshot float64
	UserMultiplierOverrideSnapshot     *float64
	EffectiveUserMultiplierSnapshot    float64
	BillingGroupLabelSnapshot          string
	ModelCode                          string
	RequestedModel                     string
	MatchedDispatchRuleID              string
	MatchedDispatchRuleSummary         string
	ResolvedLogicalModel               string
	ResolvedProviderFamily             string
	CapabilityType                     string
	GroupTargetID                      string
	UpstreamAccountID                  string
	UpstreamAccountName                string
	UpstreamTenantDisplayName          string
	EndpointID                         string
	CredentialPoolID                   string
	ProviderCode                       string
	UpstreamModel                      string
	ConversationID                     string
	Stream                             bool
	PromptTokens                       int32
	CompletionTokens                   int32
	CacheWriteTokens                   int32
	CacheReadTokens                    int32
	ReasoningTokens                    int32
	ReasoningEffort                    string
	TotalTokens                        int32
	AttemptsCount                      int32
	BillableUnitType                   string
	BillableUnits                      int64
	Resolution                         string
	CatalogBaseMicro                   int64
	TenantPayableMicro                 int64
	RetailBaseMicro                    int64
	UserPayableMicro                   int64
	UserChargedMicro                   int64
	APIKeyQuotaCostMicro               int64
	ServiceTier                        string
	BillingBreakdownJSON               []byte
	BillingStatus                      string
	SettlementError                    string
	RefundStatus                       string
	RefundReason                       string
	RefundOperatorID                   string
	SettledAt                          *time.Time
	RefundedAt                         *time.Time
	RequestStatus                      string
	HTTPStatus                         *int32
	UpstreamStatus                     *int32
	LatencyMs                          *int32
	FirstTokenLatencyMs                *int32
	RequestTotalMs                     *int32
	RequestSetupMs                     *int32
	FirstResponseByteMs                *int32
	ResponseTailMs                     *int32
	FinalAttemptHeaderMs               *int32
	FinalAttemptTotalMs                *int32
	ErrorCode                          string
	ErrorMessage                       string
	ProtocolConversionEnabled          bool
	UpstreamModelMappingApplied        bool
	PublicResponseModel                string
	UsageEstimated                     bool
	TokenUsageSource                   string
	BillingSource                      string
	CreatedAt                          time.Time
}

type UsageLogDetail struct {
	UsageLog
	ClientProtocol             string
	SelectedUpstreamProtocol   string
	SelectedUpstreamTargetType string
	SelectedUpstreamModel      string
	RequestMessages            []byte
	RequestParams              []byte
	ResponseMessage            []byte
	MediaRefs                  []byte
	RequestPath                string
	ClientIP                   string
	UserAgent                  string
	AuthMasked                 string

	// InternalErrorDetail / FailedStep / AttemptsDetail: admin-only
	// diagnostics populated from ai_request_payloads. Never surfaced on
	// tenant/user self-service usage endpoints — only the platform admin
	// detail handler reads these off UsageLogDetail.
	InternalErrorDetail string
	FailedStep          string
	// AttemptsDetail is the raw JSON array of every upstream candidate tried
	// during Execute (see serving.BuildAttemptsDetail); nil when the request
	// never reached Execute.
	AttemptsDetail []byte
}

// UsageStats is the aggregate panel shown alongside a usage-log page (computed
// over the same filter, not just the current page).
// UsageStats是用量列表的表头合计。三条金额线都要出现：单独的 user_charged 对
// 租户自有 key 流量和订阅覆盖流量恒为 0，用它当唯一合计会把真实有收入的流量显示成免费。
// 平台应收看 TotalTenantPayableMicro，租户对终端用户的收入看 TotalUserChargedMicro。
type UsageStats struct {
	TotalRequests           int64
	SuccessCount            int64
	FailedCount             int64
	TotalTokens             int64
	TotalCatalogBaseMicro   int64
	TotalTenantPayableMicro int64
	TotalUserChargedMicro   int64
	AvgLatencyMs            float64
	AvgRequestTotalMs       float64
	AvgFirstResponseByteMs  float64
}

// UsageSummaryRow aggregates usage per model. Cost fields are micro-USD.
type UsageSummaryRow struct {
	ModelCode               string
	RequestCount            int64
	TotalPromptTokens       int64
	TotalCompletionTokens   int64
	TotalTokens             int64
	TotalCatalogBaseMicro   int64
	TotalTenantPayableMicro int64
	TotalRetailBaseMicro    int64
	TotalUserPayableMicro   int64
	TotalUserChargedMicro   int64
	TotalQuotaCostMicro     int64
}

// UsageUnitSummaryRow aggregates usage per billable-unit type.
type UsageUnitSummaryRow struct {
	BillableUnitType        string
	RequestCount            int64
	TotalBillableUnits      int64
	TotalCatalogBaseMicro   int64
	TotalTenantPayableMicro int64
	TotalRetailBaseMicro    int64
	TotalUserPayableMicro   int64
	TotalUserChargedMicro   int64
}

// UsageUpstreamSummaryRow reconciles the same settled usage rows by concrete
// upstream account or credential pool. CatalogBaseMicro is the
// price-book reference amount, while TenantPayableMicro is the platform's
// receivable from the tenant.
type UsageUpstreamSummaryRow struct {
	TargetKind string
	TargetID   string
	// TargetName is empty when the account or pool has since been deleted; the
	// row is still reported, since history must not vanish with the resource.
	TargetName            string
	ProviderCode          string
	RequestCount          int64
	SuccessCount          int64
	FailedCount           int64
	TotalPromptTokens     int64
	TotalCompletionTokens int64
	TotalTokens           int64
	// TokenUnits and ImageUnits split billable_units by capability. Summing the
	// two would add tokens to image counts and mean nothing.
	TokenUnits         int64
	ImageUnits         int64
	CatalogBaseMicro   int64
	TenantPayableMicro int64
}

// UsageUserRankingRow aggregates usage per tenant user for the admin ranking view.
type UsageUserRankingRow struct {
	TenantID              string
	UserID                string
	RequestCount          int64
	SuccessCount          int64
	FailedCount           int64
	TotalTokens           int64
	TotalUserChargedMicro int64
	LastRequestedAt       time.Time
}

// UserUsageSummary is the single-row summary for one tenant user.
type UserUsageSummary struct {
	RequestCount          int64
	SuccessRequests       int64
	FailedRequests        int64
	TotalTokens           int64
	TotalPromptTokens     int64
	TotalCompletionTokens int64
	TotalUserChargedMicro int64
	AvgLatencyMs          float64
	AvgRequestTotalMs     float64
}

// DailyTrendRow is one day of aggregated usage.
// Cost fields are micro-USD.
type DailyTrendRow struct {
	Date                   string
	RequestCount           int64
	SuccessCount           int64
	FailedCount            int64
	TotalTokens            int64
	PromptTokens           int64
	CompletionTokens       int64
	CatalogBaseMicro       int64
	TenantPayableMicro     int64
	RetailBaseMicro        int64
	UserPayableMicro       int64
	UserChargedMicro       int64
	AvgLatencyMs           int64
	AvgRequestTotalMs      int64
	AvgFirstResponseByteMs int64
}

// UsageFilter scopes usage queries. Empty strings / nil times mean "no filter".
type UsageFilter struct {
	TenantID      string
	TenantName    string
	UserID        string
	UserName      string
	ModelCode     string
	RequestStatus string
	RequestSource string
	DateFrom      *time.Time
	DateTo        *time.Time
}

// UsageSummaryFilter scopes aggregate usage projections. It stays separate
// from UsageFilter so aggregate and log-row query contracts can evolve
// independently.
type UsageSummaryFilter struct {
	TenantID      string
	TenantName    string
	UserID        string
	UserName      string
	ModelCode     string
	RequestStatus string
	RequestSource string
	DateFrom      *time.Time
	DateTo        *time.Time
}

// UsageLogPage combines the filtered total, aggregate stats and page records.
type UsageLogPage struct {
	Total   int64
	Stats   UsageStats
	Records []UsageLog
}

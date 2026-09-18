package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/moneyfmt"
	"xiaodou/dai/libs/go/httpx"
)

type usageSummaryInput struct {
	TenantID      string `query:"tenant_id" doc:"租户 ID；为空表示全部租户"`
	UserID        string `query:"user_id" doc:"用户 ID；为空表示全部用户"`
	ModelCode     string `query:"model_code" doc:"模型编码过滤"`
	RequestSource string `query:"request_source" doc:"请求来源过滤"`
	DateFrom      string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo        string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
}

type usageLogsInput struct {
	ErrorsOnly    bool   `query:"errors_only" doc:"仅返回明确请求错误，不含客户端断开或已恢复的重试错误"`
	TenantID      string `query:"tenant_id" doc:"租户 ID；为空表示全部租户"`
	TenantName    string `query:"tenant_name" doc:"租户名称模糊过滤"`
	UserID        string `query:"user_id" doc:"用户 ID；为空表示全部用户"`
	UserName      string `query:"user_name" doc:"用户名模糊过滤"`
	ModelCode     string `query:"model_code" doc:"模型编码过滤"`
	RequestSource string `query:"request_source" doc:"请求来源过滤"`
	DateFrom      string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo        string `query:"date_to" doc:"结束时间，RFC3339"`
	Limit         int32  `query:"limit" default:"20" doc:"返回条数；默认 20，最大 100"`
	Offset        int32  `query:"offset" default:"0" doc:"偏移量；默认 0"`
}

type dailyTrendInput struct {
	DateFrom string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo   string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
}

type tenantUsageLogDTO struct {
	ID                                 string  `json:"id" doc:"用量日志 ID"`
	RequestID                          string  `json:"request_id" doc:"请求 ID"`
	RequestSource                      string  `json:"request_source" doc:"请求来源"`
	TenantID                           string  `json:"tenant_id" doc:"租户 ID"`
	UserID                             *string `json:"user_id,omitempty" doc:"用户 ID"`
	Username                           string  `json:"username,omitempty" doc:"用户名称"`
	ExternalUserID                     *string `json:"external_user_id,omitempty" doc:"外部用户 ID"`
	APIKeyID                           string  `json:"api_key_id,omitempty" doc:"API key ID；仅返回非敏感标识"`
	APIKeyName                         string  `json:"api_key_name,omitempty" doc:"API key 名称"`
	APIKeyLastFour                     *string `json:"api_key_last_four,omitempty" doc:"API key 后四位"`
	GroupID                            string  `json:"group_id,omitempty" doc:"命中分组 ID"`
	GroupNameSnapshot                  string  `json:"group_name_snapshot,omitempty" doc:"请求时分组名称快照"`
	GroupDefaultUserMultiplierSnapshot float64 `json:"group_default_user_multiplier_snapshot,omitempty" doc:"请求时分组用户默认倍率"`
	EffectiveUserMultiplierSnapshot    float64 `json:"effective_user_multiplier_snapshot,omitempty" doc:"请求时最终倍率快照"`
	BillingGroupLabelSnapshot          string  `json:"billing_group_label_snapshot,omitempty" doc:"请求时分组展示标签快照"`
	ModelCode                          string  `json:"model_code" doc:"模型编码"`
	CapabilityType                     string  `json:"capability_type" doc:"能力类型"`
	Stream                             bool    `json:"stream" doc:"是否流式请求"`
	PromptTokens                       int32   `json:"prompt_tokens" doc:"输入 token 数"`
	CompletionTokens                   int32   `json:"completion_tokens" doc:"输出 token 数"`
	CacheWriteTokens                   int32   `json:"cache_write_tokens" doc:"缓存写 token 数"`
	CacheReadTokens                    int32   `json:"cache_read_tokens" doc:"缓存读 token 数"`
	ReasoningTokens                    int32   `json:"reasoning_tokens" doc:"推理 token 数"`
	ReasoningEffort                    *string `json:"reasoning_effort,omitempty" doc:"客户端请求推理强度（归一化：low/medium/high/xhigh/max）"`
	TotalTokens                        int32   `json:"total_tokens" doc:"总 token 数"`
	TenantPayableUSD                   float64 `json:"tenant_payable_usd" doc:"租户应付平台的结算USD 金额"`
	RetailBaseUSD                      float64 `json:"retail_base_usd" doc:"分组零售价格表原价USD 金额"`
	UserPayableUSD                     float64 `json:"user_payable_usd" doc:"用户零售应收USD 金额"`
	UserChargedUSD                     float64 `json:"user_charged_usd" doc:"用户实际扣款USD 金额"`
	ServiceTier                        string  `json:"service_tier" doc:"服务档位：standard/fast"`
	BillingStatus                      string  `json:"billing_status" doc:"计费状态"`
	BillingStatusLabel                 string  `json:"billing_status_label" doc:"计费状态展示名"`
	RefundStatus                       string  `json:"refund_status" doc:"退款状态：none/refunded"`
	BillingSource                      string  `json:"billing_source" doc:"计费来源：payg=按量 / subscription=订阅内"`
	RequestStatus                      string  `json:"-" doc:"请求状态"`
	ProviderTerminalState              string  `json:"provider_terminal_state" doc:"上游终态"`
	ClientDeliveryState                string  `json:"client_delivery_state" doc:"客户端交付状态"`
	CancellationOrigin                 string  `json:"cancellation_origin" doc:"取消来源"`
	BillingReason                      string  `json:"billing_reason,omitempty" doc:"计费判定原因"`
	ResponseSummaryState               string  `json:"response_summary_state" doc:"响应摘要状态"`
	HTTPStatus                         *int32  `json:"http_status,omitempty" doc:"HTTP 状态码"`
	LatencyMs                          *int32  `json:"latency_ms,omitempty" doc:"请求延迟，毫秒"`
	FirstTokenLatencyMs                *int32  `json:"first_token_latency_ms,omitempty" doc:"首 token 延迟，毫秒"`
	RequestTotalMs                     *int32  `json:"request_total_ms,omitempty" doc:"请求总耗时，毫秒"`
	FirstResponseByteMs                *int32  `json:"first_response_byte_ms,omitempty" doc:"首个响应字节耗时，毫秒"`
	ErrorCode                          *string `json:"error_code,omitempty" doc:"错误码"`
	ErrorMessage                       *string `json:"error_message,omitempty" doc:"错误信息"`
	CreatedAt                          *int64  `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
}

type userUsageLogDTO struct {
	ID                              string  `json:"id" doc:"用量日志 ID"`
	RequestID                       string  `json:"request_id" doc:"请求 ID"`
	TraceID                         *string `json:"trace_id,omitempty" doc:"Trace ID"`
	TenantID                        string  `json:"tenant_id" doc:"租户 ID"`
	UserID                          *string `json:"user_id,omitempty" doc:"用户 ID"`
	RequestSource                   string  `json:"request_source" doc:"请求来源"`
	GroupID                         string  `json:"group_id,omitempty" doc:"命中分组 ID"`
	GroupNameSnapshot               string  `json:"group_name_snapshot,omitempty" doc:"请求时分组名称快照"`
	EffectiveUserMultiplierSnapshot float64 `json:"effective_user_multiplier_snapshot,omitempty" doc:"请求时最终倍率快照"`
	BillingGroupLabelSnapshot       string  `json:"billing_group_label_snapshot,omitempty" doc:"请求时分组展示标签快照"`
	ModelCode                       string  `json:"model_code" doc:"模型编码"`
	Stream                          bool    `json:"stream" doc:"是否流式请求"`
	PromptTokens                    int32   `json:"prompt_tokens" doc:"输入 token 数"`
	CompletionTokens                int32   `json:"completion_tokens" doc:"输出 token 数"`
	CacheWriteTokens                int32   `json:"cache_write_tokens" doc:"缓存写 token 数"`
	CacheReadTokens                 int32   `json:"cache_read_tokens" doc:"缓存读 token 数"`
	ReasoningTokens                 int32   `json:"reasoning_tokens" doc:"推理 token 数"`
	ReasoningEffort                 *string `json:"reasoning_effort,omitempty" doc:"客户端请求推理强度（归一化：low/medium/high/xhigh/max）"`
	TotalTokens                     int32   `json:"total_tokens" doc:"总 token 数"`
	BillableUnitType                string  `json:"billable_unit_type" doc:"计费单位类型"`
	BillableUnits                   int64   `json:"billable_units" doc:"计费单位数"`
	UserChargedUSD                  float64 `json:"user_charged_usd" doc:"用户实际扣款USD 金额"`
	ServiceTier                     string  `json:"service_tier" doc:"服务档位：standard/fast"`
	BillingStatus                   string  `json:"billing_status" doc:"计费状态"`
	RefundStatus                    string  `json:"refund_status" doc:"退款状态：none/refunded"`
	BillingSource                   string  `json:"billing_source" doc:"计费来源：payg=按量 / subscription=订阅内"`
	RequestStatus                   string  `json:"-" doc:"请求状态"`
	ProviderTerminalState           string  `json:"provider_terminal_state" doc:"上游终态"`
	ClientDeliveryState             string  `json:"client_delivery_state" doc:"客户端交付状态"`
	CancellationOrigin              string  `json:"cancellation_origin" doc:"取消来源"`
	BillingReason                   string  `json:"billing_reason,omitempty" doc:"计费判定原因"`
	ResponseSummaryState            string  `json:"response_summary_state" doc:"响应摘要状态"`
	HTTPStatus                      *int32  `json:"http_status,omitempty" doc:"HTTP 状态码"`
	LatencyMs                       *int32  `json:"latency_ms,omitempty" doc:"请求延迟，毫秒"`
	FirstTokenLatencyMs             *int32  `json:"first_token_latency_ms,omitempty" doc:"首 token 延迟，毫秒"`
	ErrorCode                       *string `json:"error_code,omitempty" doc:"错误码"`
	ErrorMessage                    *string `json:"error_message,omitempty" doc:"错误信息"`
	CreatedAt                       *int64  `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
}

type userUsageSummaryDTO struct {
	RequestCount          int64   `json:"request_count" doc:"请求数"`
	SuccessRequests       int64   `json:"success_requests" doc:"成功请求数"`
	FailedRequests        int64   `json:"failed_requests" doc:"失败请求数"`
	TotalTokens           int64   `json:"total_tokens" doc:"总 token 数"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens" doc:"输入 token 数"`
	TotalCompletionTokens int64   `json:"total_completion_tokens" doc:"输出 token 数"`
	TotalUserChargedUSD   float64 `json:"total_user_charged_usd" doc:"用户实际扣款USD 金额"`
	AvgLatencyMs          float64 `json:"avg_latency_ms" doc:"平均延迟，毫秒"`
}

type userUsageSummaryOutput struct {
	Body userUsageSummaryDTO
}

type usageSummaryRowDTO struct {
	ModelCode             string  `json:"model_code" doc:"模型编码"`
	RequestCount          int64   `json:"request_count" doc:"请求数"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens" doc:"输入 token 数"`
	TotalCompletionTokens int64   `json:"total_completion_tokens" doc:"输出 token 数"`
	CacheReadTokens       *int64  `json:"cache_read_tokens,omitempty" doc:"缓存读 token 数"`
	CacheWriteTokens      *int64  `json:"cache_write_tokens,omitempty" doc:"缓存写 token 数"`
	TotalTokens           int64   `json:"total_tokens" doc:"总 token 数"`
	TotalCatalogBaseUSD   float64 `json:"total_catalog_base_usd" doc:"目录基准价USD 金额（倍率1，谁都不付这个数）"`
	TotalTenantPayableUSD float64 `json:"total_tenant_payable_usd" doc:"平台向租户应收USD 金额"`
	TotalRetailBaseUSD    float64 `json:"total_retail_base_usd" doc:"零售价格表原价USD 金额"`
	TotalUserPayableUSD   float64 `json:"total_user_payable_usd" doc:"用户零售应收USD 金额"`
	TotalUserChargedUSD   float64 `json:"total_user_charged_usd" doc:"用户实际扣款USD 金额"`
	TotalQuotaUSD         float64 `json:"total_quota_usd" doc:"API key 配额USD 金额"`
}

type usageSummaryOutput struct {
	Body struct {
		Items []usageSummaryRowDTO `json:"items"`
		Total int                  `json:"total"`
	}
}

type usageUnitSummaryRowDTO struct {
	BillableUnitType      string  `json:"billable_unit_type" doc:"计费单位类型"`
	RequestCount          int64   `json:"request_count" doc:"请求数"`
	TotalBillableUnits    int64   `json:"total_billable_units" doc:"总计费单位数"`
	TotalCatalogBaseUSD   float64 `json:"total_catalog_base_usd"`
	TotalTenantPayableUSD float64 `json:"total_tenant_payable_usd"`
	TotalRetailBaseUSD    float64 `json:"total_retail_base_usd"`
	TotalUserPayableUSD   float64 `json:"total_user_payable_usd"`
	TotalUserChargedUSD   float64 `json:"total_user_charged_usd"`
}

type usageUnitSummaryOutput struct {
	Body struct {
		Items []usageUnitSummaryRowDTO `json:"items"`
		Total int                      `json:"total"`
	}
}

type usageUpstreamSummaryRowDTO struct {
	TargetKind            string  `json:"target_kind" doc:"上游资源类型：direct_upstream 或 oauth_pool"`
	TargetID              string  `json:"target_id" doc:"上游账号或凭证池 ID"`
	TargetName            string  `json:"target_name" doc:"上游账号或凭证池名称；资源已删除时为空，此时用 provider_code 或 ID 兜底展示"`
	ProviderCode          string  `json:"provider_code" doc:"请求时上游资源编码快照"`
	RequestCount          int64   `json:"request_count" doc:"请求数"`
	SuccessCount          int64   `json:"success_count" doc:"成功请求数"`
	FailedCount           int64   `json:"failed_count" doc:"失败请求数"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens" doc:"输入 token 合计"`
	TotalCompletionTokens int64   `json:"total_completion_tokens" doc:"输出 token 合计"`
	CacheReadTokens       *int64  `json:"cache_read_tokens,omitempty" doc:"缓存读 token 合计"`
	CacheWriteTokens      *int64  `json:"cache_write_tokens,omitempty" doc:"缓存写 token 合计"`
	TotalTokens           int64   `json:"total_tokens" doc:"总 token 数"`
	TokenUnits            int64   `json:"token_units" doc:"按 token 计费的计费单位合计"`
	ImageUnits            int64   `json:"image_units" doc:"生成图片张数合计"`
	CatalogBaseUSD        float64 `json:"catalog_base_usd" doc:"按上游资源价格表计算的参考费用"`
	TenantPayableUSD      float64 `json:"tenant_payable_usd" doc:"平台向租户结算的应收金额"`
	LastRequestedAt       *int64  `json:"last_requested_at,omitempty" doc:"最近请求时间，Unix 毫秒"`
}

type usageUpstreamSummaryOutput struct {
	Body struct {
		Items []usageUpstreamSummaryRowDTO `json:"items"`
		Total int                          `json:"total"`
	}
}

type usageUserRankingInput struct {
	TenantID      string `query:"tenant_id" doc:"租户 ID；为空表示全部租户"`
	UserID        string `query:"user_id" doc:"用户 ID；为空表示全部用户"`
	ModelCode     string `query:"model_code" doc:"模型编码过滤"`
	RequestSource string `query:"request_source" doc:"请求来源过滤"`
	DateFrom      string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo        string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
	Limit         int32  `query:"limit" default:"50" doc:"返回条数；默认 50，最大 100"`
}

type usageUserRankingRowDTO struct {
	TenantID            string  `json:"tenant_id" doc:"租户 ID"`
	UserID              string  `json:"user_id" doc:"用户 ID"`
	RequestCount        int64   `json:"request_count" doc:"请求数"`
	SuccessCount        int64   `json:"success_count" doc:"成功请求数"`
	FailedCount         int64   `json:"failed_count" doc:"失败请求数"`
	TotalTokens         int64   `json:"total_tokens" doc:"总 token 数"`
	TotalUserChargedUSD float64 `json:"total_user_charged_usd" doc:"用户实际扣款USD 金额"`
	LastRequestedAt     *int64  `json:"last_requested_at,omitempty" doc:"最近请求时间，Unix 毫秒"`
}

type usageUserRankingOutput struct {
	Body struct {
		Items    []usageUserRankingRowDTO `json:"items"`
		Total    int                      `json:"total"`
		Included IdentityIncludedDTO      `json:"included"`
	}
}

type dailyTrendRowDTO struct {
	Date                   string  `json:"date" doc:"日期，UTC yyyy-mm-dd"`
	RequestCount           int64   `json:"request_count" doc:"请求数"`
	SuccessCount           int64   `json:"success_count" doc:"成功请求数"`
	FailedCount            int64   `json:"failed_count" doc:"失败请求数"`
	TotalTokens            int64   `json:"total_tokens" doc:"总 token 数"`
	PromptTokens           int64   `json:"prompt_tokens" doc:"输入 token 数"`
	CompletionTokens       int64   `json:"completion_tokens" doc:"输出 token 数"`
	CacheReadTokens        *int64  `json:"cache_read_tokens,omitempty" doc:"缓存读 token 数"`
	CacheWriteTokens       *int64  `json:"cache_write_tokens,omitempty" doc:"缓存写 token 数"`
	CatalogBaseUSD         float64 `json:"catalog_base_usd"`
	TenantPayableUSD       float64 `json:"tenant_payable_usd"`
	RetailBaseUSD          float64 `json:"retail_base_usd"`
	UserPayableUSD         float64 `json:"user_payable_usd"`
	UserChargedUSD         float64 `json:"user_charged_usd"`
	AvgLatencyMs           int64   `json:"avg_latency_ms" doc:"平均延迟，毫秒"`
	AvgRequestTotalMs      int64   `json:"avg_request_total_ms" doc:"平均总耗时，毫秒"`
	AvgFirstResponseByteMs int64   `json:"avg_first_response_byte_ms" doc:"平均首个响应字节耗时，毫秒"`
}

type dailyTrendOutput struct {
	Body struct {
		Items []dailyTrendRowDTO `json:"items"`
		Total int                `json:"total"`
	}
}

func registerUsage(api huma.API, d UsageHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-daily-trend",
		Method:      http.MethodGet,
		Path:        "/api/v1/analytics/daily-trend",
		Summary:     "每日用量趋势",
		Description: "返回按自然日聚合的用量趋势，支持精确的 [start, end) 时间窗口；未传时默认近 30 天。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *dailyTrendInput) (*dailyTrendOutput, error) {
		if d.UsageQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("usage service is not configured")
		}
		dateFrom, dateTo, err := parseOptionalRFC3339Window(in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		dateFrom, dateTo = applyDefaultRFC3339Window(dateFrom, dateTo, 30*24*time.Hour)
		rows, err := d.UsageQueries.DailyTrend(ctx, dateFrom, dateTo)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &dailyTrendOutput{}
		out.Body.Items = make([]dailyTrendRowDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, dailyTrendRowToDTO(row))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-usage-summary",
		Method:      http.MethodGet,
		Path:        "/api/v1/usage-summary",
		Summary:     "用量模型汇总",
		Description: "按模型聚合 AI 网关用量，支持精确的 [start, end) 时间窗口。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *usageSummaryInput) (*usageSummaryOutput, error) {
		if d.UsageQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("usage service is not configured")
		}
		filter, err := usageSummaryFilterFromInput(in)
		if err != nil {
			return nil, err
		}
		rows, err := d.UsageQueries.Summary(ctx, filter)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &usageSummaryOutput{}
		out.Body.Items = make([]usageSummaryRowDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, usageSummaryRowToDTO(row))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-usage-unit-summary",
		Method:      http.MethodGet,
		Path:        "/api/v1/usage-unit-summary",
		Summary:     "用量计费单位汇总",
		Description: "按计费单位类型聚合 AI 网关用量，支持精确的 [start, end) 时间窗口。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *usageSummaryInput) (*usageUnitSummaryOutput, error) {
		if d.UsageQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("usage service is not configured")
		}
		filter, err := usageSummaryFilterFromInput(in)
		if err != nil {
			return nil, err
		}
		rows, err := d.UsageQueries.UnitSummary(ctx, filter)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &usageUnitSummaryOutput{}
		out.Body.Items = make([]usageUnitSummaryRowDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, usageUnitSummaryRowToDTO(row))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-usage-upstream-summary",
		Method:      http.MethodGet,
		Path:        "/api/v1/usage-upstream-summary",
		Summary:     "上游资源参考费用汇总",
		Description: "按实际命中的上游账号或凭证池聚合参考费用和租户结算应收，使用与 usage log 相同的结算结果。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *usageSummaryInput) (*usageUpstreamSummaryOutput, error) {
		if d.UsageQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("usage service is not configured")
		}
		filter, err := usageSummaryFilterFromInput(in)
		if err != nil {
			return nil, err
		}
		rows, err := d.UsageQueries.UpstreamSummary(ctx, filter)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &usageUpstreamSummaryOutput{}
		out.Body.Items = make([]usageUpstreamSummaryRowDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, usageUpstreamSummaryRowToDTO(row))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-usage-user-ranking",
		Method:      http.MethodGet,
		Path:        "/api/v1/usage-ranking/users",
		Summary:     "用量用户排行",
		Description: "按用户计费USD 金额降序返回用户排行，支持精确的 [start, end) 时间窗口和使用记录筛选口径。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *usageUserRankingInput) (*usageUserRankingOutput, error) {
		if d.UsageQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("usage service is not configured")
		}
		filter, err := usageSummaryFilterFromInput(&usageSummaryInput{
			TenantID:      in.TenantID,
			UserID:        in.UserID,
			ModelCode:     in.ModelCode,
			RequestSource: in.RequestSource,
			DateFrom:      in.DateFrom,
			DateTo:        in.DateTo,
		})
		if err != nil {
			return nil, err
		}
		limit, err := userUsageLimitFromInput(in.Limit)
		if err != nil {
			return nil, err
		}
		rows, err := d.UsageQueries.UserRanking(ctx, filter, limit)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &usageUserRankingOutput{}
		out.Body.Items = make([]usageUserRankingRowDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, usageUserRankingRowToDTO(row))
		}
		out.Body.Total = len(out.Body.Items)
		out.Body.Included = buildIdentityIncludedForRanking(ctx, d.IdentityProvider, d.IdentityEnrichmentFailures, rows)
		return out, nil
	})
}

func usageLogFilterFromInput(in *usageLogsInput) (domain.UsageFilter, error) {
	if in == nil {
		in = &usageLogsInput{}
	}
	dateFrom, dateTo, err := parseOptionalRFC3339Window(in.DateFrom, in.DateTo)
	if err != nil {
		return domain.UsageFilter{}, err
	}
	return domain.UsageFilter{
		ErrorsOnly:    in.ErrorsOnly,
		TenantID:      in.TenantID,
		TenantName:    in.TenantName,
		UserID:        in.UserID,
		UserName:      in.UserName,
		ModelCode:     in.ModelCode,
		RequestStatus: "",
		RequestSource: in.RequestSource,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
	}, nil
}

func userUsageLimitFromInput(limit int32) (int32, error) {
	if limit <= 0 {
		return 0, httpx.ErrBadRequest.WithDetail("invalid limit")
	}
	if limit > 100 {
		return 100, nil
	}
	return limit, nil
}

func usageSummaryFilterFromInput(in *usageSummaryInput) (domain.UsageSummaryFilter, error) {
	if in == nil {
		in = &usageSummaryInput{}
	}
	dateFrom, dateTo, err := parseOptionalRFC3339Window(in.DateFrom, in.DateTo)
	if err != nil {
		return domain.UsageSummaryFilter{}, err
	}
	return domain.UsageSummaryFilter{
		TenantID:      in.TenantID,
		UserID:        in.UserID,
		ModelCode:     in.ModelCode,
		RequestStatus: "",
		RequestSource: in.RequestSource,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
	}, nil
}

func parseOptionalRFC3339(value string, field string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithDetail("invalid " + field + ": expected RFC3339").WithCause(err)
	}
	utc := t.UTC()
	return &utc, nil
}

// billingSourceOrDefault 兜底遗留空值（批 A 前的历史行）为 payg，与写路径 coalesce 一致。
func billingSourceOrDefault(v string) string {
	if v == "" {
		return "payg"
	}
	return v
}

func tenantUsageLogToDTO(log domain.UsageLog) tenantUsageLogDTO {
	return tenantUsageLogDTO{
		ID:                                 log.ID,
		RequestID:                          log.RequestID,
		RequestSource:                      log.RequestSource,
		TenantID:                           log.TenantID,
		UserID:                             stringPtrOrNil(log.UserID),
		Username:                           log.Username,
		ExternalUserID:                     stringPtrOrNil(log.ExternalUserID),
		APIKeyID:                           log.APIKeyID,
		APIKeyName:                         log.APIKeyName,
		APIKeyLastFour:                     stringPtrOrNil(log.APIKeyLastFour),
		GroupID:                            log.GroupID,
		GroupNameSnapshot:                  log.GroupNameSnapshot,
		GroupDefaultUserMultiplierSnapshot: log.GroupDefaultUserMultiplierSnapshot,
		EffectiveUserMultiplierSnapshot:    log.EffectiveUserMultiplierSnapshot,
		BillingGroupLabelSnapshot:          log.BillingGroupLabelSnapshot,
		ModelCode:                          log.ModelCode,
		CapabilityType:                     log.CapabilityType,
		Stream:                             log.Stream,
		PromptTokens:                       log.PromptTokens,
		CompletionTokens:                   log.CompletionTokens,
		CacheWriteTokens:                   log.CacheWriteTokens,
		CacheReadTokens:                    log.CacheReadTokens,
		ReasoningTokens:                    log.ReasoningTokens,
		ReasoningEffort:                    stringPtrOrNil(log.ReasoningEffort),
		TotalTokens:                        log.TotalTokens,
		TenantPayableUSD:                   moneyfmt.MicroToUSD(log.TenantPayableMicro),
		RetailBaseUSD:                      moneyfmt.MicroToUSD(log.RetailBaseMicro),
		UserPayableUSD:                     moneyfmt.MicroToUSD(log.UserPayableMicro),
		UserChargedUSD:                     moneyfmt.MicroToUSD(log.UserChargedMicro),
		ServiceTier:                        log.ServiceTier,
		BillingStatus:                      log.BillingStatus,
		BillingStatusLabel:                 billingStatusLabel(log.BillingStatus),
		RefundStatus:                       log.RefundStatus,
		BillingSource:                      billingSourceOrDefault(log.BillingSource),
		RequestStatus:                      log.RequestStatus,
		ProviderTerminalState:              log.ProviderTerminalState,
		ClientDeliveryState:                log.ClientDeliveryState,
		CancellationOrigin:                 log.CancellationOrigin,
		BillingReason:                      log.BillingReason,
		ResponseSummaryState:               log.ResponseSummaryState,
		HTTPStatus:                         log.HTTPStatus,
		LatencyMs:                          log.LatencyMs,
		FirstTokenLatencyMs:                log.FirstTokenLatencyMs,
		RequestTotalMs:                     log.RequestTotalMs,
		FirstResponseByteMs:                log.FirstResponseByteMs,
		ErrorCode:                          stringPtrOrNil(log.ErrorCode),
		ErrorMessage:                       stringPtrOrNil(log.ErrorMessage),
		CreatedAt:                          timeToMillisPtr(log.CreatedAt),
	}
}

func userUsageLogToDTO(row domain.UsageLog) userUsageLogDTO {
	return userUsageLogDTO{
		ID:                              row.ID,
		RequestID:                       row.RequestID,
		TraceID:                         stringPtrOrNil(row.TraceID),
		TenantID:                        row.TenantID,
		UserID:                          stringPtrOrNil(row.UserID),
		RequestSource:                   row.RequestSource,
		GroupID:                         row.GroupID,
		GroupNameSnapshot:               row.GroupNameSnapshot,
		EffectiveUserMultiplierSnapshot: row.EffectiveUserMultiplierSnapshot,
		BillingGroupLabelSnapshot:       row.BillingGroupLabelSnapshot,
		ModelCode:                       row.ModelCode,
		Stream:                          row.Stream,
		PromptTokens:                    row.PromptTokens,
		CompletionTokens:                row.CompletionTokens,
		CacheWriteTokens:                row.CacheWriteTokens,
		CacheReadTokens:                 row.CacheReadTokens,
		ReasoningTokens:                 row.ReasoningTokens,
		ReasoningEffort:                 stringPtrOrNil(row.ReasoningEffort),
		TotalTokens:                     row.TotalTokens,
		BillableUnitType:                row.BillableUnitType,
		BillableUnits:                   row.BillableUnits,
		UserChargedUSD:                  moneyfmt.MicroToUSD(row.UserChargedMicro),
		ServiceTier:                     row.ServiceTier,
		BillingStatus:                   row.BillingStatus,
		RefundStatus:                    row.RefundStatus,
		BillingSource:                   billingSourceOrDefault(row.BillingSource),
		RequestStatus:                   row.RequestStatus,
		ProviderTerminalState:           row.ProviderTerminalState,
		ClientDeliveryState:             row.ClientDeliveryState,
		CancellationOrigin:              row.CancellationOrigin,
		BillingReason:                   row.BillingReason,
		ResponseSummaryState:            row.ResponseSummaryState,
		HTTPStatus:                      row.HTTPStatus,
		LatencyMs:                       row.LatencyMs,
		FirstTokenLatencyMs:             row.FirstTokenLatencyMs,
		ErrorCode:                       stringPtrOrNil(row.ErrorCode),
		ErrorMessage:                    stringPtrOrNil(row.ErrorMessage),
		CreatedAt:                       timeToMillisPtr(row.CreatedAt),
	}
}

func userUsageSummaryToDTO(summary domain.UserUsageSummary) userUsageSummaryDTO {
	return userUsageSummaryDTO{
		RequestCount:          summary.RequestCount,
		SuccessRequests:       summary.SuccessRequests,
		FailedRequests:        summary.FailedRequests,
		TotalTokens:           summary.TotalTokens,
		TotalPromptTokens:     summary.TotalPromptTokens,
		TotalCompletionTokens: summary.TotalCompletionTokens,
		TotalUserChargedUSD:   moneyfmt.MicroToUSD(summary.TotalUserChargedMicro),
		AvgLatencyMs:          summary.AvgLatencyMs,
	}
}

func billingStatusLabel(status string) string {
	switch status {
	case "pending":
		return "待结算"
	case "settled":
		return "已结算"
	case "failed":
		return "结算失败"
	case "free":
		return "免费"
	case "void":
		return "未计费"
	default:
		return status
	}
}

func dailyTrendRowToDTO(row domain.DailyTrendRow) dailyTrendRowDTO {
	return dailyTrendRowDTO{
		Date:                   row.Date,
		RequestCount:           row.RequestCount,
		SuccessCount:           row.SuccessCount,
		FailedCount:            row.FailedCount,
		TotalTokens:            row.TotalTokens,
		PromptTokens:           row.PromptTokens,
		CompletionTokens:       row.CompletionTokens,
		CacheReadTokens:        int64Ptr(row.CacheReadTokens),
		CacheWriteTokens:       int64Ptr(row.CacheWriteTokens),
		CatalogBaseUSD:         moneyfmt.MicroToUSD(row.CatalogBaseMicro),
		TenantPayableUSD:       moneyfmt.MicroToUSD(row.TenantPayableMicro),
		RetailBaseUSD:          moneyfmt.MicroToUSD(row.RetailBaseMicro),
		UserPayableUSD:         moneyfmt.MicroToUSD(row.UserPayableMicro),
		UserChargedUSD:         moneyfmt.MicroToUSD(row.UserChargedMicro),
		AvgLatencyMs:           row.AvgLatencyMs,
		AvgRequestTotalMs:      row.AvgRequestTotalMs,
		AvgFirstResponseByteMs: row.AvgFirstResponseByteMs,
	}
}

func usageSummaryRowToDTO(row domain.UsageSummaryRow) usageSummaryRowDTO {
	return usageSummaryRowDTO{
		ModelCode:             row.ModelCode,
		RequestCount:          row.RequestCount,
		TotalPromptTokens:     row.TotalPromptTokens,
		TotalCompletionTokens: row.TotalCompletionTokens,
		CacheReadTokens:       int64Ptr(row.CacheReadTokens),
		CacheWriteTokens:      int64Ptr(row.CacheWriteTokens),
		TotalTokens:           row.TotalTokens,
		TotalCatalogBaseUSD:   moneyfmt.MicroToUSD(row.TotalCatalogBaseMicro),
		TotalTenantPayableUSD: moneyfmt.MicroToUSD(row.TotalTenantPayableMicro),
		TotalRetailBaseUSD:    moneyfmt.MicroToUSD(row.TotalRetailBaseMicro),
		TotalUserPayableUSD:   moneyfmt.MicroToUSD(row.TotalUserPayableMicro),
		TotalUserChargedUSD:   moneyfmt.MicroToUSD(row.TotalUserChargedMicro),
		TotalQuotaUSD:         moneyfmt.MicroToUSD(row.TotalQuotaCostMicro),
	}
}

func usageUserRankingRowToDTO(row domain.UsageUserRankingRow) usageUserRankingRowDTO {
	return usageUserRankingRowDTO{
		TenantID:            row.TenantID,
		UserID:              row.UserID,
		RequestCount:        row.RequestCount,
		SuccessCount:        row.SuccessCount,
		FailedCount:         row.FailedCount,
		TotalTokens:         row.TotalTokens,
		TotalUserChargedUSD: moneyfmt.MicroToUSD(row.TotalUserChargedMicro),
		LastRequestedAt:     timeToMillisPtr(row.LastRequestedAt),
	}
}

func usageUnitSummaryRowToDTO(row domain.UsageUnitSummaryRow) usageUnitSummaryRowDTO {
	return usageUnitSummaryRowDTO{
		BillableUnitType:      row.BillableUnitType,
		RequestCount:          row.RequestCount,
		TotalBillableUnits:    row.TotalBillableUnits,
		TotalCatalogBaseUSD:   moneyfmt.MicroToUSD(row.TotalCatalogBaseMicro),
		TotalTenantPayableUSD: moneyfmt.MicroToUSD(row.TotalTenantPayableMicro),
		TotalRetailBaseUSD:    moneyfmt.MicroToUSD(row.TotalRetailBaseMicro),
		TotalUserPayableUSD:   moneyfmt.MicroToUSD(row.TotalUserPayableMicro),
		TotalUserChargedUSD:   moneyfmt.MicroToUSD(row.TotalUserChargedMicro),
	}
}

func usageUpstreamSummaryRowToDTO(row domain.UsageUpstreamSummaryRow) usageUpstreamSummaryRowDTO {
	return usageUpstreamSummaryRowDTO{
		TargetKind:            row.TargetKind,
		TargetID:              row.TargetID,
		TargetName:            row.TargetName,
		ProviderCode:          row.ProviderCode,
		RequestCount:          row.RequestCount,
		SuccessCount:          row.SuccessCount,
		FailedCount:           row.FailedCount,
		TotalPromptTokens:     row.TotalPromptTokens,
		TotalCompletionTokens: row.TotalCompletionTokens,
		CacheReadTokens:       int64Ptr(row.CacheReadTokens),
		CacheWriteTokens:      int64Ptr(row.CacheWriteTokens),
		TotalTokens:           row.TotalTokens,
		TokenUnits:            row.TokenUnits,
		ImageUnits:            row.ImageUnits,
		CatalogBaseUSD:        moneyfmt.MicroToUSD(row.CatalogBaseMicro),
		TenantPayableUSD:      moneyfmt.MicroToUSD(row.TenantPayableMicro),
		LastRequestedAt:       timeToMillisPtr(row.LastRequestedAt),
	}
}

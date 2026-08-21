package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/moneyfmt"
	"xiaodou/dai/libs/go/httpx"
)

type dashboardFilterInput struct {
	TenantID string `query:"tenant_id" doc:"租户 ID；为空表示全部租户"`
	UserID   string `query:"user_id" doc:"用户 ID；为空表示全部用户"`
	DateFrom string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo   string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
}

type dashboardTopModelsInput struct {
	TenantID string `query:"tenant_id" doc:"租户 ID；为空表示全部租户"`
	UserID   string `query:"user_id" doc:"用户 ID；为空表示全部用户"`
	DateFrom string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo   string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
	Limit    int32  `query:"limit" default:"10" doc:"返回条数；默认 10，最大 50"`
}

type dashboardTopTenantsInput struct {
	TenantID string `query:"tenant_id" doc:"租户 ID；为空表示全部租户"`
	UserID   string `query:"user_id" doc:"用户 ID；为空表示全部用户"`
	DateFrom string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo   string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
	Limit    int32  `query:"limit" default:"10" doc:"返回条数；默认 10，最大 50"`
}

type dashboardRecentErrorsInput struct {
	TenantID string `query:"tenant_id" doc:"租户 ID；为空表示全部租户"`
	UserID   string `query:"user_id" doc:"用户 ID；为空表示全部用户"`
	DateFrom string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo   string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
	Limit    int32  `query:"limit" default:"10" doc:"返回条数；默认 10，最大 50"`
}

type dashboardSummaryDTO struct {
	TotalRequests          int64   `json:"total_requests" doc:"请求总数"`
	SuccessfulRequests     int64   `json:"successful_requests" doc:"成功请求数"`
	FailedRequests         int64   `json:"failed_requests" doc:"失败请求数"`
	TotalTokens            int64   `json:"total_tokens" doc:"总 token 数"`
	TotalPromptTokens      int64   `json:"total_prompt_tokens" doc:"输入 token 数"`
	TotalCompletionTokens  int64   `json:"total_completion_tokens" doc:"输出 token 数"`
	TotalCatalogBaseUSD    float64 `json:"total_catalog_base_usd" doc:"上游参考成本USD 金额"`
	TotalTenantPayableUSD  float64 `json:"total_tenant_payable_usd" doc:"平台向租户结算的USD 金额"`
	TotalRetailBaseUSD     float64 `json:"total_retail_base_usd" doc:"零售价格表原价USD 金额"`
	TotalUserPayableUSD    float64 `json:"total_user_payable_usd" doc:"用户零售应收USD 金额"`
	TotalUserChargedUSD    float64 `json:"total_user_charged_usd" doc:"用户实际扣款USD 金额"`
	AvgLatencyMs           float64 `json:"avg_latency_ms" doc:"平均延迟，毫秒"`
	AvgRequestTotalMs      float64 `json:"avg_request_total_ms" doc:"平均总耗时，毫秒"`
	AvgFirstResponseByteMs float64 `json:"avg_first_response_byte_ms" doc:"平均首个响应字节耗时，毫秒"`
}

type dashboardSummaryOutput struct {
	Body dashboardSummaryDTO
}

type dashboardTopModelDTO struct {
	ModelCode             string  `json:"model_code" doc:"模型编码"`
	RequestCount          int64   `json:"request_count" doc:"请求数"`
	TotalTokens           int64   `json:"total_tokens" doc:"总 token 数"`
	TotalTenantPayableUSD float64 `json:"total_tenant_payable_usd" doc:"租户扣费USD 金额"`
}

type dashboardTopModelsOutput struct {
	Body struct {
		Items []dashboardTopModelDTO `json:"items"`
		Total int                    `json:"total"`
	}
}

type dashboardTopTenantDTO struct {
	TenantID              string  `json:"tenant_id" doc:"租户 ID"`
	RequestCount          int64   `json:"request_count" doc:"请求数"`
	TotalTokens           int64   `json:"total_tokens" doc:"总 token 数"`
	TotalTenantPayableUSD float64 `json:"total_tenant_payable_usd" doc:"租户扣费USD 金额"`
}

type dashboardTopTenantsOutput struct {
	Body struct {
		Items    []dashboardTopTenantDTO `json:"items"`
		Total    int                     `json:"total"`
		Included IdentityIncludedDTO     `json:"included"`
	}
}

type dashboardRecentErrorDTO struct {
	RequestID                  string  `json:"request_id" doc:"请求 ID"`
	ModelCode                  string  `json:"model_code" doc:"最终逻辑模型"`
	RequestedModel             *string `json:"requested_model,omitempty" doc:"客户端请求模型名"`
	MatchedDispatchRuleSummary *string `json:"matched_dispatch_rule_summary,omitempty" doc:"命中的分组调度规则摘要"`
	ResolvedLogicalModel       *string `json:"resolved_logical_model,omitempty" doc:"调度后的逻辑模型"`
	ResolvedProviderFamily     *string `json:"resolved_provider_family,omitempty" doc:"最终实际选中的上游协议家族"`
	ClientAPIFormat            *string `json:"client_api_format,omitempty" doc:"客户端 API 格式"`
	ProviderAPIFormat          *string `json:"provider_api_format,omitempty" doc:"最终上游 API 格式"`
	UpstreamModel              *string `json:"upstream_model,omitempty" doc:"最终上游模型"`
	ProtocolConversionEnabled  bool    `json:"protocol_conversion_enabled" doc:"分组是否允许协议转换"`
	RequestStatus              string  `json:"request_status" doc:"请求状态"`
	ErrorCode                  *string `json:"error_code,omitempty" doc:"错误码"`
	ErrorMessage               *string `json:"error_message,omitempty" doc:"错误信息"`
	HTTPStatus                 *int32  `json:"http_status,omitempty" doc:"上游 HTTP 状态码"`
	CreatedAt                  *int64  `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
}

type dashboardRecentErrorsOutput struct {
	Body struct {
		Items []dashboardRecentErrorDTO `json:"items"`
		Total int                       `json:"total"`
	}
}

func registerDashboard(api huma.API, d DashboardHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-get-dashboard-summary",
		Method:      http.MethodGet,
		Path:        "/api/v1/dashboard/summary",
		Summary:     "仪表盘概览",
		Description: "返回 AI 网关仪表盘核心统计，支持精确的 [start, end) 时间窗口；未传时默认近 24 小时。",
		Tags:        []string{"dashboard"},
	}, func(ctx context.Context, in *dashboardFilterInput) (*dashboardSummaryOutput, error) {
		if d.DashboardQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("dashboard service is not configured")
		}
		filter, err := dashboardFilterFromInput(in.TenantID, in.UserID, in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		summary, err := d.DashboardQueries.Summary(ctx, filter)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &dashboardSummaryOutput{}
		out.Body = dashboardSummaryToDTO(summary)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-dashboard-top-models",
		Method:      http.MethodGet,
		Path:        "/api/v1/dashboard/top-models",
		Summary:     "仪表盘模型排行",
		Description: "返回按使用量排序的模型排行，支持精确的 [start, end) 时间窗口；未传时默认近 24 小时。",
		Tags:        []string{"dashboard"},
	}, func(ctx context.Context, in *dashboardTopModelsInput) (*dashboardTopModelsOutput, error) {
		if d.DashboardQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("dashboard service is not configured")
		}
		filter, err := dashboardFilterFromInput(in.TenantID, in.UserID, in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		limit, err := dashboardLimitFromInput(in.Limit)
		if err != nil {
			return nil, err
		}
		models, err := d.DashboardQueries.TopModels(ctx, filter, limit)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &dashboardTopModelsOutput{}
		out.Body.Items = make([]dashboardTopModelDTO, 0, len(models))
		for _, model := range models {
			out.Body.Items = append(out.Body.Items, dashboardTopModelToDTO(model))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-dashboard-top-tenants",
		Method:      http.MethodGet,
		Path:        "/api/v1/dashboard/top-tenants",
		Summary:     "仪表盘租户排行",
		Description: "返回按使用量排序的租户排行，支持精确的 [start, end) 时间窗口；未传时默认近 24 小时。",
		Tags:        []string{"dashboard"},
	}, func(ctx context.Context, in *dashboardTopTenantsInput) (*dashboardTopTenantsOutput, error) {
		if d.DashboardQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("dashboard service is not configured")
		}
		filter, err := dashboardFilterFromInput(in.TenantID, in.UserID, in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		limit, err := dashboardLimitFromInput(in.Limit)
		if err != nil {
			return nil, err
		}
		tenants, err := d.DashboardQueries.TopTenants(ctx, filter, limit)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &dashboardTopTenantsOutput{}
		out.Body.Items = make([]dashboardTopTenantDTO, 0, len(tenants))
		for _, tenant := range tenants {
			out.Body.Items = append(out.Body.Items, dashboardTopTenantToDTO(tenant))
		}
		out.Body.Total = len(out.Body.Items)
		out.Body.Included = buildIdentityIncludedForDashboardTenants(ctx, d.IdentityProvider, d.IdentityEnrichmentFailures, tenants)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-dashboard-recent-errors",
		Method:      http.MethodGet,
		Path:        "/api/v1/dashboard/recent-errors",
		Summary:     "仪表盘近期错误",
		Description: "返回近期失败请求，支持精确的 [start, end) 时间窗口；未传时默认近 24 小时。",
		Tags:        []string{"dashboard"},
	}, func(ctx context.Context, in *dashboardRecentErrorsInput) (*dashboardRecentErrorsOutput, error) {
		if d.DashboardQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("dashboard service is not configured")
		}
		filter, err := dashboardFilterFromInput(in.TenantID, in.UserID, in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		limit, err := dashboardLimitFromInput(in.Limit)
		if err != nil {
			return nil, err
		}
		errs, err := d.DashboardQueries.RecentErrors(ctx, filter, limit)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &dashboardRecentErrorsOutput{}
		out.Body.Items = make([]dashboardRecentErrorDTO, 0, len(errs))
		for _, item := range errs {
			out.Body.Items = append(out.Body.Items, dashboardRecentErrorToDTO(item))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

func dashboardFilterFromInput(tenantID string, userID string, dateFromValue string, dateToValue string) (domain.DashboardFilter, error) {
	dateFrom, dateTo, err := parseOptionalRFC3339Window(dateFromValue, dateToValue)
	if err != nil {
		return domain.DashboardFilter{}, err
	}
	dateFrom, dateTo = applyDefaultRFC3339Window(dateFrom, dateTo, 24*time.Hour)
	return domain.DashboardFilter{
		TenantID: tenantID,
		UserID:   userID,
		DateFrom: dateFrom,
		DateTo:   dateTo,
	}, nil
}

func dashboardLimitFromInput(limit int32) (int32, error) {
	if limit <= 0 {
		return 0, httpx.ErrBadRequest.WithDetail("invalid limit")
	}
	if limit > 50 {
		return 50, nil
	}
	return limit, nil
}

func dashboardSummaryToDTO(s domain.DashboardSummary) dashboardSummaryDTO {
	return dashboardSummaryDTO{
		TotalRequests:          s.TotalRequests,
		SuccessfulRequests:     s.SuccessfulRequests,
		FailedRequests:         s.FailedRequests,
		TotalTokens:            s.TotalTokens,
		TotalPromptTokens:      s.TotalPromptTokens,
		TotalCompletionTokens:  s.TotalCompletionTokens,
		TotalCatalogBaseUSD:    moneyfmt.MicroToUSD(s.TotalCatalogBaseMicro),
		TotalTenantPayableUSD:  moneyfmt.MicroToUSD(s.TotalTenantPayableMicro),
		TotalRetailBaseUSD:     moneyfmt.MicroToUSD(s.TotalRetailBaseMicro),
		TotalUserPayableUSD:    moneyfmt.MicroToUSD(s.TotalUserPayableMicro),
		TotalUserChargedUSD:    moneyfmt.MicroToUSD(s.TotalUserChargedMicro),
		AvgLatencyMs:           s.AvgLatencyMs,
		AvgRequestTotalMs:      s.AvgRequestTotalMs,
		AvgFirstResponseByteMs: s.AvgFirstResponseByteMs,
	}
}

func dashboardTopModelToDTO(model domain.DashboardTopModel) dashboardTopModelDTO {
	return dashboardTopModelDTO{
		ModelCode:             model.ModelCode,
		RequestCount:          model.RequestCount,
		TotalTokens:           model.TotalTokens,
		TotalTenantPayableUSD: moneyfmt.MicroToUSD(model.TotalTenantPayableMicro),
	}
}

func dashboardTopTenantToDTO(tenant domain.DashboardTopTenant) dashboardTopTenantDTO {
	return dashboardTopTenantDTO{
		TenantID:              tenant.TenantID,
		RequestCount:          tenant.RequestCount,
		TotalTokens:           tenant.TotalTokens,
		TotalTenantPayableUSD: moneyfmt.MicroToUSD(tenant.TotalTenantPayableMicro),
	}
}

func dashboardRecentErrorToDTO(item domain.DashboardRecentError) dashboardRecentErrorDTO {
	return dashboardRecentErrorDTO{
		RequestID:                  item.RequestID,
		ModelCode:                  item.ModelCode,
		RequestedModel:             stringPtrOrNil(item.RequestedModel),
		MatchedDispatchRuleSummary: stringPtrOrNil(item.MatchedDispatchRuleSummary),
		ResolvedLogicalModel:       stringPtrOrNil(item.ResolvedLogicalModel),
		ResolvedProviderFamily:     stringPtrOrNil(item.ResolvedProviderFamily),
		ClientAPIFormat:            stringPtrOrNil(item.ClientProtocol),
		ProviderAPIFormat:          stringPtrOrNil(item.SelectedUpstreamProtocol),
		UpstreamModel:              stringPtrOrNil(item.UpstreamModel),
		ProtocolConversionEnabled:  item.ProtocolConversionEnabled,
		RequestStatus:              item.RequestStatus,
		ErrorCode:                  stringPtrOrNil(item.ErrorCode),
		ErrorMessage:               stringPtrOrNil(item.ErrorMessage),
		HTTPStatus:                 item.HTTPStatus,
		CreatedAt:                  timeToMillisPtr(item.CreatedAt),
	}
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

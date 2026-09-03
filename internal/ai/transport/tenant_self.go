package transport

// tenant_self.go registers the tenant self-service read endpoints (userType=3)
// for dashboard and usage. API-key and limit-policy controls live in
// TenantSelfControlHTTPDeps. Every handler derives the tenant scope from JWT claims
// (tenantIDFromContext) — never from a path/query param — so a tenant can only
// ever read its own resources. These mirror the tenant branches of the
// role-dispatching console dashboard and usage envelope routes while speaking
// the flat Huma contract.

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

// ---------------------------------------------------------------------------
// inputs
// ---------------------------------------------------------------------------

type tenantSelfAPIKeyWriteInput struct {
	Body apiKeyWriteRequest
}

type tenantSelfUpdateAPIKeyInput struct {
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
	Body     apiKeyWriteRequest
}

type tenantSelfUpdateAPIKeyStatusInput struct {
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
	Body     apiKeyStatusRequest
}

type tenantSelfAPIKeyIDInput struct {
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
}

// tenantSelfDashboardInput / usage inputs intentionally omit tenant_id/user_id:
// the tenant scope is taken from claims, not the caller.

type tenantSelfDashboardInput struct {
	DateFrom string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo   string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
}

type tenantSelfDashboardListInput struct {
	DateFrom string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo   string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
	Limit    int32  `query:"limit" default:"10" doc:"返回条数；默认 10，最大 50"`
}

type tenantSelfUsageLogsInput struct {
	UserID        string `query:"user_id" doc:"用户 ID 过滤；为空表示本租户全部用户"`
	ModelCode     string `query:"model_code" doc:"模型编码过滤"`
	RequestStatus string `query:"request_status" doc:"请求状态过滤"`
	RequestSource string `query:"request_source" doc:"请求来源过滤"`
	DateFrom      string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo        string `query:"date_to" doc:"结束时间，RFC3339"`
	Limit         int32  `query:"limit" default:"20" doc:"返回条数；默认 20，最大 100"`
	Offset        int32  `query:"offset" default:"0" doc:"偏移量；默认 0"`
}

type tenantSelfUsageSummaryInput struct {
	UserID        string `query:"user_id" doc:"用户 ID 过滤；为空表示本租户全部用户"`
	ModelCode     string `query:"model_code" doc:"模型编码过滤"`
	RequestStatus string `query:"request_status" doc:"请求状态过滤"`
	RequestSource string `query:"request_source" doc:"请求来源过滤"`
	DateFrom      string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo        string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
}

// registerTenantSelf mounts the tenant self-service read endpoints under the
// tenant auth group (tenantUserAuth → userType=3).
func registerTenantSelf(api huma.API, d TenantSelfReadHTTPDeps) {
	registerTenantSelfDashboard(api, d)
	registerTenantSelfUsage(api, d)
}

// ---------------------------------------------------------------------------
// /api/v1/tenants/me/api-keys[/...]  (console: handleTenantsMeAPIKeys*)
// ---------------------------------------------------------------------------

func registerTenantSelfAPIKeyWrites(api huma.API, d TenantSelfControlHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID:   "ai-create-tenant-self-api-key",
		Method:        http.MethodPost,
		Path:          "/api/v1/tenants/me/api-keys",
		Summary:       "创建租户自助 API key",
		Description:   "按当前租户 token 创建租户拥有的 API key。响应返回新密钥明文，后续也可再次复制。",
		Tags:          []string{"api-keys"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *tenantSelfAPIKeyWriteInput) (*apiKeyCreatedOutput, error) {
		if d.APIKeyWriter == nil || d.APIKeyLifecycle == nil || d.Groups == nil || d.LimitPolicies == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		write, err := tenantAPIKeyCreateInput(tenantID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureAPIKeyGroupAccessible(ctx, d.Groups, tenantID, "", in.Body.GroupID); err != nil {
			return nil, mapServiceError(err)
		}
		if cb := strings.TrimSpace(claimsUserID(ctx)); cb != "" && write.CreatedBy == "" {
			write.CreatedBy = cb
		}
		created, err := d.APIKeyWriter.Create(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		limitPolicy, err := syncAPIKeyLimitPolicy(ctx, d.LimitPolicies, created.Key.ID, in.Body.LimitPolicy, write.CreatedBy)
		if err != nil {
			_ = d.APIKeyLifecycle.Delete(ctx, created.Key.ID, tenantID)
			return nil, mapServiceError(err)
		}
		out := &apiKeyCreatedOutput{}
		out.Body.PlaintextKey = created.PlaintextKey
		out.Body.Key = apiKeyToDTO(created.Key, limitPolicy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-tenant-self-api-key",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tenants/me/api-keys/{apiKeyID}",
		Summary:     "更新租户自助 API key",
		Description: "按当前租户 token 更新本租户 API key 的基础信息与独立限流策略。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *tenantSelfUpdateAPIKeyInput) (*apiKeyOutput, error) {
		if d.APIKeys == nil || d.APIKeyWriter == nil || d.Groups == nil || d.LimitPolicies == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		write, err := apiKeyUpdateInput(tenantID, in.APIKeyID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeys, tenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureAPIKeyGroupAccessible(ctx, d.Groups, tenantID, "", in.Body.GroupID); err != nil {
			return nil, mapServiceError(err)
		}
		key, err := d.APIKeyWriter.Update(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		limitPolicy, err := syncAPIKeyLimitPolicy(ctx, d.LimitPolicies, key.ID, in.Body.LimitPolicy, userIDFromContext(ctx))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyOutput{}
		out.Body = apiKeyToDTO(key, limitPolicy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-tenant-self-api-key-status",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tenants/me/api-keys/{apiKeyID}/status",
		Summary:     "更新租户自助 API key 状态",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *tenantSelfUpdateAPIKeyStatusInput) (*apiKeyOutput, error) {
		if d.APIKeys == nil || d.APIKeyLifecycle == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		status, err := apiKeyStatusInput(in.Body.Status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeys, tenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		key, err := d.APIKeyLifecycle.UpdateStatus(ctx, in.APIKeyID, tenantID, status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyOutput{}
		out.Body = apiKeyToDTO(key, nil)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-reveal-tenant-self-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/me/api-keys/{apiKeyID}/reveal",
		Summary:     "回显租户自助 API key 明文",
		Description: "按当前租户 token 读取本租户 API key 的当前完整明文，用于再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *tenantSelfAPIKeyIDInput) (*apiKeyRevealOutput, error) {
		if d.APIKeys == nil || d.APIKeySecrets == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeys, tenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		plaintext, err := d.APIKeySecrets.Reveal(ctx, in.APIKeyID, tenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyRevealOutput{}
		out.Body.PlaintextKey = plaintext
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-rotate-tenant-self-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/me/api-keys/{apiKeyID}/rotate",
		Summary:     "轮换租户自助 API key",
		Description: "按当前租户 token 为本租户 API key 生成新密钥并失效旧密钥缓存。响应返回新密钥明文，后续也可再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *tenantSelfAPIKeyIDInput) (*apiKeyCreatedOutput, error) {
		if d.APIKeys == nil || d.APIKeySecrets == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeys, tenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		created, err := d.APIKeySecrets.Rotate(ctx, in.APIKeyID, tenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyCreatedOutput{}
		out.Body.PlaintextKey = created.PlaintextKey
		out.Body.Key = apiKeyToDTO(created.Key, nil)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-tenant-self-api-key",
		Method:      http.MethodDelete,
		Path:        "/api/v1/tenants/me/api-keys/{apiKeyID}",
		Summary:     "删除租户自助 API key",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *tenantSelfAPIKeyIDInput) (*deleteAPIKeyOutput, error) {
		if d.APIKeys == nil || d.APIKeyLifecycle == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeys, tenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := d.APIKeyLifecycle.Delete(ctx, in.APIKeyID, tenantID); err != nil {
			return nil, mapServiceError(err)
		}
		if d.LimitPolicies != nil {
			_ = d.LimitPolicies.DeleteLimitPolicies(ctx, commercial.LimitPolicyFilter{
				ScopeType: commercial.LimitScopeAPIKey,
				ScopeID:   in.APIKeyID,
			})
		}
		out := &deleteAPIKeyOutput{}
		out.Body.Deleted = true
		return out, nil
	})
}

// ---------------------------------------------------------------------------
// tenant-scoped dashboard reads (claims-scoped). The platform-only huma
// dashboard endpoints under management cannot be reused by userType=3.
// ---------------------------------------------------------------------------

func registerTenantSelfDashboard(api huma.API, d TenantSelfReadHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-get-tenant-self-dashboard-summary",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/dashboard/summary",
		Summary:     "租户自助仪表盘概览",
		Description: "按当前租户 token 返回本租户维度的仪表盘核心统计，支持精确的 [start, end) 时间窗口。",
		Tags:        []string{"dashboard"},
	}, func(ctx context.Context, in *tenantSelfDashboardInput) (*dashboardSummaryOutput, error) {
		if d.DashboardQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("dashboard service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		filter, err := tenantSelfDashboardFilter(tenantID, in.DateFrom, in.DateTo)
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
		OperationID: "ai-list-tenant-self-dashboard-top-models",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/dashboard/top-models",
		Summary:     "租户自助仪表盘模型排行",
		Description: "按当前租户 token 返回本租户维度按使用量排序的模型排行，支持精确的 [start, end) 时间窗口。",
		Tags:        []string{"dashboard"},
	}, func(ctx context.Context, in *tenantSelfDashboardListInput) (*dashboardTopModelsOutput, error) {
		if d.DashboardQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("dashboard service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		filter, err := tenantSelfDashboardFilter(tenantID, in.DateFrom, in.DateTo)
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
		OperationID: "ai-list-tenant-self-dashboard-recent-errors",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/dashboard/recent-errors",
		Summary:     "租户自助仪表盘近期错误",
		Description: "按当前租户 token 返回本租户维度的近期失败请求，支持精确的 [start, end) 时间窗口。",
		Tags:        []string{"dashboard"},
	}, func(ctx context.Context, in *tenantSelfDashboardListInput) (*dashboardRecentErrorsOutput, error) {
		if d.DashboardQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("dashboard service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		filter, err := tenantSelfDashboardFilter(tenantID, in.DateFrom, in.DateTo)
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

// ---------------------------------------------------------------------------
// tenant-scoped usage reads (claims-scoped). Mirrors the tenant branch of the
// console handleUsageLogsByRole / handleAdminListUsageSummary.
// ---------------------------------------------------------------------------

func registerTenantSelfUsage(api huma.API, d TenantSelfReadHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-usage-logs",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/usage-logs",
		Summary:     "租户自助用量日志列表",
		Description: "按当前租户 token 返回本租户可见的用量日志分页与同过滤条件下的聚合统计；包含用户名称和 API key 的非敏感标识，不含上游/供应商内部字段。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *tenantSelfUsageLogsInput) (*tenantUsageLogsOutput, error) {
		if d.UsageQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("usage service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		dateFrom, dateTo, err := parseOptionalRFC3339Window(in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		filter := domain.UsageFilter{
			TenantID:      tenantID,
			UserID:        in.UserID,
			ModelCode:     in.ModelCode,
			RequestStatus: in.RequestStatus,
			RequestSource: in.RequestSource,
			DateFrom:      dateFrom,
			DateTo:        dateTo,
		}
		limit, offset, err := usagePageFromInput(in.Limit, in.Offset)
		if err != nil {
			return nil, err
		}
		page, err := d.UsageQueries.ListLogs(ctx, filter, limit, offset)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &tenantUsageLogsOutput{}
		out.Body.Total = page.Total
		out.Body.Stats = usageStatsToDTO(page.Stats)
		out.Body.Records = make([]tenantUsageLogDTO, 0, len(page.Records))
		for _, record := range page.Records {
			out.Body.Records = append(out.Body.Records, tenantUsageLogToDTO(record))
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-usage-summary",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/usage-summary",
		Summary:     "租户自助用量模型汇总",
		Description: "按当前租户 token 按模型聚合本租户用量，支持精确的 [start, end) 时间窗口。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *tenantSelfUsageSummaryInput) (*usageSummaryOutput, error) {
		if d.UsageQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("usage service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		dateFrom, dateTo, err := parseOptionalRFC3339Window(in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		filter := domain.UsageSummaryFilter{
			TenantID:      tenantID,
			UserID:        in.UserID,
			ModelCode:     in.ModelCode,
			RequestStatus: in.RequestStatus,
			RequestSource: in.RequestSource,
			DateFrom:      dateFrom,
			DateTo:        dateTo,
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
}

func tenantSelfDashboardFilter(tenantID string, dateFromValue string, dateToValue string) (domain.DashboardFilter, error) {
	dateFrom, dateTo, err := parseOptionalRFC3339Window(dateFromValue, dateToValue)
	if err != nil {
		return domain.DashboardFilter{}, err
	}
	return domain.DashboardFilter{
		TenantID: tenantID,
		DateFrom: dateFrom,
		DateTo:   dateTo,
	}, nil
}

func claimsUserID(ctx context.Context) string {
	claims := claimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return claims.UserID
}

package transport

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/moneyfmt"
	"xiaodou/dai/libs/go/httpx"
)

type overviewSnapshotInput struct {
	View      string `query:"view" required:"true" enum:"dashboard,business,usage,cost,operations" doc:"概览视图"`
	TenantID  string `query:"tenant_id" doc:"租户 ID"`
	ModelCode string `query:"model_code" doc:"模型编码"`
	TargetID  string `query:"target_id" doc:"上游账号或账号池 ID"`
	DateFrom  string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo    string `query:"date_to" doc:"结束时间，RFC3339，按 [start, end) 解释"`
	Compare   string `query:"compare" default:"previous" enum:"previous,none" doc:"是否返回上一周期比较"`
}

type overviewSnapshotMeta struct {
	View             string `json:"view"`
	DateFrom         int64  `json:"date_from"`
	DateTo           int64  `json:"date_to"`
	GeneratedAt      int64  `json:"generated_at"`
	ComparisonWindow string `json:"comparison_window,omitempty"`
}

type overviewSummaryDTO struct {
	TotalRequests          int64    `json:"total_requests"`
	SuccessfulRequests     int64    `json:"successful_requests"`
	FailedRequests         int64    `json:"failed_requests"`
	SuccessRate            *float64 `json:"success_rate,omitempty"`
	TotalTokens            int64    `json:"total_tokens"`
	TotalPromptTokens      int64    `json:"total_prompt_tokens"`
	TotalCompletionTokens  int64    `json:"total_completion_tokens"`
	CacheReadTokens        int64    `json:"cache_read_tokens"`
	CacheWriteTokens       int64    `json:"cache_write_tokens"`
	CacheRate              *float64 `json:"cache_rate,omitempty"`
	ActiveTenants          int64    `json:"active_tenants"`
	ActiveUsers            int64    `json:"active_users"`
	ActiveAccounts         int64    `json:"active_accounts"`
	NewTenants             int64    `json:"new_tenants"`
	NewUsers               int64    `json:"new_users"`
	TotalCatalogBaseUSD    float64  `json:"total_catalog_base_usd"`
	TotalTenantPayableUSD  float64  `json:"total_tenant_payable_usd"`
	TotalUserChargedUSD    float64  `json:"total_user_charged_usd"`
	GrossMarginUSD         float64  `json:"gross_margin_usd"`
	GrossMarginRate        *float64 `json:"gross_margin_rate,omitempty"`
	AvgRequestTotalMs      float64  `json:"avg_request_total_ms"`
	P95RequestTotalMs      float64  `json:"p95_request_total_ms"`
	AvgFirstResponseByteMs float64  `json:"avg_first_response_byte_ms"`
	P95FirstResponseByteMs float64  `json:"p95_first_response_byte_ms"`
}

type overviewTrendDTO struct {
	Date                   string  `json:"date"`
	RequestCount           int64   `json:"request_count"`
	SuccessCount           int64   `json:"success_count"`
	FailedCount            int64   `json:"failed_count"`
	TotalTokens            int64   `json:"total_tokens"`
	PromptTokens           int64   `json:"prompt_tokens"`
	CompletionTokens       int64   `json:"completion_tokens"`
	CacheReadTokens        int64   `json:"cache_read_tokens"`
	CacheWriteTokens       int64   `json:"cache_write_tokens"`
	CatalogBaseUSD         float64 `json:"catalog_base_usd"`
	TenantPayableUSD       float64 `json:"tenant_payable_usd"`
	UserChargedUSD         float64 `json:"user_charged_usd"`
	AvgLatencyMs           int64   `json:"avg_latency_ms"`
	AvgRequestTotalMs      int64   `json:"avg_request_total_ms"`
	AvgFirstResponseByteMs int64   `json:"avg_first_response_byte_ms"`
}

type overviewAccountDTO struct {
	TargetKind       string   `json:"target_kind"`
	TargetID         string   `json:"target_id"`
	TargetName       string   `json:"target_name"`
	ProviderCode     string   `json:"provider_code"`
	RequestCount     int64    `json:"request_count"`
	SuccessCount     int64    `json:"success_count"`
	FailedCount      int64    `json:"failed_count"`
	SuccessRate      *float64 `json:"success_rate,omitempty"`
	PromptTokens     int64    `json:"prompt_tokens"`
	CacheReadTokens  int64    `json:"cache_read_tokens"`
	CacheWriteTokens int64    `json:"cache_write_tokens"`
	CacheRate        *float64 `json:"cache_rate,omitempty"`
	TotalTokens      int64    `json:"total_tokens"`
	CatalogBaseUSD   float64  `json:"catalog_base_usd"`
	TenantPayableUSD float64  `json:"tenant_payable_usd"`
	LastRequestedAt  *int64   `json:"last_requested_at,omitempty"`
	HealthStatus     string   `json:"health_status"`
}

type overviewSnapshotOutput struct {
	Body struct {
		Meta       overviewSnapshotMeta     `json:"meta"`
		Summary    overviewSummaryDTO       `json:"summary"`
		Comparison *overviewSummaryDTO      `json:"comparison,omitempty"`
		Trends     []overviewTrendDTO       `json:"trends"`
		Accounts   []overviewAccountDTO     `json:"accounts"`
		Models     []usageSummaryRowDTO     `json:"models"`
		Tenants    []dashboardTopTenantDTO  `json:"tenants"`
		Users      []usageUserRankingRowDTO `json:"users"`
		Included   IdentityIncludedDTO      `json:"included"`
	}
}

func registerOverview(api huma.API, d OverviewHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "admin-overview-snapshot",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/overview/snapshot",
		Summary:     "管理员经营概览快照",
		Description: "返回仪表盘、业务、用量、成本和运维页面共享的经营统计快照。",
		Tags:        []string{"admin-overview"},
	}, func(ctx context.Context, in *overviewSnapshotInput) (*overviewSnapshotOutput, error) {
		if d.DashboardQueries == nil || d.UsageQueries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("overview service is not configured")
		}
		if in == nil {
			in = &overviewSnapshotInput{View: "dashboard", Compare: "previous"}
		}
		dashboardFilter, err := dashboardFilterFromInput(in.TenantID, "", in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		dashboardFilter.ModelCode = in.ModelCode
		usageFilter := domain.UsageSummaryFilter{
			TenantID:  in.TenantID,
			ModelCode: in.ModelCode,
			DateFrom:  dashboardFilter.DateFrom,
			DateTo:    dashboardFilter.DateTo,
		}

		summary, err := d.DashboardQueries.Summary(ctx, dashboardFilter)
		if err != nil {
			return nil, mapServiceError(err)
		}
		trends, err := d.UsageQueries.DailyTrend(ctx, dashboardFilter.DateFrom, dashboardFilter.DateTo)
		if err != nil {
			return nil, mapServiceError(err)
		}
		models, err := d.UsageQueries.Summary(ctx, usageFilter)
		if err != nil {
			return nil, mapServiceError(err)
		}
		accounts, err := d.UsageQueries.UpstreamSummary(ctx, usageFilter)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if in.TargetID != "" {
			accounts = filterOverviewAccounts(accounts, in.TargetID)
		}
		sortOverviewAccounts(accounts)
		if len(accounts) > 50 {
			accounts = accounts[:50]
		}
		if len(models) > 50 {
			models = models[:50]
		}
		users, err := d.UsageQueries.UserRanking(ctx, usageFilter, 20)
		if err != nil {
			return nil, mapServiceError(err)
		}
		topTenants, err := d.DashboardQueries.TopTenants(ctx, dashboardFilter, 20)
		if err != nil {
			return nil, mapServiceError(err)
		}

		out := &overviewSnapshotOutput{}
		out.Body.Meta = overviewSnapshotMeta{
			View:        in.View,
			DateFrom:    dashboardFilter.DateFrom.UnixMilli(),
			DateTo:      dashboardFilter.DateTo.UnixMilli(),
			GeneratedAt: time.Now().UTC().UnixMilli(),
		}
		out.Body.Summary = overviewSummaryFromDomain(summary)
		out.Body.Trends = make([]overviewTrendDTO, 0, len(trends))
		for _, row := range trends {
			out.Body.Trends = append(out.Body.Trends, overviewTrendFromDomain(row))
		}
		out.Body.Accounts = make([]overviewAccountDTO, 0, len(accounts))
		for _, row := range accounts {
			out.Body.Accounts = append(out.Body.Accounts, overviewAccountFromDomain(row))
		}
		out.Body.Models = make([]usageSummaryRowDTO, 0, len(models))
		for _, row := range models {
			out.Body.Models = append(out.Body.Models, usageSummaryRowToDTO(row))
		}
		out.Body.Tenants = make([]dashboardTopTenantDTO, 0, len(topTenants))
		for _, row := range topTenants {
			out.Body.Tenants = append(out.Body.Tenants, dashboardTopTenantToDTO(row))
		}
		out.Body.Users = make([]usageUserRankingRowDTO, 0, len(users))
		for _, row := range users {
			out.Body.Users = append(out.Body.Users, usageUserRankingRowToDTO(row))
		}
		out.Body.Included = buildIdentityIncludedForDashboardTenants(ctx, d.IdentityProvider, d.IdentityEnrichmentFailures, topTenants)

		if in.Compare != "none" {
			previousFrom, previousTo := previousOverviewWindow(dashboardFilter.DateFrom, dashboardFilter.DateTo)
			previous, compareErr := d.DashboardQueries.Summary(ctx, domain.DashboardFilter{TenantID: in.TenantID, ModelCode: in.ModelCode, DateFrom: previousFrom, DateTo: previousTo})
			if compareErr != nil {
				return nil, mapServiceError(compareErr)
			}
			out.Body.Comparison = pointerToOverviewSummary(overviewSummaryFromDomain(previous))
			out.Body.Meta.ComparisonWindow = "previous_period"
		}
		return out, nil
	})
}

func overviewSummaryFromDomain(s domain.DashboardSummary) overviewSummaryDTO {
	return overviewSummaryDTO{
		TotalRequests: s.TotalRequests, SuccessfulRequests: s.SuccessfulRequests, FailedRequests: s.FailedRequests,
		SuccessRate: overviewRate(s.SuccessfulRequests, s.TotalRequests), TotalTokens: s.TotalTokens,
		TotalPromptTokens: s.TotalPromptTokens, TotalCompletionTokens: s.TotalCompletionTokens,
		CacheReadTokens: s.CacheReadTokens, CacheWriteTokens: s.CacheWriteTokens,
		CacheRate:     overviewRate(s.CacheReadTokens, s.TotalPromptTokens+s.CacheReadTokens),
		ActiveTenants: s.ActiveTenants, ActiveUsers: s.ActiveUsers, ActiveAccounts: s.ActiveAccounts,
		NewTenants: s.NewTenants, NewUsers: s.NewUsers,
		TotalCatalogBaseUSD:   moneyfmt.MicroToUSD(s.TotalCatalogBaseMicro),
		TotalTenantPayableUSD: moneyfmt.MicroToUSD(s.TotalTenantPayableMicro),
		TotalUserChargedUSD:   moneyfmt.MicroToUSD(s.TotalUserChargedMicro),
		GrossMarginUSD:        moneyfmt.MicroToUSD(s.TotalTenantPayableMicro - s.TotalCatalogBaseMicro),
		GrossMarginRate:       overviewRate(s.TotalTenantPayableMicro-s.TotalCatalogBaseMicro, s.TotalTenantPayableMicro),
		AvgRequestTotalMs:     s.AvgRequestTotalMs, P95RequestTotalMs: s.P95RequestTotalMs,
		AvgFirstResponseByteMs: s.AvgFirstResponseByteMs, P95FirstResponseByteMs: s.P95FirstResponseByteMs,
	}
}

func overviewAccountFromDomain(row domain.UsageUpstreamSummaryRow) overviewAccountDTO {
	return overviewAccountDTO{
		TargetKind: row.TargetKind, TargetID: row.TargetID, TargetName: row.TargetName, ProviderCode: row.ProviderCode,
		RequestCount: row.RequestCount, SuccessCount: row.SuccessCount, FailedCount: row.FailedCount,
		SuccessRate: overviewRate(row.SuccessCount, row.RequestCount), PromptTokens: row.TotalPromptTokens,
		CacheReadTokens: row.CacheReadTokens, CacheWriteTokens: row.CacheWriteTokens,
		CacheRate: overviewRate(row.CacheReadTokens, row.TotalPromptTokens+row.CacheReadTokens), TotalTokens: row.TotalTokens,
		CatalogBaseUSD: moneyfmt.MicroToUSD(row.CatalogBaseMicro), TenantPayableUSD: moneyfmt.MicroToUSD(row.TenantPayableMicro),
		LastRequestedAt: timeToMillisPtr(row.LastRequestedAt), HealthStatus: "unknown",
	}
}

func overviewTrendFromDomain(row domain.DailyTrendRow) overviewTrendDTO {
	return overviewTrendDTO{Date: row.Date, RequestCount: row.RequestCount, SuccessCount: row.SuccessCount, FailedCount: row.FailedCount,
		TotalTokens: row.TotalTokens, PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens,
		CacheReadTokens: row.CacheReadTokens, CacheWriteTokens: row.CacheWriteTokens,
		CatalogBaseUSD: moneyfmt.MicroToUSD(row.CatalogBaseMicro), TenantPayableUSD: moneyfmt.MicroToUSD(row.TenantPayableMicro), UserChargedUSD: moneyfmt.MicroToUSD(row.UserChargedMicro),
		AvgLatencyMs: row.AvgLatencyMs, AvgRequestTotalMs: row.AvgRequestTotalMs, AvgFirstResponseByteMs: row.AvgFirstResponseByteMs}
}

func overviewRate(numerator, denominator int64) *float64 {
	if denominator <= 0 {
		return nil
	}
	value := float64(numerator) * 100 / float64(denominator)
	return &value
}

func previousOverviewWindow(from, to *time.Time) (*time.Time, *time.Time) {
	if from == nil || to == nil {
		return nil, nil
	}
	duration := to.Sub(*from)
	previousTo := from.Add(-time.Nanosecond)
	previousFrom := previousTo.Add(-duration)
	return &previousFrom, &previousTo
}

func pointerToOverviewSummary(value overviewSummaryDTO) *overviewSummaryDTO { return &value }

func filterOverviewAccounts(rows []domain.UsageUpstreamSummaryRow, targetID string) []domain.UsageUpstreamSummaryRow {
	filtered := rows[:0]
	for _, row := range rows {
		if row.TargetID == targetID {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// Keep the output order deterministic when repositories return an unspecified
// order after filtering or when a future rollup implementation is introduced.
func sortOverviewAccounts(rows []domain.UsageUpstreamSummaryRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].RequestCount != rows[j].RequestCount {
			return rows[i].RequestCount > rows[j].RequestCount
		}
		return rows[i].TargetID < rows[j].TargetID
	})
}

package transport

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	billingdomain "xiaodou/dai/internal/billing"
	systemports "xiaodou/dai/internal/system/ports"
	"xiaodou/dai/libs/go/httpx"
)

// ---- DTO ----

type failedTxAlert struct {
	RequestID    string `json:"requestId"`
	TerminalNote string `json:"settlementError"`
	Status       string `json:"status"`
	CreatedTime  int64  `json:"createdTime"`
}

type dashboardAlertsOutput struct {
	Body struct {
		FailedTransactions []failedTxAlert `json:"failedTransactions"`
	}
}

type trendInput struct {
	TimeFrom    int64  `query:"timeFrom" required:"false"`
	TimeTo      int64  `query:"timeTo" required:"false"`
	TenantID    string `query:"tenantId" required:"false"`
	AccountType string `query:"accountType" required:"false"`
}

type trendPoint struct {
	TimeLabel string  `json:"timeLabel"`
	AmountUSD float64 `json:"amountUsd"`
}

type consumptionTrendOutput struct {
	Body struct {
		TotalUSD   float64      `json:"totalUsd"`
		DataPoints []trendPoint `json:"dataPoints"`
	}
}

type resourceStatItem struct {
	ClientName string  `json:"clientName"`
	ClientID   string  `json:"clientId"`
	AmountUSD  float64 `json:"amountUsd"`
	Percentage string  `json:"percentage"`
}

type resourceStatsInput struct {
	TimeFrom int64  `query:"timeFrom" required:"false"`
	TimeTo   int64  `query:"timeTo" required:"false"`
	TenantID string `query:"tenantId" required:"false"`
}

type resourceStatsOutput struct {
	Body struct {
		Resources []resourceStatItem `json:"resources"`
	}
}

type globalStatsInput struct {
	TimeFrom int64 `query:"timeFrom" required:"false"`
	TimeTo   int64 `query:"timeTo" required:"false"`
}

type globalStatsOutput struct {
	Body systemports.GlobalStats
}

// registerAdminDashboard 注册 dashboard 与分析端点。
func registerAdminDashboard(api huma.API, d adminDashboardModule) {
	h := newAdminDashboardHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysUser := huma.Middlewares{ua, requireCapability(api, auth.CapabilityPlatformAdmin)}

	huma.Register(api, huma.Operation{OperationID: "admin-dashboard-alerts", Method: http.MethodGet, Path: "/api/v1/dashboard/alerts",
		Summary: "Dashboard 告警（异常扣费）", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.dashboardAlerts)
	huma.Register(api, huma.Operation{OperationID: "admin-consumption-trend", Method: http.MethodGet, Path: "/api/v1/analytics/consumption-trend",
		Summary: "消费趋势", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.consumptionTrend)
	huma.Register(api, huma.Operation{OperationID: "admin-resource-statistics", Method: http.MethodGet, Path: "/api/v1/analytics/resource-statistics",
		Summary: "资源消耗占比", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.resourceStatistics)
	huma.Register(api, huma.Operation{OperationID: "admin-global-stats", Method: http.MethodGet, Path: "/api/v1/analytics/global-stats",
		Summary: "全局统计", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.globalStats)
}

func (h *adminHandlers) dashboardAlerts(ctx context.Context, _ *struct{}) (*dashboardAlertsOutput, error) {
	out := &dashboardAlertsOutput{}
	out.Body.FailedTransactions = []failedTxAlert{}

	alerts, err := h.systemRepo.ListFailedTransactionAlerts(ctx)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	for _, alert := range alerts {
		out.Body.FailedTransactions = append(out.Body.FailedTransactions, failedTxAlert{
			RequestID:    alert.RequestID,
			TerminalNote: alert.SettlementError,
			Status:       alert.BillingStatus,
			CreatedTime:  alert.CreatedAt.UTC().UnixMilli(),
		})
	}
	return out, nil
}

func (h *adminHandlers) consumptionTrend(ctx context.Context, in *trendInput) (*consumptionTrendOutput, error) {
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	timeFrom, timeTo = applyDefaultAdminAnalyticsWindow(timeFrom, timeTo, 7*24*time.Hour)
	params := systemports.ConsumptionTrendQuery{TimeFrom: timeFrom, TimeTo: timeTo}
	if in.TenantID != "" {
		params.TenantID = &in.TenantID
	}
	if in.AccountType != "" {
		var at int64
		if _, err := fmt.Sscan(in.AccountType, &at); err == nil {
			params.AccountType = &at
		}
	}
	rows, err := h.systemRepo.GetConsumptionTrend(ctx, params)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	out := &consumptionTrendOutput{}
	out.Body.DataPoints = make([]trendPoint, 0, len(rows))
	for _, row := range rows {
		out.Body.TotalUSD += row.Total
		out.Body.DataPoints = append(out.Body.DataPoints, trendPoint{TimeLabel: row.Day.Format("2006-01-02"), AmountUSD: row.Total})
	}
	return out, nil
}

func (h *adminHandlers) resourceStatistics(ctx context.Context, in *resourceStatsInput) (*resourceStatsOutput, error) {
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	timeFrom, timeTo = applyDefaultAdminAnalyticsWindow(timeFrom, timeTo, 7*24*time.Hour)
	params := systemports.ResourceStatisticsQuery{TimeFrom: timeFrom, TimeTo: timeTo}
	if in.TenantID != "" {
		params.TenantID = &in.TenantID
	}
	rows, err := h.systemRepo.GetResourceStatistics(ctx, params)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	var totalUSD float64
	for _, row := range rows {
		totalUSD += row.Total
	}
	out := &resourceStatsOutput{}
	out.Body.Resources = make([]resourceStatItem, 0, len(rows))
	for _, row := range rows {
		pct := 0.0
		if totalUSD > 0 {
			pct = row.Total / totalUSD * 100
		}
		name := row.ClientName
		if name == "" {
			name = "未知系统"
		}
		out.Body.Resources = append(out.Body.Resources, resourceStatItem{
			ClientName: name, ClientID: row.ClientID, AmountUSD: row.Total, Percentage: fmt.Sprintf("%.1f", pct),
		})
	}
	return out, nil
}

func (h *adminHandlers) globalStats(ctx context.Context, in *globalStatsInput) (*globalStatsOutput, error) {
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	timeFrom, timeTo = applyDefaultAdminAnalyticsWindow(timeFrom, timeTo, 30*24*time.Hour)
	stats, err := h.systemRepo.GetGlobalStats(ctx, timeFrom, timeTo)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	return &globalStatsOutput{Body: stats}, nil
}

func applyDefaultAdminAnalyticsWindow(timeFrom, timeTo *time.Time, duration time.Duration) (*time.Time, *time.Time) {
	if timeFrom != nil || timeTo != nil {
		return timeFrom, timeTo
	}
	endAt := billingdomain.NowUTC()
	startAt := endAt.Add(-duration)
	return &startAt, &endAt
}

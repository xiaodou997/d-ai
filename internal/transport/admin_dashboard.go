package transport

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	billingdomain "xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
	systempg "xiaodou/dai/internal/system/pg"
	"xiaodou/dai/libs/go/httpx"
)

// ---- DTO ----

type preAuthAlert struct {
	EventID     string `json:"eventId"`
	TenantID    string `json:"tenantId"`
	UserID      string `json:"userId"`
	CreatedTime int64  `json:"createdTime"`
}

type failedTxAlert struct {
	EventID      string `json:"eventId"`
	TerminalNote string `json:"terminalNote"`
	Status       string `json:"status"`
	CreatedTime  int64  `json:"createdTime"`
}

type dashboardAlertsOutput struct {
	Body struct {
		TimeoutPreAuths    []preAuthAlert  `json:"timeoutPreAuths"`
		FailedTransactions []failedTxAlert `json:"failedTransactions"`
	}
}

type cancelPreAuthInput struct {
	ID   string `path:"id"`
	Body *struct {
		Note string `json:"note"`
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
	Credits   float64 `json:"credits"`
}

type consumptionTrendOutput struct {
	Body struct {
		TotalCredits float64      `json:"totalCredits"`
		DataPoints   []trendPoint `json:"dataPoints"`
	}
}

type resourceStatItem struct {
	ClientName string  `json:"clientName"`
	ClientID   string  `json:"clientId"`
	Credits    float64 `json:"credits"`
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
	Body systempg.GlobalStatsRow
}

// registerAdminDashboard 注册 dashboard / 分析 / 预授权取消 / 租户应用授权查询端点。
func registerAdminDashboard(api huma.API, d Deps) {
	h := newAdminHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysUser := huma.Middlewares{ua, requireUserType(api, 1, 2)}

	huma.Register(api, huma.Operation{OperationID: "admin-dashboard-alerts", Method: http.MethodGet, Path: "/api/v1/dashboard/alerts",
		Summary: "Dashboard 告警（超时预授权 / 异常释放）", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.dashboardAlerts)
	huma.Register(api, huma.Operation{OperationID: "admin-cancel-preauth", Method: http.MethodPost, Path: "/api/v1/billing/events/{id}/cancel",
		Summary: "取消卡住的预授权", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.cancelPreAuth)
	huma.Register(api, huma.Operation{OperationID: "admin-consumption-trend", Method: http.MethodGet, Path: "/api/v1/analytics/consumption-trend",
		Summary: "消费趋势", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.consumptionTrend)
	huma.Register(api, huma.Operation{OperationID: "admin-resource-statistics", Method: http.MethodGet, Path: "/api/v1/analytics/resource-statistics",
		Summary: "资源消耗占比", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.resourceStatistics)
	huma.Register(api, huma.Operation{OperationID: "admin-global-stats", Method: http.MethodGet, Path: "/api/v1/analytics/global-stats",
		Summary: "全局统计", Tags: []string{"admin-dashboard"}, Middlewares: sysUser}, h.globalStats)
}

func (h *adminHandlers) dashboardAlerts(ctx context.Context, _ *struct{}) (*dashboardAlertsOutput, error) {
	out := &dashboardAlertsOutput{}
	out.Body.TimeoutPreAuths = []preAuthAlert{}
	out.Body.FailedTransactions = []failedTxAlert{}

	stuck, _ := h.txRepo.FindStuckPreAuth(30)
	for _, tx := range stuck {
		out.Body.TimeoutPreAuths = append(out.Body.TimeoutPreAuths, preAuthAlert{
			EventID: tx.EventID, TenantID: tx.TenantID, UserID: tx.UserID, CreatedTime: tx.CreatedAt.UnixMilli(),
		})
	}
	released, _ := h.txRepo.FindReleasedInHours(24, 20)
	for _, tx := range released {
		out.Body.FailedTransactions = append(out.Body.FailedTransactions, failedTxAlert{
			EventID: tx.EventID, TerminalNote: tx.TerminalNote, Status: tx.Status, CreatedTime: tx.CreatedAt.UnixMilli(),
		})
	}
	return out, nil
}

func (h *adminHandlers) cancelPreAuth(ctx context.Context, in *cancelPreAuthInput) (*eventStatusOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if _, err := h.deduction.Cancel(billingsvc.CancelParams{EventID: in.ID}); err != nil {
		return nil, toProblem(err)
	}
	note := ""
	if in.Body != nil {
		note = in.Body.Note
	}
	_ = h.deduction.AppendOp(ctx, in.ID, map[string]any{
		"action": "admin_cancelled", "operator_id": claims.UserID, "note": note,
		"at": billingdomain.NowUTC().UnixMilli(),
	})
	out := &eventStatusOutput{}
	out.Body.EventID = in.ID
	out.Body.Status = "cancelled"
	return out, nil
}

func (h *adminHandlers) consumptionTrend(ctx context.Context, in *trendInput) (*consumptionTrendOutput, error) {
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	timeFrom, timeTo = applyDefaultAdminAnalyticsWindow(timeFrom, timeTo, 7*24*time.Hour)
	params := systempg.GetConsumptionTrendParams{TimeFrom: timeFrom, TimeTo: timeTo}
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
		out.Body.TotalCredits += row.Total
		out.Body.DataPoints = append(out.Body.DataPoints, trendPoint{TimeLabel: row.Day.Format("2006-01-02"), Credits: row.Total})
	}
	return out, nil
}

func (h *adminHandlers) resourceStatistics(ctx context.Context, in *resourceStatsInput) (*resourceStatsOutput, error) {
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	timeFrom, timeTo = applyDefaultAdminAnalyticsWindow(timeFrom, timeTo, 7*24*time.Hour)
	params := systempg.GetResourceStatisticsParams{TimeFrom: timeFrom, TimeTo: timeTo}
	if in.TenantID != "" {
		params.TenantID = &in.TenantID
	}
	rows, err := h.systemRepo.GetResourceStatistics(ctx, params)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	var totalCredits float64
	for _, row := range rows {
		totalCredits += float64(row.Total)
	}
	out := &resourceStatsOutput{}
	out.Body.Resources = make([]resourceStatItem, 0, len(rows))
	for _, row := range rows {
		pct := 0.0
		if totalCredits > 0 {
			pct = float64(row.Total) / totalCredits * 100
		}
		name := row.ClientName
		if name == "" {
			name = "未知系统"
		}
		out.Body.Resources = append(out.Body.Resources, resourceStatItem{
			ClientName: name, ClientID: row.ClientID, Credits: row.Total, Percentage: fmt.Sprintf("%.1f", pct),
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

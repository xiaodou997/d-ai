package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

var _ UsageQueryReader = (*observabilitycontrol.UsageService)(nil)

type usageQueryReaderStub struct {
	dailyFrom       *time.Time
	dailyTo         *time.Time
	listFilter      domain.UsageFilter
	listLimit       int32
	listOffset      int32
	detailRequestID string
	summaryFilter   domain.UsageSummaryFilter
	unitFilter      domain.UsageSummaryFilter
	upstreamFilter  domain.UsageSummaryFilter
	rankingFilter   domain.UsageSummaryFilter
	rankingLimit    int32
	userTenantID    string
	userID          string
	userSource      string
	dailyRows       []domain.DailyTrendRow
	page            domain.UsageLogPage
	detail          domain.UsageLogDetail
	summaryRows     []domain.UsageSummaryRow
	unitRows        []domain.UsageUnitSummaryRow
	upstreamRows    []domain.UsageUpstreamSummaryRow
	rankingRows     []domain.UsageUserRankingRow
	userSummary     domain.UserUsageSummary
}

func (s *usageQueryReaderStub) DailyTrend(_ context.Context, dateFrom, dateTo *time.Time) ([]domain.DailyTrendRow, error) {
	s.dailyFrom, s.dailyTo = dateFrom, dateTo
	return s.dailyRows, nil
}

func (s *usageQueryReaderStub) ListLogs(_ context.Context, filter domain.UsageFilter, limit, offset int32) (domain.UsageLogPage, error) {
	s.listFilter, s.listLimit, s.listOffset = filter, limit, offset
	return s.page, nil
}

func (s *usageQueryReaderStub) GetLogDetail(_ context.Context, requestID string) (domain.UsageLogDetail, error) {
	s.detailRequestID = requestID
	return s.detail, nil
}

func (s *usageQueryReaderStub) Summary(_ context.Context, filter domain.UsageSummaryFilter) ([]domain.UsageSummaryRow, error) {
	s.summaryFilter = filter
	return s.summaryRows, nil
}

func (s *usageQueryReaderStub) UnitSummary(_ context.Context, filter domain.UsageSummaryFilter) ([]domain.UsageUnitSummaryRow, error) {
	s.unitFilter = filter
	return s.unitRows, nil
}

func (s *usageQueryReaderStub) UpstreamSummary(_ context.Context, filter domain.UsageSummaryFilter) ([]domain.UsageUpstreamSummaryRow, error) {
	s.upstreamFilter = filter
	return s.upstreamRows, nil
}

func (s *usageQueryReaderStub) UserRanking(_ context.Context, filter domain.UsageSummaryFilter, limit int32) ([]domain.UsageUserRankingRow, error) {
	s.rankingFilter, s.rankingLimit = filter, limit
	return s.rankingRows, nil
}

func (s *usageQueryReaderStub) UserSummary(_ context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error) {
	s.userTenantID, s.userID, s.userSource = tenantID, userID, requestSource
	return s.userSummary, nil
}

func TestUsageRoutesUseQueryReader(t *testing.T) {
	createdAt := time.Date(2026, time.August, 21, 3, 4, 5, 0, time.UTC)
	reader := &usageQueryReaderStub{
		dailyRows: []domain.DailyTrendRow{{Date: "2026-08-20", RequestCount: 2, TenantPayableMicro: 1_250_000}},
		page: domain.UsageLogPage{
			Total:   3,
			Stats:   domain.UsageStats{TotalRequests: 3, TotalTenantPayableMicro: 2_500_000},
			Records: []domain.UsageLog{{ID: "log-1", RequestID: "request-1", RequestStatus: "success", CreatedAt: createdAt}},
		},
		detail:       domain.UsageLogDetail{UsageLog: domain.UsageLog{RequestID: "request-1", RequestStatus: "failed"}},
		summaryRows:  []domain.UsageSummaryRow{{ModelCode: "gpt-test", RequestCount: 4, TotalTenantPayableMicro: 3_750_000}},
		unitRows:     []domain.UsageUnitSummaryRow{{BillableUnitType: "token", RequestCount: 5, TotalTenantPayableMicro: 4_500_000}},
		upstreamRows: []domain.UsageUpstreamSummaryRow{{TargetKind: "direct_upstream", TargetID: "upstream-1", RequestCount: 6, TenantPayableMicro: 5_250_000}},
		rankingRows:  []domain.UsageUserRankingRow{{TenantID: "tenant-1", UserID: "user-1", RequestCount: 7, TotalUserChargedMicro: 6_500_000, LastRequestedAt: createdAt}},
		userSummary:  domain.UserUsageSummary{RequestCount: 8, TotalUserChargedMicro: 7_250_000},
	}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerUsage(api, UsageHTTPDeps{UsageQueries: reader})
	registerUserSelfUsage(api, UserSelfReadHTTPDeps{UsageQueries: reader})

	window := "date_from=2026-08-20T00:00:00Z&date_to=2026-08-21T00:00:00Z"
	trendRecorder := performUsageRequest(router, "/api/v1/analytics/daily-trend?"+window)
	requireUsageStatus(t, trendRecorder, http.StatusOK)
	assertUsageWindow(t, reader.dailyFrom, reader.dailyTo)
	var trend struct {
		Items []dailyTrendRowDTO `json:"items"`
		Total int                `json:"total"`
	}
	decodeUsageResponse(t, trendRecorder, &trend)
	if trend.Total != 1 || trend.Items[0].TenantPayableUSD != 1.25 {
		t.Fatalf("daily trend response = %#v", trend)
	}

	filter := "tenant_id=tenant-1&user_id=user-1&model_code=gpt-test&request_status=success&request_source=workspace&" + window
	logsRecorder := performUsageRequest(router, "/api/v1/usage-logs?"+filter+"&limit=999&offset=3")
	requireUsageStatus(t, logsRecorder, http.StatusOK)
	assertUsageFilter(t, reader.listFilter)
	if reader.listLimit != 100 || reader.listOffset != 3 {
		t.Fatalf("list page = limit %d offset %d", reader.listLimit, reader.listOffset)
	}
	var logs struct {
		Total   int64         `json:"total"`
		Stats   usageStatsDTO `json:"stats"`
		Records []usageLogDTO `json:"records"`
	}
	decodeUsageResponse(t, logsRecorder, &logs)
	if logs.Total != 3 || logs.Stats.TotalTenantPayableUSD != 2.5 || len(logs.Records) != 1 || logs.Records[0].RequestID != "request-1" {
		t.Fatalf("usage logs response = %#v", logs)
	}

	detailRecorder := performUsageRequest(router, "/api/v1/usage-logs/request-1")
	requireUsageStatus(t, detailRecorder, http.StatusOK)
	if reader.detailRequestID != "request-1" {
		t.Fatalf("detail request id = %q", reader.detailRequestID)
	}
	var detail usageLogDetailDTO
	decodeUsageResponse(t, detailRecorder, &detail)
	if detail.RequestID != "request-1" || detail.RequestStatus != "failed" {
		t.Fatalf("detail response = %#v", detail)
	}

	summaryRecorder := performUsageRequest(router, "/api/v1/usage-summary?"+filter)
	requireUsageStatus(t, summaryRecorder, http.StatusOK)
	assertUsageSummaryFilter(t, reader.summaryFilter)
	var summary struct {
		Items []usageSummaryRowDTO `json:"items"`
		Total int                  `json:"total"`
	}
	decodeUsageResponse(t, summaryRecorder, &summary)
	if summary.Total != 1 || summary.Items[0].ModelCode != "gpt-test" || summary.Items[0].TotalTenantPayableUSD != 3.75 {
		t.Fatalf("summary response = %#v", summary)
	}

	unitRecorder := performUsageRequest(router, "/api/v1/usage-unit-summary?"+filter)
	requireUsageStatus(t, unitRecorder, http.StatusOK)
	assertUsageSummaryFilter(t, reader.unitFilter)
	var unit struct {
		Items []usageUnitSummaryRowDTO `json:"items"`
		Total int                      `json:"total"`
	}
	decodeUsageResponse(t, unitRecorder, &unit)
	if unit.Total != 1 || unit.Items[0].BillableUnitType != "token" || unit.Items[0].TotalTenantPayableUSD != 4.5 {
		t.Fatalf("unit summary response = %#v", unit)
	}

	upstreamRecorder := performUsageRequest(router, "/api/v1/usage-upstream-summary?"+filter)
	requireUsageStatus(t, upstreamRecorder, http.StatusOK)
	assertUsageSummaryFilter(t, reader.upstreamFilter)
	var upstream struct {
		Items []usageUpstreamSummaryRowDTO `json:"items"`
		Total int                          `json:"total"`
	}
	decodeUsageResponse(t, upstreamRecorder, &upstream)
	if upstream.Total != 1 || upstream.Items[0].TargetID != "upstream-1" || upstream.Items[0].TenantPayableUSD != 5.25 {
		t.Fatalf("upstream summary response = %#v", upstream)
	}

	rankingRecorder := performUsageRequest(router, "/api/v1/usage-ranking/users?"+filter+"&limit=999")
	requireUsageStatus(t, rankingRecorder, http.StatusOK)
	assertUsageSummaryFilter(t, reader.rankingFilter)
	if reader.rankingLimit != 100 {
		t.Fatalf("ranking limit = %d, want 100", reader.rankingLimit)
	}
	var ranking struct {
		Items []usageUserRankingRowDTO `json:"items"`
		Total int                      `json:"total"`
	}
	decodeUsageResponse(t, rankingRecorder, &ranking)
	if ranking.Total != 1 || ranking.Items[0].UserID != "user-1" || ranking.Items[0].TotalUserChargedUSD != 6.5 {
		t.Fatalf("ranking response = %#v", ranking)
	}

	userHandler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ctx := context.WithValue(request.Context(), authClaimsContextKey{}, &auth.Claims{TenantID: "tenant-1", UserID: "user-1"})
		router.ServeHTTP(w, request.WithContext(ctx))
	})
	userRecorder := performUsageRequest(userHandler, "/api/v1/user-usage-summary?request_source=workspace")
	requireUsageStatus(t, userRecorder, http.StatusOK)
	if reader.userTenantID != "tenant-1" || reader.userID != "user-1" || reader.userSource != "workspace" {
		t.Fatalf("user summary scope = tenant %q user %q source %q", reader.userTenantID, reader.userID, reader.userSource)
	}
	var userSummary userUsageSummaryDTO
	decodeUsageResponse(t, userRecorder, &userSummary)
	if userSummary.RequestCount != 8 || userSummary.TotalUserChargedUSD != 7.25 {
		t.Fatalf("user summary response = %#v", userSummary)
	}
}

func TestUsageRoutesRequireQueryReader(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerUsage(api, UsageHTTPDeps{})

	recorder := performUsageRequest(router, "/api/v1/usage-logs")
	requireUsageStatus(t, recorder, http.StatusServiceUnavailable)
}

func TestUsageRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	paths := []string{
		"/api/v1/analytics/daily-trend",
		"/api/v1/usage-logs",
		"/api/v1/usage-logs/request-1",
		"/api/v1/usage-summary",
		"/api/v1/usage-unit-summary",
		"/api/v1/usage-upstream-summary",
		"/api/v1/usage-ranking/users",
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, CoreHTTPDeps{})
	for _, path := range paths {
		recorder := performUsageRequest(coreRouter, path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI usage route %s status = %d, want %d", path, recorder.Code, http.StatusNotFound)
		}
	}

	usageRouter, usageAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterUsage(usageAPI, UsageHTTPDeps{})
	for _, path := range paths {
		recorder := performUsageRequest(usageRouter, path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent usage route %s status = %d, want %d", path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performUsageRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func requireUsageStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, want, recorder.Body.String())
	}
}

func decodeUsageResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertUsageWindow(t *testing.T, dateFrom, dateTo *time.Time) {
	t.Helper()
	if dateFrom == nil || dateTo == nil || dateFrom.Format(time.RFC3339) != "2026-08-20T00:00:00Z" || dateTo.Format(time.RFC3339) != "2026-08-21T00:00:00Z" {
		t.Fatalf("window = %v to %v", dateFrom, dateTo)
	}
}

func assertUsageFilter(t *testing.T, filter domain.UsageFilter) {
	t.Helper()
	if filter.TenantID != "tenant-1" || filter.UserID != "user-1" || filter.ModelCode != "gpt-test" || filter.RequestStatus != "success" || filter.RequestSource != "workspace" {
		t.Fatalf("filter = %#v", filter)
	}
	assertUsageWindow(t, filter.DateFrom, filter.DateTo)
}

func assertUsageSummaryFilter(t *testing.T, filter domain.UsageSummaryFilter) {
	t.Helper()
	if filter.TenantID != "tenant-1" || filter.UserID != "user-1" || filter.ModelCode != "gpt-test" || filter.RequestStatus != "success" || filter.RequestSource != "workspace" {
		t.Fatalf("summary filter = %#v", filter)
	}
	assertUsageWindow(t, filter.DateFrom, filter.DateTo)
}

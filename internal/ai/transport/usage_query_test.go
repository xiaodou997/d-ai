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

func TestUsageRoutesRequireQueryReader(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerUsage(api, UsageHTTPDeps{})

	recorder := performUsageRequest(router, "/api/v1/usage-summary")
	requireUsageStatus(t, recorder, http.StatusServiceUnavailable)
}

func TestUsageRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	paths := []string{
		"/api/v1/analytics/daily-trend",
		"/api/v2/requests",
		"/api/v2/request-errors",
		"/api/v2/billing/settlements",
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

func TestUsageLogsExplicitErrorFilter(t *testing.T) {
	filter, err := usageLogFilterFromInput(&usageLogsInput{ErrorsOnly: true})
	if err != nil || !filter.ErrorsOnly || filter.RequestStatus != "" {
		t.Fatalf("explicit error filter must not impose a lifecycle status: %+v, %v", filter, err)
	}
}

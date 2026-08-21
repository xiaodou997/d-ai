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

var _ DashboardQueryReader = (*observabilitycontrol.DashboardService)(nil)

type dashboardQueryReaderStub struct {
	summaryFilter      domain.DashboardFilter
	topModelsFilter    domain.DashboardFilter
	topModelsLimit     int32
	topTenantsFilter   domain.DashboardFilter
	topTenantsLimit    int32
	recentErrorsFilter domain.DashboardFilter
	recentErrorsLimit  int32
	summary            domain.DashboardSummary
	topModels          []domain.DashboardTopModel
	topTenants         []domain.DashboardTopTenant
	recentErrors       []domain.DashboardRecentError
}

func (s *dashboardQueryReaderStub) Summary(_ context.Context, filter domain.DashboardFilter) (domain.DashboardSummary, error) {
	s.summaryFilter = filter
	return s.summary, nil
}

func (s *dashboardQueryReaderStub) TopModels(_ context.Context, filter domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error) {
	s.topModelsFilter = filter
	s.topModelsLimit = limit
	return s.topModels, nil
}

func (s *dashboardQueryReaderStub) TopTenants(_ context.Context, filter domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error) {
	s.topTenantsFilter = filter
	s.topTenantsLimit = limit
	return s.topTenants, nil
}

func (s *dashboardQueryReaderStub) RecentErrors(_ context.Context, filter domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error) {
	s.recentErrorsFilter = filter
	s.recentErrorsLimit = limit
	return s.recentErrors, nil
}

func TestDashboardRoutesUseQueryReader(t *testing.T) {
	status := int32(http.StatusBadGateway)
	createdAt := time.Date(2026, time.August, 21, 3, 4, 5, 0, time.UTC)
	reader := &dashboardQueryReaderStub{
		summary:    domain.DashboardSummary{TotalRequests: 7, TotalTenantPayableMicro: 1_250_000},
		topModels:  []domain.DashboardTopModel{{ModelCode: "gpt-test", RequestCount: 6, TotalTenantPayableMicro: 2_500_000}},
		topTenants: []domain.DashboardTopTenant{{TenantID: "tenant-1", RequestCount: 5, TotalTenantPayableMicro: 3_750_000}},
		recentErrors: []domain.DashboardRecentError{{
			RequestID: "request-1", ModelCode: "gpt-test", RequestStatus: "failed",
			ErrorCode: "upstream_error", HTTPStatus: &status, CreatedAt: createdAt,
		}},
	}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerDashboard(api, DashboardHTTPDeps{DashboardQueries: reader})

	window := "tenant_id=tenant-1&user_id=user-1&date_from=2026-08-20T00:00:00Z&date_to=2026-08-21T00:00:00Z"

	summaryRecorder := performDashboardRequest(router, "/api/v1/dashboard/summary?"+window)
	if summaryRecorder.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", summaryRecorder.Code, summaryRecorder.Body.String())
	}
	assertDashboardFilter(t, reader.summaryFilter)
	var summary dashboardSummaryDTO
	decodeDashboardResponse(t, summaryRecorder, &summary)
	if summary.TotalRequests != 7 || summary.TotalTenantPayableUSD != 1.25 {
		t.Fatalf("summary response = %#v", summary)
	}

	modelsRecorder := performDashboardRequest(router, "/api/v1/dashboard/top-models?"+window+"&limit=999")
	if modelsRecorder.Code != http.StatusOK {
		t.Fatalf("top models status = %d, body = %s", modelsRecorder.Code, modelsRecorder.Body.String())
	}
	assertDashboardFilter(t, reader.topModelsFilter)
	if reader.topModelsLimit != 50 {
		t.Fatalf("top models limit = %d, want 50", reader.topModelsLimit)
	}
	var models struct {
		Items []dashboardTopModelDTO `json:"items"`
		Total int                    `json:"total"`
	}
	decodeDashboardResponse(t, modelsRecorder, &models)
	if models.Total != 1 || models.Items[0].ModelCode != "gpt-test" || models.Items[0].TotalTenantPayableUSD != 2.5 {
		t.Fatalf("top models response = %#v", models)
	}

	tenantsRecorder := performDashboardRequest(router, "/api/v1/dashboard/top-tenants?"+window+"&limit=9")
	if tenantsRecorder.Code != http.StatusOK {
		t.Fatalf("top tenants status = %d, body = %s", tenantsRecorder.Code, tenantsRecorder.Body.String())
	}
	assertDashboardFilter(t, reader.topTenantsFilter)
	if reader.topTenantsLimit != 9 {
		t.Fatalf("top tenants limit = %d, want 9", reader.topTenantsLimit)
	}
	var tenants struct {
		Items    []dashboardTopTenantDTO `json:"items"`
		Total    int                     `json:"total"`
		Included IdentityIncludedDTO     `json:"included"`
	}
	decodeDashboardResponse(t, tenantsRecorder, &tenants)
	if tenants.Total != 1 || tenants.Items[0].TenantID != "tenant-1" || tenants.Items[0].TotalTenantPayableUSD != 3.75 {
		t.Fatalf("top tenants response = %#v", tenants)
	}
	if len(tenants.Included.Users) != 0 || len(tenants.Included.Tenants) != 0 {
		t.Fatalf("top tenants included = %#v", tenants.Included)
	}

	errorsRecorder := performDashboardRequest(router, "/api/v1/dashboard/recent-errors?"+window+"&limit=8")
	if errorsRecorder.Code != http.StatusOK {
		t.Fatalf("recent errors status = %d, body = %s", errorsRecorder.Code, errorsRecorder.Body.String())
	}
	assertDashboardFilter(t, reader.recentErrorsFilter)
	if reader.recentErrorsLimit != 8 {
		t.Fatalf("recent errors limit = %d, want 8", reader.recentErrorsLimit)
	}
	var recentErrors struct {
		Items []dashboardRecentErrorDTO `json:"items"`
		Total int                       `json:"total"`
	}
	decodeDashboardResponse(t, errorsRecorder, &recentErrors)
	item := recentErrors.Items[0]
	if recentErrors.Total != 1 || item.RequestID != "request-1" || item.ErrorCode == nil || *item.ErrorCode != "upstream_error" {
		t.Fatalf("recent errors response = %#v", recentErrors)
	}
	if item.CreatedAt == nil || *item.CreatedAt != createdAt.UnixMilli() || item.HTTPStatus == nil || *item.HTTPStatus != status {
		t.Fatalf("recent error projection = %#v", item)
	}
}

func TestDashboardRoutesRequireQueryReader(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerDashboard(api, DashboardHTTPDeps{})

	recorder := performDashboardRequest(router, "/api/v1/dashboard/summary")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDashboardRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	paths := []string{
		"/api/v1/dashboard/summary",
		"/api/v1/dashboard/top-models",
		"/api/v1/dashboard/top-tenants",
		"/api/v1/dashboard/recent-errors",
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, path := range paths {
		recorder := performDashboardRequest(coreRouter, path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI dashboard route %s status = %d, want %d", path, recorder.Code, http.StatusNotFound)
		}
	}

	dashboardRouter, dashboardAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterDashboard(dashboardAPI, DashboardHTTPDeps{})
	for _, path := range paths {
		recorder := performDashboardRequest(dashboardRouter, path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent dashboard route %s status = %d, want %d", path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performDashboardRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeDashboardResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertDashboardFilter(t *testing.T, filter domain.DashboardFilter) {
	t.Helper()
	if filter.TenantID != "tenant-1" || filter.UserID != "user-1" {
		t.Fatalf("filter scope = %#v", filter)
	}
	if filter.DateFrom == nil || filter.DateTo == nil || filter.DateFrom.Format(time.RFC3339) != "2026-08-20T00:00:00Z" || filter.DateTo.Format(time.RFC3339) != "2026-08-21T00:00:00Z" {
		t.Fatalf("filter window = %#v", filter)
	}
}

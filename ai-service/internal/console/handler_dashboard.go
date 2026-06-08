package console

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/unihub/ai-service/internal/credits"
	"xiaodou/unihub/ai-service/internal/domain"
)

func (s *Console) handleAdminDashboardTopModels(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	models, err := s.dashboardSvc.TopModels(r.Context(), dashboardFilterFromParams(params), limit)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, dashboardTopModelsFromDomain(models))
}

func (s *Console) handleAdminDashboardTopTenants(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	tenants, err := s.dashboardSvc.TopTenants(r.Context(), dashboardFilterFromParams(params), limit)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, dashboardTopTenantsFromDomain(tenants))
}

func (s *Console) handleAdminDashboardRecentErrors(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	errs, err := s.dashboardSvc.RecentErrors(r.Context(), dashboardFilterFromParams(params), limit)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, dashboardRecentErrorsFromDomain(errs))
}

// dashboardFilterFromParams converts the HTTP-layer scoped params (already
// role-filtered by scopedDashboardParams) into a service-layer filter.
func dashboardFilterFromParams(p dashboardParams) domain.DashboardFilter {
	return domain.DashboardFilter{
		TenantID: p.tenantID,
		UserID:   p.userID,
		Since:    pgTimestamptzToPtr(p.since),
	}
}

// ---------------------------------------------------------------------------
// domain → wire DTO（micro→credits + 重建 pgtype，保 JSON 契约逐字节一致）
// ---------------------------------------------------------------------------

func dashboardSummaryFromDomain(s domain.DashboardSummary) dashboardSummaryDTO {
	return dashboardSummaryDTO{
		TotalRequests:         s.TotalRequests,
		SuccessfulRequests:    s.SuccessfulRequests,
		FailedRequests:        s.FailedRequests,
		TotalTokens:           s.TotalTokens,
		TotalPromptTokens:     s.TotalPromptTokens,
		TotalCompletionTokens: s.TotalCompletionTokens,
		TotalProviderCredits:  credits.MicroToCredits(s.TotalProviderCostMicro),
		TotalPlatformCredits:  credits.MicroToCredits(s.TotalPlatformCostMicro),
		TotalUserCredits:      credits.MicroToCredits(s.TotalUserCostMicro),
		AvgLatencyMs:          s.AvgLatencyMs,
	}
}

func dashboardTopModelsFromDomain(models []domain.DashboardTopModel) []dashboardTopModelDTO {
	out := make([]dashboardTopModelDTO, len(models))
	for i, m := range models {
		out[i] = dashboardTopModelDTO{
			ModelCode:    m.ModelCode,
			RequestCount: m.RequestCount,
			TotalTokens:  m.TotalTokens,
			TotalCredits: credits.MicroToCredits(m.TotalCostMicro),
		}
	}
	return out
}

func dashboardTopTenantsFromDomain(tenants []domain.DashboardTopTenant) []dashboardTopTenantDTO {
	out := make([]dashboardTopTenantDTO, len(tenants))
	for i, t := range tenants {
		out[i] = dashboardTopTenantDTO{
			TenantID:     t.TenantID,
			RequestCount: t.RequestCount,
			TotalTokens:  t.TotalTokens,
			TotalCredits: credits.MicroToCredits(t.TotalCostMicro),
		}
	}
	return out
}

func dashboardRecentErrorsFromDomain(errs []domain.DashboardRecentError) []dashboardRecentErrorDTO {
	out := make([]dashboardRecentErrorDTO, len(errs))
	for i, e := range errs {
		dto := dashboardRecentErrorDTO{
			RequestID:     e.RequestID,
			ModelCode:     e.ModelCode,
			RequestStatus: e.RequestStatus,
			ErrorCode:     optionalTextValue(e.ErrorCode),
			ErrorMessage:  optionalTextValue(e.ErrorMessage),
			CreatedAt:     timeToMillisPtr(e.CreatedAt),
		}
		if e.HTTPStatus != nil {
			dto.HttpStatus = pgtype.Int4{Int32: *e.HTTPStatus, Valid: true}
		}
		out[i] = dto
	}
	return out
}

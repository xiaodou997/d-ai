package httpserver

import (
	"net/http"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

func (s *Server) handleAdminDashboardTopModels(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	rows, err := s.queries.ListDashboardTopModels(r.Context(), dbgen.ListDashboardTopModelsParams{
		TenantID: optionalTextValue(params.tenantID),
		UserID:   optionalTextValue(params.userID),
		Since:    params.since,
		Limit:    limit,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list dashboard top models failed", err)
		return
	}
	writeOK(w, rows)
}

func (s *Server) handleAdminDashboardTopTenants(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	rows, err := s.queries.ListDashboardTopTenants(r.Context(), dbgen.ListDashboardTopTenantsParams{
		TenantID: optionalTextValue(params.tenantID),
		UserID:   optionalTextValue(params.userID),
		Since:    params.since,
		Limit:    limit,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list dashboard top tenants failed", err)
		return
	}
	writeOK(w, rows)
}

func (s *Server) handleAdminDashboardRecentErrors(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	rows, err := s.queries.ListDashboardRecentErrors(r.Context(), dbgen.ListDashboardRecentErrorsParams{
		TenantID: optionalTextValue(params.tenantID),
		UserID:   optionalTextValue(params.userID),
		Since:    params.since,
		Limit:    limit,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list dashboard recent errors failed", err)
		return
	}
	writeOK(w, fromListDashboardRecentErrors(rows))
}

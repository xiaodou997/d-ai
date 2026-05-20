package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	dbgen "xiaodou/uni-ai-api/internal/db/gen"
)

// ============================================================================
// Tenant Model Grant Handlers
// ============================================================================

func (s *Server) handleAdminListTenantModelGrants(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}

	rows, err := s.queries.ListTenantModelGrants(r.Context(), tenantID)
	if err != nil {
		s.writeAdminServerError(w, r, "list tenant model grants failed", err)
		return
	}
	writeOK(w, fromListTenantModelGrants(rows))
}

func (s *Server) handleAdminGrantModelToTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}

	var req grantModelToTenantRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model_id is required")
		return
	}
	modelID, err := parseUUID(req.ModelID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid model_id")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.GrantModelToTenant(r.Context(), dbgen.GrantModelToTenantParams{
		TenantID:  tenantID,
		ModelID:   modelID,
		Status:    req.Status,
		CreatedBy: optionalTextValue(req.CreatedBy),
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromAiTenantModelGrant(row))
}

func (s *Server) handleAdminUpdateTenantModelGrantStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateTenantModelGrantStatus(r.Context(), dbgen.UpdateTenantModelGrantStatusParams{
		TenantID: tenantID,
		ModelID:  modelID,
		Status:   status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromAiTenantModelGrant(row))
}

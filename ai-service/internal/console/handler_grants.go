package console

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/unihub/ai-service/internal/domain"
	grantsvc "xiaodou/unihub/ai-service/internal/service/grant"
)

// ============================================================================
// Tenant Model Grant Handlers
// ============================================================================

func (s *Console) handleAdminListTenantModelGrants(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}

	grants, err := s.grantSvc.ListForTenant(r.Context(), tenantID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, listTenantModelGrantsFromDomain(grants))
}

func (s *Console) handleAdminGrantModelToTenant(w http.ResponseWriter, r *http.Request) {
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
	if _, err := parseUUID(req.ModelID); err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid model_id")
		return
	}

	grant, err := s.grantSvc.GrantToTenant(r.Context(), grantsvc.GrantInput{
		TenantID:  tenantID,
		ModelID:   req.ModelID,
		Status:    req.Status,
		CreatedBy: req.CreatedBy,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, tenantModelGrantFromDomain(grant))
}

func (s *Console) handleAdminUpdateTenantModelGrantStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	if _, ok := parseUUIDParam(w, r, "modelID"); !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	grant, err := s.grantSvc.UpdateStatus(r.Context(), tenantID, chi.URLParam(r, "modelID"), status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, tenantModelGrantFromDomain(grant))
}

// ---------------------------------------------------------------------------
// domain.TenantModelGrant → wire DTO（重建 pgtype 保 JSON 契约逐字节一致）
// ---------------------------------------------------------------------------

func tenantModelGrantFromDomain(g domain.TenantModelGrant) tenantModelGrantDTO {
	var id, modelID pgtype.UUID
	_ = id.Scan(g.ID)
	_ = modelID.Scan(g.ModelID)
	return tenantModelGrantDTO{
		ID:        id,
		TenantID:  g.TenantID,
		ModelID:   modelID,
		Status:    g.Status,
		CreatedBy: optionalTextValue(g.CreatedBy),
		CreatedAt: grantCreatedAtMillis(g),
	}
}

func listTenantModelGrantsFromDomain(grants []domain.TenantModelGrant) []listTenantModelGrantDTO {
	out := make([]listTenantModelGrantDTO, len(grants))
	for i, g := range grants {
		var id, modelID pgtype.UUID
		_ = id.Scan(g.ID)
		_ = modelID.Scan(g.ModelID)
		out[i] = listTenantModelGrantDTO{
			ID:             id,
			TenantID:       g.TenantID,
			ModelID:        modelID,
			Status:         g.Status,
			CreatedBy:      optionalTextValue(g.CreatedBy),
			CreatedAt:      grantCreatedAtMillis(g),
			ModelCode:      g.ModelCode,
			CapabilityType: g.CapabilityType,
		}
	}
	return out
}

func grantCreatedAtMillis(g domain.TenantModelGrant) *int64 {
	if g.CreatedAt.IsZero() {
		return nil
	}
	ms := g.CreatedAt.UnixMilli()
	return &ms
}

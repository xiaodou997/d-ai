package httpserver

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

func (s *Server) handleAdminListModels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.queries.ListAdminModels(r.Context())
	if err != nil {
		s.writeAdminServerError(w, r, "list models failed", err)
		return
	}
	writeOK(w, fromModels(rows))
}

func (s *Server) handleAdminCreateModel(w http.ResponseWriter, r *http.Request) {
	var req createModelRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelCode == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model_code is required")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.CreateModel(r.Context(), dbgen.CreateModelParams{
		ModelCode:              req.ModelCode,
		CapabilityType:         req.CapabilityType,
		ContextWindow:          optionalInt4(req.ContextWindow),
		DefaultMaxOutputTokens: int32OrDefault(req.DefaultMaxOutputTokens, 2048),
		MaxOutputTokens:        optionalInt4(req.MaxOutputTokens),
		Status:                 req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromModel(row))
}

func (s *Server) handleAdminUpdateModel(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	var req createModelRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelCode == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model_code is required")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	row, err := s.queries.UpdateModel(r.Context(), dbgen.UpdateModelParams{
		ID: modelID, ModelCode: req.ModelCode, CapabilityType: req.CapabilityType,
		ContextWindow: optionalInt4(req.ContextWindow), DefaultMaxOutputTokens: int32OrDefault(req.DefaultMaxOutputTokens, 2048),
		MaxOutputTokens: optionalInt4(req.MaxOutputTokens), Status: req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromModel(row))
}

func (s *Server) handleAdminUpdateModelStatus(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateModelStatus(r.Context(), dbgen.UpdateModelStatusParams{
		ID:     modelID,
		Status: status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromModel(row))
}

func (s *Server) handleAdminGetModelPrice(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	row, err := s.queries.GetModelPrice(r.Context(), modelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOK(w, nil)
			return
		}
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromGetModelPrice(row))
}

func (s *Server) handleAdminUpsertModelPrice(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	var req modelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if message := validateModelPriceCredits(req); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	imagePrices := jsonArrayOrDefault(req.ImagePrices)
	videoPrices := jsonArrayOrDefault(req.VideoPrices)
	row, err := s.queries.UpsertModelPrice(r.Context(), dbgen.UpsertModelPriceParams{
		ModelID:                 modelID,
		InputPricePer1m:         req.InputPricePer1M,
		OutputPricePer1m:        req.OutputPricePer1M,
		ImagePrices:             imagePrices,
		VideoPrices:             videoPrices,
		AudioTtsPricePer1mChars: req.AudioTTSPricePer1MChars,
		AudioSttPricePerMinute:  req.AudioSTTPricePerMinute,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpsertModelPrice(row))
}

// ============================================================================
// Tenant Model Price Override Handlers
// ============================================================================

func (s *Server) handleAdminListTenantModelPriceOverrides(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	rows, err := s.queries.ListTenantModelPriceOverrides(r.Context(), tenantID)
	if err != nil {
		s.writeAdminServerError(w, r, "list tenant model price overrides failed", err)
		return
	}
	writeOK(w, fromListTenantModelPriceOverrides(rows))
}

func (s *Server) handleAdminGetTenantModelPriceOverride(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	row, err := s.queries.GetTenantModelPriceOverride(r.Context(), dbgen.GetTenantModelPriceOverrideParams{
		TenantID: tenantID,
		ModelID:  modelID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOK(w, nil)
			return
		}
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromGetTenantModelPriceOverride(row))
}

func (s *Server) handleAdminUpsertTenantModelPriceOverride(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	var req modelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if message := validateModelPriceCredits(req); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	imagePrices := jsonArrayOrDefault(req.ImagePrices)
	videoPrices := jsonArrayOrDefault(req.VideoPrices)
	adminCtx, _ := adminFromContext(r.Context())
	row, err := s.queries.UpsertTenantModelPriceOverride(r.Context(), dbgen.UpsertTenantModelPriceOverrideParams{
		TenantID:                tenantID,
		ModelID:                 modelID,
		InputPricePer1m:         req.InputPricePer1M,
		OutputPricePer1m:        req.OutputPricePer1M,
		ImagePrices:             imagePrices,
		VideoPrices:             videoPrices,
		AudioTtsPricePer1mChars: req.AudioTTSPricePer1MChars,
		AudioSttPricePerMinute:  req.AudioSTTPricePerMinute,
		CreatedBy:               optionalTextString(adminCtx.Actor),
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpsertTenantModelPriceOverride(row))
}

func (s *Server) handleAdminDeleteTenantModelPriceOverride(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	err := s.queries.DeleteTenantModelPriceOverride(r.Context(), dbgen.DeleteTenantModelPriceOverrideParams{
		TenantID: tenantID,
		ModelID:  modelID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, nil)
}

// ============================================================================
// Tenant User Price Handlers (租户售价 - 租户对用户的定价)
// ============================================================================

func (s *Server) handleAdminListTenantUserPrices(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	rows, err := s.queries.ListTenantUserPrices(r.Context(), tenantID)
	if err != nil {
		s.writeAdminServerError(w, r, "list tenant user prices failed", err)
		return
	}
	writeOK(w, fromListTenantUserPrices(rows))
}

func (s *Server) handleAdminGetTenantUserPrice(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	row, err := s.queries.GetTenantUserPrice(r.Context(), dbgen.GetTenantUserPriceParams{
		TenantID: tenantID,
		ModelID:  modelID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOK(w, nil)
			return
		}
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromGetTenantUserPrice(row))
}

func (s *Server) handleAdminUpsertTenantUserPrice(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	var req modelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if message := validateModelPriceCredits(req); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	imagePrices := jsonArrayOrDefault(req.ImagePrices)
	videoPrices := jsonArrayOrDefault(req.VideoPrices)
	adminCtx, _ := adminFromContext(r.Context())
	row, err := s.queries.UpsertTenantUserPrice(r.Context(), dbgen.UpsertTenantUserPriceParams{
		TenantID:                tenantID,
		ModelID:                 modelID,
		InputPricePer1m:         req.InputPricePer1M,
		OutputPricePer1m:        req.OutputPricePer1M,
		ImagePrices:             imagePrices,
		VideoPrices:             videoPrices,
		AudioTtsPricePer1mChars: req.AudioTTSPricePer1MChars,
		AudioSttPricePerMinute:  req.AudioSTTPricePerMinute,
		CreatedBy:               optionalTextString(adminCtx.Actor),
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpsertTenantUserPrice(row))
}

func (s *Server) handleAdminDeleteTenantUserPrice(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	err := s.queries.DeleteTenantUserPrice(r.Context(), dbgen.DeleteTenantUserPriceParams{
		TenantID: tenantID,
		ModelID:  modelID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, nil)
}

// ============================================================================
// Model Route Handlers
// ============================================================================

func (s *Server) handleAdminListModelRoutes(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}

	rows, err := s.queries.ListModelRoutes(r.Context(), modelID)
	if err != nil {
		s.writeAdminServerError(w, r, "list model routes failed", err)
		return
	}
	writeOK(w, fromListModelRoutes(rows))
}

func (s *Server) handleAdminCreateModelRoute(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}

	var req createModelRouteRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	if req.UpstreamDeploymentID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "upstream_deployment_id is required")
		return
	}

	upstreamDeploymentID, err := parseUUID(req.UpstreamDeploymentID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid upstream_deployment_id")
		return
	}
	row, err := s.queries.CreateModelRoute(r.Context(), dbgen.CreateModelRouteParams{
		ModelID:              modelID,
		UpstreamDeploymentID: upstreamDeploymentID,
		Priority:             int32OrDefault(req.Priority, 100),
		Weight:               int32OrDefault(req.Weight, 100),
		SupportsStream:       boolOrDefault(req.SupportsStream, true),
		Status:               req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromModelRoute(row))
}

func (s *Server) handleAdminGetModelRoute(w http.ResponseWriter, r *http.Request) {
	routeID, ok := parseUUIDParam(w, r, "routeID")
	if !ok {
		return
	}

	row, err := s.queries.GetModelRoute(r.Context(), routeID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromModelRoute(row))
}

func (s *Server) handleAdminUpdateModelRoute(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	routeID, ok := parseUUIDParam(w, r, "routeID")
	if !ok {
		return
	}
	var req createModelRouteRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	if req.UpstreamDeploymentID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "upstream_deployment_id is required")
		return
	}

	upstreamDeploymentID, err := parseUUID(req.UpstreamDeploymentID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid upstream_deployment_id")
		return
	}
	row, err := s.queries.UpdateModelRoute(r.Context(), dbgen.UpdateModelRouteParams{
		ModelID:              modelID,
		ID:                   routeID,
		UpstreamDeploymentID: upstreamDeploymentID,
		Priority:             int32OrDefault(req.Priority, 100),
		Weight:               int32OrDefault(req.Weight, 100),
		SupportsStream:       boolOrDefault(req.SupportsStream, true),
		Status:               req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromModelRoute(row))
}

func (s *Server) handleAdminUpdateModelRouteStatus(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	routeID, ok := parseUUIDParam(w, r, "routeID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateModelRouteStatus(r.Context(), dbgen.UpdateModelRouteStatusParams{
		ModelID: modelID,
		ID:      routeID,
		Status:  status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromModelRoute(row))
}

func (s *Server) handleAdminDeleteModelRoute(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	routeID, ok := parseUUIDParam(w, r, "routeID")
	if !ok {
		return
	}

	// Delete route using direct SQL since no generated function exists
	result, err := s.postgres.Exec(r.Context(),
		"DELETE FROM ai_model_routes WHERE model_id = $1 AND id = $2",
		modelID, routeID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "route not found")
		return
	}
	writeOK(w, map[string]string{"status": "deleted"})
}

func validateModelPriceCredits(req modelPriceRequest) string {
	fields := map[string]int64{
		"input_price_per_1m":           req.InputPricePer1M,
		"output_price_per_1m":          req.OutputPricePer1M,
		"audio_tts_price_per_1m_chars": req.AudioTTSPricePer1MChars,
		"audio_stt_price_per_minute":   req.AudioSTTPricePerMinute,
	}
	for name, value := range fields {
		if value < 0 {
			return fmt.Sprintf("%s must be a non-negative integer credit value", name)
		}
	}
	return ""
}

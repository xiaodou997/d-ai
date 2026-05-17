package httpserver

import (
	"fmt"
	"net/http"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

// ============================================================================
// Upstream Deployment Handlers
// ============================================================================

func (s *Server) handleAdminListUpstreamDeployments(w http.ResponseWriter, r *http.Request) {
	// endpoint_id is optional - if not provided, list all deployments
	endpointIDParam := r.URL.Query().Get("endpoint_id")

	var rows []dbgen.ListUpstreamDeploymentsRow
	var err error

	if endpointIDParam != "" {
		endpointID, parseErr := parseUUID(endpointIDParam)
		if parseErr != nil {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid endpoint_id")
			return
		}
		rows, err = s.queries.ListUpstreamDeployments(r.Context(), endpointID)
	} else {
		// List all deployments using direct query since no endpoint_id filter needed
		query := `
			SELECT
			  ud.id,
			  ud.endpoint_id,
			  ud.name,
			  ud.upstream_model,
			  ud.capability_type,
			  ud.upstream_protocol,
			  ud.request_path,
			  ud.upstream_parameters,
			  ud.tags,
			  ud.health_status,
			  ud.last_health_check_at,
			  ud.last_health_error,
			  ud.status,
			  ud.created_at,
			  ud.updated_at,
			  e.name AS endpoint_name,
			  e.base_url,
			  e.weight AS endpoint_weight,
			  p.id AS provider_id,
			  p.code AS provider_code,
			  p.name AS provider_name
			FROM ai_upstream_deployments ud
			JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
			JOIN ai_providers p ON p.id = e.provider_id
			ORDER BY ud.name ASC`
		dbRows, queryErr := s.postgres.Query(r.Context(), query)
		if queryErr != nil {
			err = queryErr
		} else {
			defer dbRows.Close()
			for dbRows.Next() {
				var row dbgen.ListUpstreamDeploymentsRow
				if scanErr := dbRows.Scan(
					&row.ID,
					&row.EndpointID,
					&row.Name,
					&row.UpstreamModel,
					&row.CapabilityType,
					&row.UpstreamProtocol,
					&row.RequestPath,
					&row.UpstreamParameters,
					&row.Tags,
					&row.HealthStatus,
					&row.LastHealthCheckAt,
					&row.LastHealthError,
					&row.Status,
					&row.CreatedAt,
					&row.UpdatedAt,
					&row.EndpointName,
					&row.BaseUrl,
					&row.EndpointWeight,
					&row.ProviderID,
					&row.ProviderCode,
					&row.ProviderName,
				); scanErr != nil {
					err = scanErr
					break
				}
				rows = append(rows, row)
			}
			if closeErr := dbRows.Err(); closeErr != nil {
				err = closeErr
			}
		}
	}

	if err != nil {
		s.writeAdminServerError(w, r, "list upstream deployments failed", err)
		return
	}
	writeOK(w, fromListUpstreamDeployments(rows))
}

func (s *Server) handleAdminCreateUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
	var req createUpstreamDeploymentRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.EndpointID == "" || req.UpstreamModel == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "endpoint_id, upstream_model and name are required")
		return
	}
	endpointID, err := parseUUID(req.EndpointID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid endpoint_id")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.UpstreamProtocol == "" {
		req.UpstreamProtocol = defaultProtocol
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.CreateUpstreamDeployment(r.Context(), dbgen.CreateUpstreamDeploymentParams{
		EndpointID:         endpointID,
		Name:               req.Name,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		RequestPath:        optionalTextParam(req.RequestPath),
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		Tags:               jsonObjectOrDefault(req.Tags),
		Status:             req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromAiUpstreamDeployment(row))
}

func (s *Server) handleAdminGetUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	row, err := s.queries.GetUpstreamDeployment(r.Context(), deploymentID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromGetUpstreamDeployment(row))
}

func (s *Server) handleAdminUpdateUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	var req createUpstreamDeploymentRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.UpstreamModel == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "upstream_model and name are required")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.UpstreamProtocol == "" {
		req.UpstreamProtocol = defaultProtocol
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.UpdateUpstreamDeployment(r.Context(), dbgen.UpdateUpstreamDeploymentParams{
		ID:                 deploymentID,
		Name:               req.Name,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		RequestPath:        optionalTextParam(req.RequestPath),
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		Tags:               jsonObjectOrDefault(req.Tags),
		Status:             req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromAiUpstreamDeployment(row))
}

func (s *Server) handleAdminUpdateUpstreamDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateUpstreamDeploymentStatus(r.Context(), dbgen.UpdateUpstreamDeploymentStatusParams{
		ID:     deploymentID,
		Status: status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromAiUpstreamDeployment(row))
}

// ============================================================================
// Upstream Deployment Cost Price Handlers
// ============================================================================

func (s *Server) handleAdminListUpstreamDeploymentCostPrices(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	rows, err := s.queries.ListUpstreamDeploymentCostPrices(r.Context(), deploymentID)
	if err != nil {
		s.writeAdminServerError(w, r, "list upstream deployment cost prices failed", err)
		return
	}
	writeOK(w, fromListUpstreamDeploymentCostPrices(rows))
}

func (s *Server) handleAdminCreateUpstreamDeploymentCostPrice(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	var req createUpstreamDeploymentCostPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.Currency == "" {
		req.Currency = "CNY_CREDITS"
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	if message := validateUpstreamDeploymentCostPriceCredits(req); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	effectiveFrom, err := parseEffectiveFrom(req.EffectiveFrom)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid effective_from")
		return
	}

	row, err := s.queries.CreateUpstreamDeploymentCostPrice(r.Context(), dbgen.CreateUpstreamDeploymentCostPriceParams{
		UpstreamDeploymentID: deploymentID,
		CapabilityType:       req.CapabilityType,
		Currency:             req.Currency,
		InputCostPer1m:       req.InputCostPer1M,
		OutputCostPer1m:      req.OutputCostPer1M,
		RequestCost:          req.RequestCost,
		ImageCost:            req.ImageCost,
		ImageSizePrices:      jsonObjectOrDefault(req.ImageSizePrices),
		VideoCostPerSecond:   req.VideoCostPerSecond,
		EffectiveFrom:        effectiveFrom,
		Status:               req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromCreateUpstreamDeploymentCostPrice(row))
}

func (s *Server) handleAdminUpdateUpstreamDeploymentCostPrice(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	priceID, ok := parseUUIDParam(w, r, "priceID")
	if !ok {
		return
	}
	var req createUpstreamDeploymentCostPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.Currency == "" {
		req.Currency = "CNY_CREDITS"
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	if message := validateUpstreamDeploymentCostPriceCredits(req); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	effectiveFrom, err := parseEffectiveFrom(req.EffectiveFrom)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid effective_from")
		return
	}

	row, err := s.queries.UpdateUpstreamDeploymentCostPrice(r.Context(), dbgen.UpdateUpstreamDeploymentCostPriceParams{
		UpstreamDeploymentID: deploymentID,
		ID:                   priceID,
		CapabilityType:       req.CapabilityType,
		Currency:             req.Currency,
		InputCostPer1m:       req.InputCostPer1M,
		OutputCostPer1m:      req.OutputCostPer1M,
		RequestCost:          req.RequestCost,
		ImageCost:            req.ImageCost,
		ImageSizePrices:      jsonObjectOrDefault(req.ImageSizePrices),
		VideoCostPerSecond:   req.VideoCostPerSecond,
		EffectiveFrom:        effectiveFrom,
		Status:               req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpdateUpstreamDeploymentCostPrice(row))
}

func (s *Server) handleAdminUpdateUpstreamDeploymentCostPriceStatus(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	priceID, ok := parseUUIDParam(w, r, "priceID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateUpstreamDeploymentCostPriceStatus(r.Context(), dbgen.UpdateUpstreamDeploymentCostPriceStatusParams{
		UpstreamDeploymentID: deploymentID,
		ID:                   priceID,
		Status:               status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpdateUpstreamDeploymentCostPriceStatus(row))
}

func validateUpstreamDeploymentCostPriceCredits(req createUpstreamDeploymentCostPriceRequest) string {
	fields := map[string]int64{
		"input_cost_per_1m":     req.InputCostPer1M,
		"output_cost_per_1m":    req.OutputCostPer1M,
		"request_cost":          req.RequestCost,
		"image_cost":            req.ImageCost,
		"video_cost_per_second": req.VideoCostPerSecond,
	}
	for name, value := range fields {
		if value < 0 {
			return fmt.Sprintf("%s must be a non-negative integer credit value", name)
		}
	}
	return ""
}

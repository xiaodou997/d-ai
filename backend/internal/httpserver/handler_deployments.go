package httpserver

import (
	"net/http"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

// ============================================================================
// Upstream Deployment Handlers
// ============================================================================

func (s *Server) handleAdminListUpstreamDeployments(w http.ResponseWriter, r *http.Request) {
	endpointIDParam := r.URL.Query().Get("endpoint_id")
	poolIDParam := r.URL.Query().Get("credential_pool_id")

	var rows []dbgen.ListUpstreamDeploymentsRow
	var err error

	if endpointIDParam != "" {
		endpointID, parseErr := parseUUID(endpointIDParam)
		if parseErr != nil {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid endpoint_id")
			return
		}
		rows, err = s.queries.ListUpstreamDeployments(r.Context(), endpointID)
	} else if poolIDParam != "" {
		poolID, parseErr := parseUUID(poolIDParam)
		if parseErr != nil {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid credential_pool_id")
			return
		}
		const poolQuery = `
			SELECT
			  ud.id,
			  ud.endpoint_id,
			  ud.credential_pool_id,
			  ud.upstream_model,
			  ud.capability_type,
			  ud.upstream_protocol,
			  ud.request_path,
			  ud.upstream_parameters,
			  ud.pricing,
			  ud.health_status,
			  ud.last_health_check_at,
			  ud.last_health_error,
			  ud.status,
			  ud.created_at,
			  ud.updated_at,
			  'pool'                          AS credential_source,
			  ''                              AS endpoint_name,
			  ''                              AS base_url,
			  0                               AS endpoint_weight,
			  '00000000-0000-0000-0000-000000000000'::uuid AS provider_id,
			  ''                              AS provider_code,
			  ''                              AS provider_name,
			  COALESCE(cp.name, '')           AS pool_name,
			  COALESCE(cp.fixed_provider_type, '') AS fixed_provider_type
			FROM ai_upstream_deployments ud
			LEFT JOIN ai_credential_pools cp ON cp.id = ud.credential_pool_id
			WHERE ud.credential_pool_id = $1
			ORDER BY ud.upstream_model ASC`
		dbRows, queryErr := s.postgres.Query(r.Context(), poolQuery, poolID)
		if queryErr != nil {
			err = queryErr
		} else {
			defer dbRows.Close()
			for dbRows.Next() {
				var row dbgen.ListUpstreamDeploymentsRow
				if scanErr := dbRows.Scan(
					&row.ID, &row.EndpointID, &row.CredentialPoolID,
					&row.UpstreamModel, &row.CapabilityType, &row.UpstreamProtocol,
					&row.RequestPath, &row.UpstreamParameters, &row.Pricing,
					&row.HealthStatus, &row.LastHealthCheckAt, &row.LastHealthError,
					&row.Status, &row.CreatedAt, &row.UpdatedAt,
					&row.CredentialSource, &row.EndpointName, &row.BaseUrl,
					&row.EndpointWeight, &row.ProviderID, &row.ProviderCode,
					&row.ProviderName, &row.PoolName, &row.FixedProviderType,
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
	} else {
		const query = `
			SELECT
			  ud.id,
			  ud.endpoint_id,
			  ud.credential_pool_id,
			  ud.upstream_model,
			  ud.capability_type,
			  ud.upstream_protocol,
			  ud.request_path,
			  ud.upstream_parameters,
			  ud.pricing,
			  ud.health_status,
			  ud.last_health_check_at,
			  ud.last_health_error,
			  ud.status,
			  ud.created_at,
			  ud.updated_at,
			  CASE WHEN ud.endpoint_id IS NOT NULL THEN 'endpoint' ELSE 'pool' END AS credential_source,
			  COALESCE(e.name, '')          AS endpoint_name,
			  COALESCE(e.base_url, '')      AS base_url,
			  COALESCE(e.weight, 0)         AS endpoint_weight,
			  COALESCE(p.id, '00000000-0000-0000-0000-000000000000'::uuid) AS provider_id,
			  COALESCE(p.code, '')          AS provider_code,
			  COALESCE(p.name, '')          AS provider_name,
			  COALESCE(cp.name, '')         AS pool_name,
			  COALESCE(cp.fixed_provider_type, '') AS fixed_provider_type
			FROM ai_upstream_deployments ud
			LEFT JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
			LEFT JOIN ai_providers p ON p.id = e.provider_id
			LEFT JOIN ai_credential_pools cp ON cp.id = ud.credential_pool_id
			ORDER BY ud.upstream_model ASC`
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
					&row.CredentialPoolID,
					&row.UpstreamModel,
					&row.CapabilityType,
					&row.UpstreamProtocol,
					&row.RequestPath,
					&row.UpstreamParameters,
					&row.Pricing,
					&row.HealthStatus,
					&row.LastHealthCheckAt,
					&row.LastHealthError,
					&row.Status,
					&row.CreatedAt,
					&row.UpdatedAt,
					&row.CredentialSource,
					&row.EndpointName,
					&row.BaseUrl,
					&row.EndpointWeight,
					&row.ProviderID,
					&row.ProviderCode,
					&row.ProviderName,
					&row.PoolName,
					&row.FixedProviderType,
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
	if req.UpstreamModel == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "upstream_model is required")
		return
	}

	isEndpoint := req.EndpointID != ""
	isPool := req.CredentialPoolID != ""
	if isEndpoint == isPool {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "exactly one of endpoint_id or credential_pool_id must be set")
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

	if isPool {
		poolID, err := parseUUID(req.CredentialPoolID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid credential_pool_id")
			return
		}
		row, err := s.queries.CreatePoolDeployment(r.Context(), dbgen.CreatePoolDeploymentParams{
			CredentialPoolID: poolID,
			UpstreamModel:    req.UpstreamModel,
			CapabilityType:   req.CapabilityType,
			UpstreamProtocol: req.UpstreamProtocol,
		})
		if err != nil {
			writeDBErr(w, err)
			return
		}
		writeOK(w, fromAiUpstreamDeployment(row))
		return
	}

	endpointID, err := parseUUID(req.EndpointID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid endpoint_id")
		return
	}
	row, err := s.queries.CreateUpstreamDeployment(r.Context(), dbgen.CreateUpstreamDeploymentParams{
		EndpointID:         endpointID,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		RequestPath:        optionalTextParam(req.RequestPath),
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		Pricing:            jsonObjectOrDefault(req.Pricing),
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
	if req.UpstreamModel == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "upstream_model is required")
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
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		RequestPath:        optionalTextParam(req.RequestPath),
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		Pricing:            jsonObjectOrDefault(req.Pricing),
		Status:             req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromAiUpstreamDeployment(row))
}

func (s *Server) handleAdminDeleteUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	// Cascade: delete model routes and conversation bindings, then the deployment itself.
	_, err := s.postgres.Exec(r.Context(),
		`DELETE FROM ai_model_routes WHERE upstream_deployment_id = $1`,
		deploymentID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	_, err = s.postgres.Exec(r.Context(),
		`DELETE FROM ai_conversation_bindings WHERE upstream_deployment_id = $1`,
		deploymentID)
	if err != nil {
		writeDBErr(w, err)
		return
	}

	result, err := s.postgres.Exec(r.Context(),
		`DELETE FROM ai_upstream_deployments WHERE id = $1`,
		deploymentID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "deployment not found")
		return
	}
	writeOK(w, map[string]string{"status": "deleted"})
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

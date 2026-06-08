package console

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
)

// nullableUUIDParam parses an optional UUID string into pgtype.UUID; an empty
// string yields a NULL value. Returns ok=false on a malformed non-empty value.
func nullableUUIDParam(s string) (pgtype.UUID, bool) {
	if strings.TrimSpace(s) == "" {
		return pgtype.UUID{}, true
	}
	u, err := parseUUID(s)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return u, true
}

// numericFloatVal reads a pgtype.Numeric into float64 (0 for NULL/invalid).
func numericFloatVal(n pgtype.Numeric) float64 {
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

// costMultiplierNumeric converts an optional multiplier to pgtype.Numeric.
// nil → NULL (inherit endpoint binding); the billing layer COALESCEs to 1.
func costMultiplierNumeric(v *float64) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(*v, 'f', -1, 64))
	return n
}

// ============================================================================
// Upstream Deployment Handlers
// ============================================================================

const upstreamDeploymentListBaseQuery = `
SELECT
  ud.id,
  ud.endpoint_id,
  ud.credential_pool_id,
  ud.upstream_model,
  ud.capability_type,
  ud.upstream_protocol,
  ud.request_path,
  ud.upstream_parameters,
  ud.price_book_id,
  ud.cost_multiplier,
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
LEFT JOIN ai_credential_pools cp ON cp.id = ud.credential_pool_id`

func (s *Console) handleAdminListUpstreamDeployments(w http.ResponseWriter, r *http.Request) {
	query, args, err := buildUpstreamDeploymentListQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	dbRows, queryErr := s.postgres.Query(r.Context(), query, args...)
	if queryErr != nil {
		s.writeAdminServerError(w, r, "list upstream deployments failed", queryErr)
		return
	}
	defer dbRows.Close()

	rows, scanErr := scanUpstreamDeploymentRows(dbRows)
	if scanErr != nil {
		s.writeAdminServerError(w, r, "list upstream deployments failed", scanErr)
		return
	}

	writeOK(w, fromListUpstreamDeployments(rows))
}

func buildUpstreamDeploymentListQuery(r *http.Request) (string, []any, error) {
	endpointIDParam := r.URL.Query().Get("endpoint_id")
	providerIDParam := r.URL.Query().Get("provider_id")
	poolIDParam := r.URL.Query().Get("credential_pool_id")

	where := ""
	args := make([]any, 0, 1)

	switch {
	case endpointIDParam != "":
		endpointID, err := parseUUID(endpointIDParam)
		if err != nil {
			return "", nil, fmt.Errorf("invalid endpoint_id")
		}
		where = "WHERE ud.endpoint_id = $1"
		args = append(args, endpointID)
	case providerIDParam != "":
		providerID, err := parseUUID(providerIDParam)
		if err != nil {
			return "", nil, fmt.Errorf("invalid provider_id")
		}
		where = "WHERE e.provider_id = $1"
		args = append(args, providerID)
	case poolIDParam != "":
		poolID, err := parseUUID(poolIDParam)
		if err != nil {
			return "", nil, fmt.Errorf("invalid credential_pool_id")
		}
		where = "WHERE ud.credential_pool_id = $1"
		args = append(args, poolID)
	}

	query := strings.TrimSpace(upstreamDeploymentListBaseQuery)
	if where != "" {
		query += "\n" + where
	}
	query += "\nORDER BY ud.upstream_model ASC"
	return query, args, nil
}

func scanUpstreamDeploymentRows(rows pgx.Rows) ([]dbgen.ListUpstreamDeploymentsRow, error) {
	items := make([]dbgen.ListUpstreamDeploymentsRow, 0)
	for rows.Next() {
		var row dbgen.ListUpstreamDeploymentsRow
		if err := rows.Scan(
			&row.ID,
			&row.EndpointID,
			&row.CredentialPoolID,
			&row.UpstreamModel,
			&row.CapabilityType,
			&row.UpstreamProtocol,
			&row.RequestPath,
			&row.UpstreamParameters,
			&row.PriceBookID,
			&row.CostMultiplier,
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
		); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Console) handleAdminCreateUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
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
	priceBookID, ok := nullableUUIDParam(req.PriceBookID)
	if !ok {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid price_book_id")
		return
	}
	row, err := s.queries.CreateUpstreamDeployment(r.Context(), dbgen.CreateUpstreamDeploymentParams{
		EndpointID:         endpointID,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		RequestPath:        optionalTextParam(req.RequestPath),
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		PriceBookID:        priceBookID,
		CostMultiplier:     costMultiplierNumeric(req.CostMultiplier),
		Status:             req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromAiUpstreamDeployment(row))
}

func (s *Console) handleAdminGetUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
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

func (s *Console) handleAdminUpdateUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
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

	priceBookID, ok := nullableUUIDParam(req.PriceBookID)
	if !ok {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid price_book_id")
		return
	}
	row, err := s.queries.UpdateUpstreamDeployment(r.Context(), dbgen.UpdateUpstreamDeploymentParams{
		ID:                 deploymentID,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		RequestPath:        optionalTextParam(req.RequestPath),
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		PriceBookID:        priceBookID,
		CostMultiplier:     costMultiplierNumeric(req.CostMultiplier),
		Status:             req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromAiUpstreamDeployment(row))
}

func (s *Console) handleAdminDeleteUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
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

func (s *Console) handleAdminUpdateUpstreamDeploymentStatus(w http.ResponseWriter, r *http.Request) {
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

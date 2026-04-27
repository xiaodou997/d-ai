package httpserver

import (
	"net/http"
	"strconv"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

type runtimeLimitPolicyRequest struct {
	ScopeType        string `json:"scope_type"`
	ScopeID          string `json:"scope_id"`
	CapabilityType   string `json:"capability_type"`
	ModelCode        string `json:"model_code"`
	RPMLimit         *int32 `json:"rpm_limit"`
	TPMLimit         *int32 `json:"tpm_limit"`
	ConcurrencyLimit *int32 `json:"concurrency_limit"`
	Status           string `json:"status"`
	CreatedBy        string `json:"created_by"`
}

func (s *Server) handleAdminListRuntimeLimitPolicies(w http.ResponseWriter, r *http.Request) {
	limit := int32(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeAdminError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = int32(parsed)
	}
	rows, err := s.queries.ListRuntimeLimitPolicies(r.Context(), dbgen.ListRuntimeLimitPoliciesParams{
		ScopeType:      optionalTextValue(r.URL.Query().Get("scope_type")),
		ScopeID:        optionalTextValue(r.URL.Query().Get("scope_id")),
		CapabilityType: optionalTextValue(r.URL.Query().Get("capability_type")),
		ModelCode:      optionalTextValue(r.URL.Query().Get("model_code")),
		Status:         optionalTextValue(r.URL.Query().Get("status")),
		Limit:          limit,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list runtime limit policies failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateRuntimeLimitPolicy(w http.ResponseWriter, r *http.Request) {
	var req runtimeLimitPolicyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if !validateRuntimeLimitPolicyRequest(w, &req) {
		return
	}
	row, err := s.queries.CreateRuntimeLimitPolicy(r.Context(), dbgen.CreateRuntimeLimitPolicyParams{
		ScopeType:        req.ScopeType,
		ScopeID:          req.ScopeID,
		CapabilityType:   req.CapabilityType,
		ModelCode:        optionalTextValue(req.ModelCode),
		RpmLimit:         optionalInt4(req.RPMLimit),
		TpmLimit:         optionalInt4(req.TPMLimit),
		ConcurrencyLimit: optionalInt4(req.ConcurrencyLimit),
		Status:           req.Status,
		CreatedBy:        optionalTextValue(req.CreatedBy),
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminUpdateRuntimeLimitPolicy(w http.ResponseWriter, r *http.Request) {
	policyID, ok := parseUUIDParam(w, r, "policyID")
	if !ok {
		return
	}
	var req runtimeLimitPolicyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if !validateRuntimeLimitPolicyRequest(w, &req) {
		return
	}
	row, err := s.queries.UpdateRuntimeLimitPolicy(r.Context(), dbgen.UpdateRuntimeLimitPolicyParams{
		ID:               policyID,
		ScopeType:        req.ScopeType,
		ScopeID:          req.ScopeID,
		CapabilityType:   req.CapabilityType,
		ModelCode:        optionalTextValue(req.ModelCode),
		RpmLimit:         optionalInt4(req.RPMLimit),
		TpmLimit:         optionalInt4(req.TPMLimit),
		ConcurrencyLimit: optionalInt4(req.ConcurrencyLimit),
		Status:           req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateRuntimeLimitPolicyStatus(w http.ResponseWriter, r *http.Request) {
	policyID, ok := parseUUIDParam(w, r, "policyID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}
	row, err := s.queries.UpdateRuntimeLimitPolicyStatus(r.Context(), dbgen.UpdateRuntimeLimitPolicyStatusParams{
		ID:     policyID,
		Status: status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func validateRuntimeLimitPolicyRequest(w http.ResponseWriter, req *runtimeLimitPolicyRequest) bool {
	if req.ScopeType == "" || req.ScopeID == "" {
		writeAdminError(w, http.StatusBadRequest, "scope_type and scope_id are required")
		return false
	}
	switch req.ScopeType {
	case "tenant", "user", "api_key", "provider", "endpoint":
	default:
		writeAdminError(w, http.StatusBadRequest, "invalid scope_type")
		return false
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	switch req.CapabilityType {
	case "chat", "image", "video", "embedding", "audio", "rerank":
	default:
		writeAdminError(w, http.StatusBadRequest, "invalid capability_type")
		return false
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	switch req.Status {
	case "active", "inactive", "disabled":
	default:
		writeAdminError(w, http.StatusBadRequest, "invalid status")
		return false
	}
	if req.RPMLimit == nil && req.TPMLimit == nil && req.ConcurrencyLimit == nil {
		writeAdminError(w, http.StatusBadRequest, "at least one limit is required")
		return false
	}
	if !positiveOptionalInt32(req.RPMLimit) || !positiveOptionalInt32(req.TPMLimit) || !positiveOptionalInt32(req.ConcurrencyLimit) {
		writeAdminError(w, http.StatusBadRequest, "limits must be positive")
		return false
	}
	return true
}

func positiveOptionalInt32(value *int32) bool {
	return value == nil || *value > 0
}

package httpserver

import (
	"net/http"

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
	rows, err := s.queries.ListLimitPolicies(r.Context())
	if err != nil {
		s.writeAdminServerError(w, r, "list runtime limit policies failed", err)
		return
	}
	writeOK(w, fromLimitPolicies(rows))
}

func (s *Server) handleAdminCreateRuntimeLimitPolicy(w http.ResponseWriter, r *http.Request) {
	var req runtimeLimitPolicyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if !validateRuntimeLimitPolicyRequest(w, &req) {
		return
	}
	row, err := s.queries.CreateLimitPolicy(r.Context(), dbgen.CreateLimitPolicyParams{
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
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromLimitPolicy(row))
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
	row, err := s.queries.UpdateLimitPolicy(r.Context(), dbgen.UpdateLimitPolicyParams{
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
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromLimitPolicy(row))
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
	row, err := s.queries.UpdateLimitPolicyStatus(r.Context(), dbgen.UpdateLimitPolicyStatusParams{
		ID:     policyID,
		Status: status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromLimitPolicy(row))
}

func validateRuntimeLimitPolicyRequest(w http.ResponseWriter, req *runtimeLimitPolicyRequest) bool {
	if req.ScopeType == "" || req.ScopeID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "scope_type and scope_id are required")
		return false
	}
	switch req.ScopeType {
	case "tenant", "user", "api_key", "provider", "endpoint":
	default:
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid scope_type")
		return false
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	switch req.CapabilityType {
	case "chat", "image", "video", "embedding", "audio", "rerank":
	default:
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid capability_type")
		return false
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	switch req.Status {
	case "active", "inactive", "disabled":
	default:
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid status")
		return false
	}
	if req.RPMLimit == nil && req.TPMLimit == nil && req.ConcurrencyLimit == nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "at least one limit is required")
		return false
	}
	if !positiveOptionalInt32(req.RPMLimit) || !positiveOptionalInt32(req.TPMLimit) || !positiveOptionalInt32(req.ConcurrencyLimit) {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "limits must be positive")
		return false
	}
	return true
}

func positiveOptionalInt32(value *int32) bool {
	return value == nil || *value > 0
}
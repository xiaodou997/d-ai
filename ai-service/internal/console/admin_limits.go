package console

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/unihub/ai-service/internal/domain"
	limitsvc "xiaodou/unihub/ai-service/internal/service/limit"
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

func (r *runtimeLimitPolicyRequest) toInput() limitsvc.PolicyInput {
	return limitsvc.PolicyInput{
		ScopeType:        r.ScopeType,
		ScopeID:          r.ScopeID,
		CapabilityType:   r.CapabilityType,
		ModelCode:        r.ModelCode,
		RpmLimit:         r.RPMLimit,
		TpmLimit:         r.TPMLimit,
		ConcurrencyLimit: r.ConcurrencyLimit,
		Status:           r.Status,
		CreatedBy:        r.CreatedBy,
	}
}

func (s *Console) handleAdminListRuntimeLimitPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := s.limitSvc.List(r.Context())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, limitPoliciesFromDomain(policies))
}

func (s *Console) handleAdminCreateRuntimeLimitPolicy(w http.ResponseWriter, r *http.Request) {
	var req runtimeLimitPolicyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if !validateRuntimeLimitPolicyRequest(w, &req) {
		return
	}
	policy, err := s.limitSvc.Create(r.Context(), req.toInput())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, limitPolicyFromDomain(policy))
}

func (s *Console) handleAdminUpdateRuntimeLimitPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "policyID"); !ok {
		return
	}
	var req runtimeLimitPolicyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if !validateRuntimeLimitPolicyRequest(w, &req) {
		return
	}
	policy, err := s.limitSvc.Update(r.Context(), chi.URLParam(r, "policyID"), req.toInput())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, limitPolicyFromDomain(policy))
}

func (s *Console) handleAdminUpdateRuntimeLimitPolicyStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "policyID"); !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}
	policy, err := s.limitSvc.UpdateStatus(r.Context(), chi.URLParam(r, "policyID"), status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, limitPolicyFromDomain(policy))
}

// validateRuntimeLimitPolicyRequest preserves the exact 400 messages of the
// legacy handler (前端零改). The service layer re-validates the same rules as a
// defense-in-depth invariant, so the two never diverge.
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

// ---------------------------------------------------------------------------
// domain.RuntimeLimitPolicy → wire DTO（重建 pgtype 保 JSON 契约逐字节一致）
// ---------------------------------------------------------------------------

func limitPolicyFromDomain(p domain.RuntimeLimitPolicy) limitPolicyDTO {
	var id pgtype.UUID
	_ = id.Scan(p.ID)
	return limitPolicyDTO{
		ID:               id,
		ScopeType:        p.ScopeType,
		ScopeID:          p.ScopeID,
		CapabilityType:   p.CapabilityType,
		ModelCode:        textPtrToPg(p.ModelCode),
		RpmLimit:         optionalInt4(p.RpmLimit),
		TpmLimit:         optionalInt4(p.TpmLimit),
		ConcurrencyLimit: optionalInt4(p.ConcurrencyLimit),
		Status:           p.Status,
		CreatedBy:        optionalTextValue(p.CreatedBy),
		CreatedAt:        timeToMillisPtr(p.CreatedAt),
		UpdatedAt:        timeToMillisPtr(p.UpdatedAt),
	}
}

func limitPoliciesFromDomain(policies []domain.RuntimeLimitPolicy) []limitPolicyDTO {
	out := make([]limitPolicyDTO, len(policies))
	for i, p := range policies {
		out[i] = limitPolicyFromDomain(p)
	}
	return out
}

package console

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"xiaodou/unihub/ai-service/internal/credits"
	"xiaodou/unihub/ai-service/internal/domain"
	apikeysvc "xiaodou/unihub/ai-service/internal/service/apikey"
)

// ============================================================================
// Admin — Tenant API Key handlers
// ============================================================================

func (s *Console) handleAdminListTenantAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	keys, err := s.apiKeySvc.ListForTenant(r.Context(), tenantID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeysFromDomain(keys))
}

func (s *Console) handleAdminCreateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	var req createTenantAPIKeyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "name is required")
		return
	}
	if message := validateOptionalCreditAmount("quota_limit_credits", req.QuotaLimitCredits); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	created, err := s.apiKeySvc.Create(r.Context(), apikeysvc.CreateInput{
		OwnerType:         domain.OwnerTenant,
		TenantID:          tenantID,
		Name:              req.Name,
		QuotaLimitCredits: req.QuotaLimitCredits,
		AllowedModels:     req.AllowedModels,
		Status:            req.Status,
		ExpiresAt:         adminTimestampToPtr(req.ExpiresAt),
		CreatedBy:         req.CreatedBy,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, createAPIKeyResponse{PlaintextKey: created.PlaintextKey, Key: apiKeyFromDomain(created.Key)})
}

func (s *Console) handleAdminUpdateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	if _, ok := parseUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	var req createTenantAPIKeyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "name is required")
		return
	}
	if message := validateOptionalCreditAmount("quota_limit_credits", req.QuotaLimitCredits); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	key, err := s.apiKeySvc.Update(r.Context(), apikeysvc.UpdateInput{
		ID:                chi.URLParam(r, "apiKeyID"),
		TenantID:          tenantID,
		Name:              req.Name,
		QuotaLimitCredits: req.QuotaLimitCredits,
		AllowedModels:     req.AllowedModels,
		Status:            req.Status,
		ExpiresAt:         adminTimestampToPtr(req.ExpiresAt),
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeyFromDomain(key))
}

func (s *Console) handleAdminUpdateTenantAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	if _, ok := parseUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}
	key, err := s.apiKeySvc.UpdateStatus(r.Context(), chi.URLParam(r, "apiKeyID"), tenantID, status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeyFromDomain(key))
}

func (s *Console) handleAdminDeleteTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	if _, ok := parseUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	if err := s.apiKeySvc.Delete(r.Context(), chi.URLParam(r, "apiKeyID"), tenantID); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

func (s *Console) handleAdminRotateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	if _, ok := parseUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	created, err := s.apiKeySvc.Rotate(r.Context(), chi.URLParam(r, "apiKeyID"), tenantID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, rotateAPIKeyResponse{PlaintextKey: created.PlaintextKey, Key: apiKeyFromDomain(created.Key)})
}

// ============================================================================
// Admin — User API Key handlers
// ============================================================================

func (s *Console) handleAdminListUserAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	keys, err := s.apiKeySvc.ListForUser(r.Context(), tenantID, userID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeysFromDomain(keys))
}

func (s *Console) handleAdminCreateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	var req createTenantAPIKeyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "name is required")
		return
	}
	if message := validateOptionalCreditAmount("quota_limit_credits", req.QuotaLimitCredits); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	created, err := s.apiKeySvc.Create(r.Context(), apikeysvc.CreateInput{
		OwnerType:         domain.OwnerUser,
		TenantID:          tenantID,
		UserID:            userID,
		Name:              req.Name,
		QuotaLimitCredits: req.QuotaLimitCredits,
		AllowedModels:     req.AllowedModels,
		Status:            req.Status,
		ExpiresAt:         adminTimestampToPtr(req.ExpiresAt),
		CreatedBy:         req.CreatedBy,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, createAPIKeyResponse{PlaintextKey: created.PlaintextKey, Key: apiKeyFromDomain(created.Key)})
}

func (s *Console) handleAdminUpdateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	var req createTenantAPIKeyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "name is required")
		return
	}
	if message := validateOptionalCreditAmount("quota_limit_credits", req.QuotaLimitCredits); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	key, err := s.apiKeySvc.Update(r.Context(), apikeysvc.UpdateInput{
		ID:                chi.URLParam(r, "apiKeyID"),
		TenantID:          tenantID,
		Name:              req.Name,
		QuotaLimitCredits: req.QuotaLimitCredits,
		AllowedModels:     req.AllowedModels,
		Status:            req.Status,
		ExpiresAt:         adminTimestampToPtr(req.ExpiresAt),
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeyFromDomain(key))
}

func (s *Console) handleAdminUpdateUserAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}
	key, err := s.apiKeySvc.UpdateStatus(r.Context(), chi.URLParam(r, "apiKeyID"), tenantID, status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeyFromDomain(key))
}

func (s *Console) handleAdminDeleteUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	if err := s.apiKeySvc.Delete(r.Context(), chi.URLParam(r, "apiKeyID"), tenantID); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

func (s *Console) handleAdminRotateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	created, err := s.apiKeySvc.Rotate(r.Context(), chi.URLParam(r, "apiKeyID"), tenantID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, rotateAPIKeyResponse{PlaintextKey: created.PlaintextKey, Key: apiKeyFromDomain(created.Key)})
}

// ============================================================================
// request → service helpers + domain.APIKey → wire DTO
// ============================================================================

func adminTimestampToPtr(t *adminTimestamp) *time.Time {
	if t == nil || !t.Valid {
		return nil
	}
	tt := t.Time
	return &tt
}

// microPtrToWholeCreditsPtr converts an optional micro-credit quota into the
// whole-credit wire value (nil = unlimited).
func microPtrToWholeCreditsPtr(micro *int64) *int64 {
	if micro == nil {
		return nil
	}
	v := credits.MicroToWholeCredits(*micro)
	return &v
}

func allowedModelsRaw(ms []string) json.RawMessage {
	b, _ := json.Marshal(ms)
	return rawJSON(b)
}

func apiKeyFromDomain(k domain.APIKey) apiKeyDTO {
	return apiKeyDTO{
		ID:                   pgUUIDFromString(k.ID),
		OwnerType:            string(k.OwnerType),
		TenantID:             k.TenantID,
		UserID:               optionalTextValue(k.UserID),
		LastFour:             optionalTextValue(k.LastFour),
		Name:                 k.Name,
		QuotaLimitCredits:    microPtrToWholeCreditsPtr(k.QuotaLimitMicro),
		QuotaUsedCredits:     credits.MicroToCredits(k.QuotaUsedMicro),
		QuotaReservedCredits: credits.MicroToCredits(k.QuotaReservedMicro),
		AllowedModels:        allowedModelsRaw(k.AllowedModels),
		Status:               k.Status,
		ExpiresAt:            timePtrToMillis(k.ExpiresAt),
		CreatedBy:            optionalTextValue(k.CreatedBy),
		CreatedAt:            timeToMillisPtr(k.CreatedAt),
		UpdatedAt:            timeToMillisPtr(k.UpdatedAt),
	}
}

func apiKeysFromDomain(keys []domain.APIKey) []apiKeyDTO {
	out := make([]apiKeyDTO, len(keys))
	for i, k := range keys {
		out[i] = apiKeyFromDomain(k)
	}
	return out
}

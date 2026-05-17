package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"uni-ai-api/backend/internal/apikey"
	dbgen "uni-ai-api/backend/internal/db/gen"
)

// ============================================================================
// Tenant API Key Handlers
// ============================================================================

func (s *Server) handleAdminListTenantAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}

	rows, err := s.queries.ListTenantAPIKeys(r.Context(), tenantID)
	if err != nil {
		s.writeAdminServerError(w, r, "list tenant api keys failed", err)
		return
	}
	writeOK(w, fromListTenantAPIKeys(rows))
}

func (s *Server) handleAdminCreateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
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
	if req.Status == "" {
		req.Status = defaultStatus
	}

	key, err := apikey.Generate()
	if err != nil {
		s.writeAdminServerError(w, r, "generate api key failed", err)
		return
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid allowed_models")
		return
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid expires_at")
		return
	}

	row, err := s.queries.CreateTenantAPIKey(r.Context(), dbgen.CreateTenantAPIKeyParams{
		TenantID:      tenantID,
		KeyHash:       apikey.Hash(key),
		KeyPrefix:     apikey.PrefixForDisplay(key),
		Name:          req.Name,
		QuotaLimit:    optionalInt8(req.QuotaLimit),
		AllowedModels: allowedModels,
		Status:        req.Status,
		ExpiresAt:     expiresAt,
		CreatedBy:     optionalTextValue(req.CreatedBy),
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, createTenantAPIKeyResponse{
		APIKey: key,
		Key:    fromCreateTenantAPIKey(row),
	})
}

func (s *Server) handleAdminUpdateTenantAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateTenantAPIKeyStatus(r.Context(), dbgen.UpdateTenantAPIKeyStatusParams{
		TenantID: tenantID,
		ID:       apiKeyID,
		Status:   status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpdateTenantAPIKeyStatus(row))
}

func (s *Server) handleAdminUpdateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
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
	if req.Status == "" {
		req.Status = defaultStatus
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid allowed_models")
		return
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid expires_at")
		return
	}
	row, err := s.queries.UpdateTenantAPIKey(r.Context(), dbgen.UpdateTenantAPIKeyParams{
		TenantID: tenantID, ID: apiKeyID, Name: req.Name, QuotaLimit: optionalInt8(req.QuotaLimit), AllowedModels: allowedModels,
		Status: req.Status, ExpiresAt: expiresAt,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpdateTenantAPIKey(row))
}

func (s *Server) handleAdminListUserAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}

	rows, err := s.queries.ListUserAPIKeys(r.Context(), dbgen.ListUserAPIKeysParams{
		TenantID: tenantID,
		UserID:   optionalTextValue(userID),
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list user api keys failed", err)
		return
	}
	writeOK(w, fromListUserAPIKeys(rows))
}

func (s *Server) handleAdminCreateUserAPIKey(w http.ResponseWriter, r *http.Request) {
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
	if req.Status == "" {
		req.Status = defaultStatus
	}

	key, err := apikey.Generate()
	if err != nil {
		s.writeAdminServerError(w, r, "generate api key failed", err)
		return
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid allowed_models")
		return
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid expires_at")
		return
	}

	row, err := s.queries.CreateUserAPIKey(r.Context(), dbgen.CreateUserAPIKeyParams{
		TenantID:      tenantID,
		UserID:        optionalTextValue(userID),
		KeyHash:       apikey.Hash(key),
		KeyPrefix:     apikey.PrefixForDisplay(key),
		Name:          req.Name,
		QuotaLimit:    optionalInt8(req.QuotaLimit),
		AllowedModels: allowedModels,
		Status:        req.Status,
		ExpiresAt:     expiresAt,
		CreatedBy:     optionalTextValue(req.CreatedBy),
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, createUserAPIKeyResponse{
		APIKey: key,
		Key:    fromCreateUserAPIKey(row),
	})
}

func (s *Server) handleAdminUpdateUserAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateUserAPIKeyStatus(r.Context(), dbgen.UpdateUserAPIKeyStatusParams{
		TenantID: tenantID,
		UserID:   optionalTextValue(userID),
		ID:       apiKeyID,
		Status:   status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpdateUserAPIKeyStatus(row))
}

func (s *Server) handleAdminUpdateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
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
	if req.Status == "" {
		req.Status = defaultStatus
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid allowed_models")
		return
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid expires_at")
		return
	}
	row, err := s.queries.UpdateUserAPIKey(r.Context(), dbgen.UpdateUserAPIKeyParams{
		TenantID: tenantID, UserID: optionalTextValue(userID), ID: apiKeyID, Name: req.Name, QuotaLimit: optionalInt8(req.QuotaLimit),
		AllowedModels: allowedModels, Status: req.Status, ExpiresAt: expiresAt,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpdateUserAPIKey(row))
}

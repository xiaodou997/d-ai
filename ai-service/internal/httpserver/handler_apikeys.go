package httpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/uni-ai-api/internal/apikey"
	dbgen "xiaodou/uni-ai-api/internal/db/gen"
)

// ============================================================================
// Admin — Tenant API Key handlers
// ============================================================================

func (s *Server) handleAdminListTenantAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	rows, err := s.queries.ListAPIKeys(r.Context(), dbgen.ListAPIKeysParams{
		TenantID:  tenantID,
		OwnerType: pgtype.Text{String: "tenant", Valid: true},
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list tenant api keys failed", err)
		return
	}
	writeOK(w, apiKeyDTOsFromList(rows))
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
	row, err := s.queries.CreateAPIKey(r.Context(), dbgen.CreateAPIKeyParams{
		OwnerType:     "tenant",
		TenantID:      tenantID,
		KeyHash:       apikey.Hash(key),
		LastFour:      pgtype.Text{String: apikey.LastFour(key), Valid: true},
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
	writeOK(w, createAPIKeyResponse{PlaintextKey: key, Key: apiKeyDTOFromCreate(row)})
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
	row, err := s.queries.UpdateAPIKey(r.Context(), dbgen.UpdateAPIKeyParams{
		ID: apiKeyID, TenantID: tenantID, Name: req.Name,
		QuotaLimit: optionalInt8(req.QuotaLimit), AllowedModels: allowedModels,
		Status: req.Status, ExpiresAt: expiresAt,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), row.KeyHash)
	writeOK(w, apiKeyDTOFromUpdate(row))
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
	row, err := s.queries.UpdateAPIKeyStatus(r.Context(), dbgen.UpdateAPIKeyStatusParams{
		ID: apiKeyID, TenantID: tenantID, Status: status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), row.KeyHash)
	writeOK(w, apiKeyDTOFromUpdateStatus(row))
}

func (s *Server) handleAdminDeleteTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	keyHash, err := s.queries.DeleteAPIKey(r.Context(), dbgen.DeleteAPIKeyParams{
		ID: apiKeyID, TenantID: tenantID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), keyHash)
	writeOK(w, nil)
}

func (s *Server) handleAdminRotateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	existing, err := s.queries.GetAPIKeyByID(r.Context(), dbgen.GetAPIKeyByIDParams{
		ID: apiKeyID, TenantID: tenantID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	newKey, err := apikey.Generate()
	if err != nil {
		s.writeAdminServerError(w, r, "generate api key failed", err)
		return
	}
	row, err := s.queries.RotateAPIKey(r.Context(), dbgen.RotateAPIKeyParams{
		ID: apiKeyID, TenantID: tenantID,
		KeyHash:  apikey.Hash(newKey),
		LastFour: pgtype.Text{String: apikey.LastFour(newKey), Valid: true},
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), existing.KeyHash)
	writeOK(w, rotateAPIKeyResponse{PlaintextKey: newKey, Key: apiKeyDTOFromRotate(row)})
}

// ============================================================================
// Admin — User API Key handlers
// ============================================================================

func (s *Server) handleAdminListUserAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	rows, err := s.queries.ListAPIKeys(r.Context(), dbgen.ListAPIKeysParams{
		TenantID:  tenantID,
		OwnerType: pgtype.Text{String: "user", Valid: true},
		UserID:    pgtype.Text{String: userID, Valid: true},
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list user api keys failed", err)
		return
	}
	writeOK(w, apiKeyDTOsFromList(rows))
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
	row, err := s.queries.CreateAPIKey(r.Context(), dbgen.CreateAPIKeyParams{
		OwnerType:     "user",
		TenantID:      tenantID,
		UserID:        pgtype.Text{String: userID, Valid: true},
		KeyHash:       apikey.Hash(key),
		LastFour:      pgtype.Text{String: apikey.LastFour(key), Valid: true},
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
	writeOK(w, createAPIKeyResponse{PlaintextKey: key, Key: apiKeyDTOFromCreate(row)})
}

func (s *Server) handleAdminUpdateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := tenantUserParams(w, r)
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
	row, err := s.queries.UpdateAPIKey(r.Context(), dbgen.UpdateAPIKeyParams{
		ID: apiKeyID, TenantID: tenantID, Name: req.Name,
		QuotaLimit: optionalInt8(req.QuotaLimit), AllowedModels: allowedModels,
		Status: req.Status, ExpiresAt: expiresAt,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), row.KeyHash)
	writeOK(w, apiKeyDTOFromUpdate(row))
}

func (s *Server) handleAdminUpdateUserAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := tenantUserParams(w, r)
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
	row, err := s.queries.UpdateAPIKeyStatus(r.Context(), dbgen.UpdateAPIKeyStatusParams{
		ID: apiKeyID, TenantID: tenantID, Status: status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), row.KeyHash)
	writeOK(w, apiKeyDTOFromUpdateStatus(row))
}

func (s *Server) handleAdminDeleteUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	keyHash, err := s.queries.DeleteAPIKey(r.Context(), dbgen.DeleteAPIKeyParams{
		ID: apiKeyID, TenantID: tenantID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), keyHash)
	writeOK(w, nil)
}

func (s *Server) handleAdminRotateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	existing, err := s.queries.GetAPIKeyByID(r.Context(), dbgen.GetAPIKeyByIDParams{
		ID: apiKeyID, TenantID: tenantID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	newKey, err := apikey.Generate()
	if err != nil {
		s.writeAdminServerError(w, r, "generate api key failed", err)
		return
	}
	row, err := s.queries.RotateAPIKey(r.Context(), dbgen.RotateAPIKeyParams{
		ID: apiKeyID, TenantID: tenantID,
		KeyHash:  apikey.Hash(newKey),
		LastFour: pgtype.Text{String: apikey.LastFour(newKey), Valid: true},
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), existing.KeyHash)
	writeOK(w, rotateAPIKeyResponse{PlaintextKey: newKey, Key: apiKeyDTOFromRotate(row)})
}

// ============================================================================
// Cache invalidation helper
// ============================================================================

func (s *Server) invalidateAPIKeyCache(ctx context.Context, keyHash string) {
	if s.apiKeyCache == nil {
		return
	}
	if err := s.apiKeyCache.Del(ctx, keyHash); err != nil {
		s.logger.Error("api key cache invalidation failed", "error", err, "key_hash_prefix", keyHash[:8])
	}
}

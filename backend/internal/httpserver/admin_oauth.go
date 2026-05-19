package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	pgadapter "uni-ai-api/backend/internal/adapters/postgres"
	"uni-ai-api/backend/internal/domain"
)

// ============================================================================
// Pool request / response types
// ============================================================================

type createCredentialPoolRequest struct {
	Name              string `json:"name"`
	FixedProviderType string `json:"fixed_provider_type"` // codex|claude_oauth|gemini_cli|antigravity
	OAuthStrategy     string `json:"oauth_strategy"`      // round_robin|weighted; default: round_robin
	Notes             string `json:"notes"`
	Status            string `json:"status"` // ignored on create; always "active"
}

type patchCredentialPoolRequest struct {
	Name          *string `json:"name"`
	OAuthStrategy *string `json:"oauth_strategy"`
	Notes         *string `json:"notes"`
	Status        *string `json:"status"` // active|disabled
}

type credentialPoolResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	FixedProviderType string `json:"fixed_provider_type"`
	OAuthStrategy     string `json:"oauth_strategy"`
	Notes             string `json:"notes,omitempty"`
	Status            string `json:"status"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

// ============================================================================
// Credential request / response types
// ============================================================================

type importPoolCredentialRequest struct {
	Name         string         `json:"name"`
	ProviderType string         `json:"provider_type"`
	Email        string         `json:"email"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	TokenType    string         `json:"token_type"`
	Scope        string         `json:"scope"`
	ExpiresAt    *int64         `json:"expires_at"`    // Unix seconds
	Weight       *int           `json:"weight"`
	AuthMetadata map[string]any `json:"auth_metadata"`

	// Codex-specific top-level export fields; merged into AuthMetadata if AuthMetadata is empty.
	AccountID     string `json:"account_id"`
	PlanType      string `json:"plan_type"`
	UserID        string `json:"user_id"`
	AccountUserID string `json:"account_user_id"`
}

type patchPoolCredentialRequest struct {
	Status *string `json:"status"` // active|disabled
	Weight *int    `json:"weight"`
}

type poolCredentialResponse struct {
	ID                   string         `json:"id"`
	PoolID               string         `json:"pool_id"`
	Name                 string         `json:"name"`
	ProviderType         string         `json:"provider_type"`
	Email                string         `json:"email"`
	TokenType            string         `json:"token_type"`
	Scope                string         `json:"scope"`
	ExpiresAt            *int64         `json:"expires_at,omitempty"`
	AuthMetadata         map[string]any `json:"auth_metadata"`
	Weight               int            `json:"weight"`
	Status               string         `json:"status"`
	InvalidReason        string         `json:"invalid_reason,omitempty"`
	LastUsedAt           *int64         `json:"last_used_at,omitempty"`
	LastRefreshedAt      *int64         `json:"last_refreshed_at,omitempty"`
	LastFailedAt         *int64         `json:"last_failed_at,omitempty"`
	ConsecutiveFailCount int            `json:"consecutive_fail_count"`
	SuccessCount         int64          `json:"success_count"`
	FailCount            int64          `json:"fail_count"`
	CreatedAt            int64          `json:"created_at"`
	UpdatedAt            int64          `json:"updated_at"`
}

// ============================================================================
// Pool CRUD
// ============================================================================

// handleAdminListCredentialPools returns all credential pools.
//
//	GET /api/v1/credential-pools
func (s *Server) handleAdminListCredentialPools(w http.ResponseWriter, r *http.Request) {
	pools, err := s.oauthCreds.ListPools(r.Context())
	if err != nil {
		s.writeAdminServerError(w, r, "list credential pools failed", err)
		return
	}
	out := make([]credentialPoolResponse, 0, len(pools))
	for _, p := range pools {
		out = append(out, poolToResponse(p))
	}
	writeOK(w, out)
}

// handleAdminCreateCredentialPool creates a new credential pool.
//
//	POST /api/v1/credential-pools
func (s *Server) handleAdminCreateCredentialPool(w http.ResponseWriter, r *http.Request) {
	var req createCredentialPoolRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "name is required")
		return
	}
	if req.FixedProviderType == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "fixed_provider_type is required")
		return
	}
	switch req.FixedProviderType {
	case string(domain.FixedProviderCodex),
		string(domain.FixedProviderClaudeOAuth),
		string(domain.FixedProviderGeminiCLI),
		string(domain.FixedProviderAntigravity):
	default:
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "fixed_provider_type must be one of: codex, claude_oauth, gemini_cli, antigravity")
		return
	}

	id, err := s.oauthCreds.CreatePool(r.Context(), pgadapter.CredentialPoolInput{
		Name:              req.Name,
		FixedProviderType: req.FixedProviderType,
		OAuthStrategy:     req.OAuthStrategy,
		Notes:             req.Notes,
		Status:            "active",
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	p, err := s.oauthCreds.GetPool(r.Context(), id)
	if err != nil {
		s.logger.Warn("pool created but read-back failed", "error", err, "pool_id", id)
		writeOK(w, map[string]string{"id": id})
		return
	}
	writeOK(w, poolToResponse(*p))
}

// handleAdminPatchCredentialPool updates a pool's mutable fields.
//
//	PATCH /api/v1/credential-pools/:poolID
func (s *Server) handleAdminPatchCredentialPool(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "poolID")
	if poolID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "poolID is required")
		return
	}
	p, err := s.oauthCreds.GetPool(r.Context(), poolID)
	if err != nil {
		writeDBErr(w, err)
		return
	}

	var req patchCredentialPoolRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}

	in := pgadapter.CredentialPoolInput{
		Name:          p.Name,
		OAuthStrategy: p.OAuthStrategy,
		Notes:         p.Notes,
		Status:        p.Status,
	}
	if req.Name != nil {
		in.Name = *req.Name
	}
	if req.OAuthStrategy != nil {
		switch *req.OAuthStrategy {
		case "round_robin", "weighted":
		default:
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "oauth_strategy must be round_robin or weighted")
			return
		}
		in.OAuthStrategy = *req.OAuthStrategy
	}
	if req.Notes != nil {
		in.Notes = *req.Notes
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "disabled":
		default:
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "status must be active or disabled")
			return
		}
		in.Status = *req.Status
	}

	if err := s.oauthCreds.UpdatePool(r.Context(), poolID, in); err != nil {
		writeDBErr(w, err)
		return
	}
	updated, err := s.oauthCreds.GetPool(r.Context(), poolID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, poolToResponse(*updated))
}

// handleAdminDeleteCredentialPool removes a pool and all its credentials.
//
//	DELETE /api/v1/credential-pools/:poolID
func (s *Server) handleAdminDeleteCredentialPool(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "poolID")
	if poolID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "poolID is required")
		return
	}
	if err := s.oauthCreds.DeletePool(r.Context(), poolID); err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, nil)
}

// ============================================================================
// Credentials management within a pool
// ============================================================================

// handleAdminListPoolCredentials lists all credentials in a pool.
//
//	GET /api/v1/credential-pools/:poolID/credentials
func (s *Server) handleAdminListPoolCredentials(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "poolID")
	if poolID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "poolID is required")
		return
	}
	rows, err := s.oauthCreds.ListForPool(r.Context(), poolID)
	if err != nil {
		s.writeAdminServerError(w, r, "list pool credentials failed", err)
		return
	}
	out := make([]poolCredentialResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, credRowToResponse(row))
	}
	writeOK(w, out)
}

// handleAdminCreatePoolCredential imports a credential into a pool.
//
//	POST /api/v1/credential-pools/:poolID/credentials
func (s *Server) handleAdminCreatePoolCredential(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "poolID")
	if poolID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "poolID is required")
		return
	}

	var req importPoolCredentialRequest
	if !decodeAdminJSONLenient(w, r, &req) {
		return
	}
	if req.AccessToken == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "access_token is required")
		return
	}

	// Merge top-level Codex export fields into AuthMetadata.
	meta := req.AuthMetadata
	if meta == nil {
		meta = map[string]any{}
	}
	if req.AccountID != "" && meta["account_id"] == nil {
		meta["account_id"] = req.AccountID
	}
	if req.PlanType != "" && meta["plan_type"] == nil {
		meta["plan_type"] = req.PlanType
	}
	if req.UserID != "" && meta["user_id"] == nil {
		meta["user_id"] = req.UserID
	}
	if req.AccountUserID != "" && meta["account_user_id"] == nil {
		meta["account_user_id"] = req.AccountUserID
	}

	name := req.Name
	if name == "" {
		name = req.Email
	}
	if name == "" {
		name = req.ProviderType
	}

	weight := 100
	if req.Weight != nil && *req.Weight > 0 {
		weight = *req.Weight
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0).UTC()
		expiresAt = &t
	}

	// Derive provider_type from pool if not supplied.
	providerType := req.ProviderType
	if providerType == "" {
		p, err := s.oauthCreds.GetPool(r.Context(), poolID)
		if err != nil {
			writeDBErr(w, err)
			return
		}
		providerType = string(p.FixedProviderType)
	}

	id, err := s.oauthCreds.Create(r.Context(), poolID, pgadapter.OAuthCredentialInput{
		Name:         name,
		ProviderType: providerType,
		Email:        req.Email,
		AccessToken:  req.AccessToken,
		RefreshToken: req.RefreshToken,
		TokenType:    req.TokenType,
		Scope:        req.Scope,
		ExpiresAt:    expiresAt,
		AuthMetadata: meta,
		Weight:       weight,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	row, err := s.oauthCreds.GetByID(r.Context(), id)
	if err != nil {
		s.logger.Warn("credential created but read-back failed", "error", err, "credential_id", id)
		writeOK(w, map[string]string{"id": id})
		return
	}
	writeOK(w, credRowToResponse(*row))
}

// handleAdminPatchPoolCredential updates status or weight of a credential.
//
//	PATCH /api/v1/credential-pools/:poolID/credentials/:credID
func (s *Server) handleAdminPatchPoolCredential(w http.ResponseWriter, r *http.Request) {
	credID := chi.URLParam(r, "credID")
	if credID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "credID is required")
		return
	}

	var req patchPoolCredentialRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}

	if req.Status != nil {
		switch *req.Status {
		case "active", "disabled":
		default:
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "status must be 'active' or 'disabled'")
			return
		}
		if err := s.oauthCreds.UpdateStatus(r.Context(), credID, *req.Status); err != nil {
			writeDBErr(w, err)
			return
		}
	}
	if req.Weight != nil {
		if *req.Weight < 0 {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "weight must be >= 0")
			return
		}
		if err := s.oauthCreds.UpdateWeight(r.Context(), credID, *req.Weight); err != nil {
			writeDBErr(w, err)
			return
		}
	}

	row, err := s.oauthCreds.GetByID(r.Context(), credID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, credRowToResponse(*row))
}

// handleAdminDeletePoolCredential permanently removes a credential.
//
//	DELETE /api/v1/credential-pools/:poolID/credentials/:credID
func (s *Server) handleAdminDeletePoolCredential(w http.ResponseWriter, r *http.Request) {
	credID := chi.URLParam(r, "credID")
	if credID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "credID is required")
		return
	}
	if err := s.oauthCreds.Delete(r.Context(), credID); err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, nil)
}

// handleAdminRefreshPoolCredential triggers an immediate token refresh.
//
//	POST /api/v1/credential-pools/:poolID/credentials/:credID/refresh
func (s *Server) handleAdminRefreshPoolCredential(w http.ResponseWriter, r *http.Request) {
	credID := chi.URLParam(r, "credID")
	if credID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "credID is required")
		return
	}
	if err := s.tokenRefresher.RefreshByID(r.Context(), credID); err != nil {
		writeErr(w, http.StatusBadGateway, BizErrInternal, "token refresh failed: "+err.Error())
		return
	}
	row, err := s.oauthCreds.GetByID(r.Context(), credID)
	if err != nil {
		s.logger.Warn("token refreshed but read-back failed", "error", err, "credential_id", credID)
		writeOK(w, map[string]string{"status": "ok"})
		return
	}
	writeOK(w, credRowToResponse(*row))
}

// ============================================================================
// Available models for a pool
// ============================================================================

// handleAdminGetPoolAvailableModels returns the list of upstream models for a pool.
// It first tries to fetch the list dynamically (for API-key providers), then falls
// back to the hard-coded preset for each fixed provider type.
//
//	GET /api/v1/credential-pools/:poolID/available-models
func (s *Server) handleAdminGetPoolAvailableModels(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "poolID")
	if poolID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "poolID is required")
		return
	}
	p, err := s.oauthCreds.GetPool(r.Context(), poolID)
	if err != nil {
		writeDBErr(w, err)
		return
	}

	models := poolPresetModels(p.FixedProviderType)
	writeOK(w, map[string]any{
		"pool_id":             p.ID,
		"fixed_provider_type": string(p.FixedProviderType),
		"models":              models,
		"source":              "preset",
	})
}

// poolPresetModels returns the hardcoded upstream model list for a fixed provider.
func poolPresetModels(pt domain.FixedProviderType) []string {
	switch pt {
	case domain.FixedProviderCodex:
		return []string{"gpt-4o", "gpt-4o-mini", "o1", "o1-mini", "o3", "o3-mini", "o4-mini"}
	case domain.FixedProviderClaudeOAuth:
		return []string{"claude-opus-4-5", "claude-sonnet-4-5", "claude-haiku-4-5"}
	case domain.FixedProviderGeminiCLI, domain.FixedProviderAntigravity:
		return []string{"gemini-2.0-flash", "gemini-2.5-pro", "gemini-2.5-flash"}
	default:
		return []string{}
	}
}

// ============================================================================
// Pool health summary
// ============================================================================

// handleAdminGetOAuthPoolHealth returns per-pool OAuth credential health aggregates.
//
//	GET /api/v1/oauth-pool-health
func (s *Server) handleAdminGetOAuthPoolHealth(w http.ResponseWriter, r *http.Request) {
	rows, err := s.oauthCreds.GetPoolHealthSummary(r.Context())
	if err != nil {
		s.writeAdminServerError(w, r, "get oauth pool health failed", err)
		return
	}

	type healthItem struct {
		PoolID            string `json:"pool_id"`
		PoolName          string `json:"pool_name"`
		FixedProviderType string `json:"fixed_provider_type"`
		OAuthStrategy     string `json:"oauth_strategy"`
		Total             int    `json:"total"`
		Active            int    `json:"active"`
		Invalid           int    `json:"invalid"`
		Disabled          int    `json:"disabled"`
		ExpiringSoon      int    `json:"expiring_soon"`
	}

	out := make([]healthItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, healthItem{
			PoolID:            row.PoolID,
			PoolName:          row.PoolName,
			FixedProviderType: row.FixedProviderType,
			OAuthStrategy:     row.OAuthStrategy,
			Total:             row.Total,
			Active:            row.Active,
			Invalid:           row.Invalid,
			Disabled:          row.Disabled,
			ExpiringSoon:      row.ExpiringSoon,
		})
	}
	writeOK(w, out)
}

// ============================================================================
// Helpers
// ============================================================================

func poolToResponse(p domain.CredentialPool) credentialPoolResponse {
	return credentialPoolResponse{
		ID:                p.ID,
		Name:              p.Name,
		FixedProviderType: string(p.FixedProviderType),
		OAuthStrategy:     p.OAuthStrategy,
		Notes:             p.Notes,
		Status:            p.Status,
		CreatedAt:         p.CreatedAt.UnixMilli(),
		UpdatedAt:         p.UpdatedAt.UnixMilli(),
	}
}

func credRowToResponse(row pgadapter.OAuthCredentialRow) poolCredentialResponse {
	resp := poolCredentialResponse{
		ID:                   row.ID,
		PoolID:               row.PoolID,
		Name:                 row.Name,
		ProviderType:         row.ProviderType,
		Email:                row.Email,
		TokenType:            row.TokenType,
		Scope:                row.Scope,
		Weight:               row.Weight,
		Status:               row.Status,
		InvalidReason:        row.InvalidReason,
		ConsecutiveFailCount: row.ConsecutiveFailCount,
		SuccessCount:         row.SuccessCount,
		FailCount:            row.FailCount,
		CreatedAt:            row.CreatedAt.UnixMilli(),
		UpdatedAt:            row.UpdatedAt.UnixMilli(),
	}
	if len(row.AuthMetadataRaw) > 0 {
		m := make(map[string]any)
		_ = json.Unmarshal(row.AuthMetadataRaw, &m)
		resp.AuthMetadata = m
	}
	if row.ExpiresAt != nil {
		ms := row.ExpiresAt.UnixMilli()
		resp.ExpiresAt = &ms
	}
	if row.LastUsedAt != nil {
		ms := row.LastUsedAt.UnixMilli()
		resp.LastUsedAt = &ms
	}
	if row.LastRefreshedAt != nil {
		ms := row.LastRefreshedAt.UnixMilli()
		resp.LastRefreshedAt = &ms
	}
	if row.LastFailedAt != nil {
		ms := row.LastFailedAt.UnixMilli()
		resp.LastFailedAt = &ms
	}
	return resp
}

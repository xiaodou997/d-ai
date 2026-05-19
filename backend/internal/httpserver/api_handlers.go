package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"uni-ai-api/backend/internal/apikey"
	dbgen "uni-ai-api/backend/internal/db/gen"
)

// ============================================================================
// 租户自己的资源（根据 token 中的 tenantID 自动过滤）
// ============================================================================

// handleTenantAPIKeysSelf - 租户查看自己的 API Key 列表
func (s *Server) handleTenantAPIKeysSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	var tenantID string
	if ac.Role == apiRolePlatform {
		tenantID = r.URL.Query().Get("tenant_id")
	} else if ac.Role == apiRoleTenant {
		tenantID = ac.TenantID
	} else {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return
	}

	rows, err := s.queries.ListAPIKeys(r.Context(), dbgen.ListAPIKeysParams{
		TenantID:  tenantID,
		OwnerType: pgtype.Text{String: "tenant", Valid: true},
	})
	if err != nil {
		s.logger.Error("list tenant api keys failed", "error", err)
		writeDBErr(w, err)
		return
	}
	writeOK(w, apiKeyDTOsFromList(rows))
}

// handleTenantModelGrantsSelf - 租户查看自己的模型授权
func (s *Server) handleTenantModelGrantsSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	var tenantID string
	if ac.Role == apiRolePlatform {
		tenantID = r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenant_id required for platform")
			return
		}
	} else if ac.Role == apiRoleTenant {
		tenantID = ac.TenantID
	} else {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return
	}

	rows, err := s.queries.ListTenantModelGrants(r.Context(), tenantID)
	if err != nil {
		s.logger.Error("list tenant model grants failed", "error", err)
		writeDBErr(w, err)
		return
	}

	writeOK(w, fromListTenantModelGrants(rows))
}

// ============================================================================
// 用户自己的资源（根据 token 中的 userID 自动过滤）
// ============================================================================

// handleUserAPIKeysSelf - 用户查看自己的 API Key 列表
func (s *Server) handleUserAPIKeysSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	var params dbgen.ListAPIKeysParams
	params.OwnerType = pgtype.Text{String: "user", Valid: true}

	switch ac.Role {
	case apiRoleUser:
		params.TenantID = ac.TenantID
		params.UserID = pgtype.Text{String: ac.UserID, Valid: true}
	case apiRoleTenant:
		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "user_id required for tenant")
			return
		}
		params.TenantID = ac.TenantID
		params.UserID = pgtype.Text{String: userID, Valid: true}
	case apiRolePlatform:
		tenantID := r.URL.Query().Get("tenant_id")
		userID := r.URL.Query().Get("user_id")
		if tenantID == "" || userID == "" {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenant_id and user_id required for platform")
			return
		}
		params.TenantID = tenantID
		params.UserID = pgtype.Text{String: userID, Valid: true}
	default:
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return
	}

	rows, err := s.queries.ListAPIKeys(r.Context(), params)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, apiKeyDTOsFromList(rows))
}

// handleUserModelGrantsSelf - 用户查看自己可用的模型授权
func (s *Server) handleUserModelGrantsSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only for end users")
		return
	}

	rows, err := s.queries.ListUserAvailableModels(r.Context(), ac.TenantID)
	if err != nil {
		s.logger.Error("list user available models failed", "error", err)
		writeDBErr(w, err)
		return
	}

	writeOK(w, fromListUserAvailableModels(rows))
}

// ============================================================================
// Dashboard（根据角色返回不同维度数据）
// ============================================================================

// handleDashboardSummaryByRole - Dashboard 概览，根据角色返回不同数据
func (s *Server) handleDashboardSummaryByRole(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	params := dbgen.GetDashboardSummaryParams{}
	since, ok := parseDashboardSince(w, r)
	if !ok {
		return
	}
	params.Since = since
	if ac.Role == apiRoleTenant {
		params.TenantID = pgtype.Text{String: ac.TenantID, Valid: true}
	} else if ac.Role == apiRoleUser {
		params.TenantID = pgtype.Text{String: ac.TenantID, Valid: true}
		params.UserID = pgtype.Text{String: ac.UserID, Valid: true}
	}

	summary, err := s.queries.GetDashboardSummary(r.Context(), params)
	if err != nil {
		s.logger.Error("dashboard summary failed", "error", err)
		writeDBErr(w, err)
		return
	}

	writeOK(w, summary)
}

// ============================================================================
// Usage Logs（根据角色返回不同范围数据）
// ============================================================================

// handleUsageLogsByRole - 使用日志，根据角色返回不同范围
func (s *Server) handleUsageLogsByRole(w http.ResponseWriter, r *http.Request) {
	s.handleAdminListUsageLogs(w, r)
}

// ============================================================================
// 租户售价（仅租户可管理）
// ============================================================================

// handleUserPricesSelf - 租户管理自己的售价
func (s *Server) handleUserPricesSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can manage user prices")
		return
	}

	rows, err := s.queries.ListTenantUserPrices(r.Context(), ac.TenantID)
	if err != nil {
		s.logger.Error("list tenant user prices failed", "error", err)
		writeDBErr(w, err)
		return
	}

	writeOK(w, fromListTenantUserPrices(rows))
}

// ============================================================================
// 用户使用记录（仅用户可查看）
// ============================================================================

// handleUserUsageLogsSelf - 用户查看自己的使用记录
func (s *Server) handleUserUsageLogsSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only for end users")
		return
	}

	limit := int32(100)
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := parseInt32(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	rows, err := s.queries.ListUsageLogsByTenantUser(r.Context(), dbgen.ListUsageLogsByTenantUserParams{
		TenantID: ac.TenantID,
		UserID:   pgtype.Text{String: ac.UserID, Valid: true},
		Limit:    limit,
	})
	if err != nil {
		s.logger.Error("list user usage logs failed", "error", err)
		writeDBErr(w, err)
		return
	}

	writeOK(w, fromListUsageLogsByUser(rows))
}

// handleUserUsageSummarySelf - 用户查看自己的使用汇总
func (s *Server) handleUserUsageSummarySelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only for end users")
		return
	}

	summary, err := s.queries.ListUsageSummaryByTenantUser(r.Context(), dbgen.ListUsageSummaryByTenantUserParams{
		TenantID: ac.TenantID,
		UserID:   ac.UserID,
	})
	if err != nil {
		s.logger.Error("list user usage summary failed", "error", err)
		writeDBErr(w, err)
		return
	}

	writeOK(w, summary)
}

// ============================================================================
// /tenants/me/* routes - 租户自管理路由
// ============================================================================

// handleTenantsMeAPIKeysCreate - 租户创建自己的 API Key
func (s *Server) handleTenantsMeAPIKeysCreate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can create own api keys")
		return
	}
	var req createAPIKeyRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	key, err := apikey.Generate()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "key generation failed")
		return
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid allowed_models")
		return
	}
	row, err := s.queries.CreateAPIKey(r.Context(), dbgen.CreateAPIKeyParams{
		OwnerType:     "tenant",
		TenantID:      ac.TenantID,
		KeyHash:       apikey.Hash(key),
		LastFour:      pgtype.Text{String: apikey.LastFour(key), Valid: true},
		Name:          req.Name,
		QuotaLimit:    pgtype.Int8{Int64: req.QuotaLimit, Valid: req.QuotaLimit > 0},
		AllowedModels: allowedModels,
		Status:        defaultStatus,
		CreatedBy:     pgtype.Text{String: ac.UserID, Valid: true},
	})
	if err != nil {
		s.logger.Error("create tenant api key failed", "error", err)
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "create failed")
		return
	}
	writeOK(w, createAPIKeyResponse{PlaintextKey: key, Key: apiKeyDTOFromCreate(row)})
}

// handleTenantsMeAPIKeysUpdate - 租户更新自己的 API Key
func (s *Server) handleTenantsMeAPIKeysUpdate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can update own api keys")
		return
	}
	apiKeyID, ok := parseAPIUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	var req updateAPIKeyRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid allowed_models")
		return
	}
	row, err := s.queries.UpdateAPIKey(r.Context(), dbgen.UpdateAPIKeyParams{
		TenantID:      ac.TenantID,
		ID:            apiKeyID,
		Name:          req.Name,
		QuotaLimit:    pgtype.Int8{Int64: req.QuotaLimit, Valid: req.QuotaLimit > 0},
		AllowedModels: allowedModels,
		Status:        defaultStatus,
	})
	if err != nil {
		s.logger.Error("update tenant api key failed", "error", err)
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "update failed")
		return
	}
	s.invalidateAPIKeyCache(r.Context(), row.KeyHash)
	writeOK(w, apiKeyDTOFromUpdate(row))
}

// handleTenantsMeAPIKeysStatus - 租户更新自己的 API Key 状态
func (s *Server) handleTenantsMeAPIKeysStatus(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can update own api keys")
		return
	}
	apiKeyID, ok := parseAPIUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	var req updateStatusRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	row, err := s.queries.UpdateAPIKeyStatus(r.Context(), dbgen.UpdateAPIKeyStatusParams{
		TenantID: ac.TenantID,
		ID:       apiKeyID,
		Status:   req.Status,
	})
	if err != nil {
		s.logger.Error("update tenant api key status failed", "error", err)
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "update failed")
		return
	}
	s.invalidateAPIKeyCache(r.Context(), row.KeyHash)
	writeOK(w, apiKeyDTOFromUpdateStatus(row))
}

// handleTenantsMeAPIKeysRotate - 租户 Rotate 自己的 API Key
func (s *Server) handleTenantsMeAPIKeysRotate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can rotate own api keys")
		return
	}
	apiKeyID, ok := parseAPIUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	existing, err := s.queries.GetAPIKeyByID(r.Context(), dbgen.GetAPIKeyByIDParams{
		ID: apiKeyID, TenantID: ac.TenantID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	newKey, err := apikey.Generate()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "key generation failed")
		return
	}
	row, err := s.queries.RotateAPIKey(r.Context(), dbgen.RotateAPIKeyParams{
		ID: apiKeyID, TenantID: ac.TenantID,
		KeyHash:  apikey.Hash(newKey),
		LastFour: pgtype.Text{String: apikey.LastFour(newKey), Valid: true},
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "rotate failed")
		return
	}
	s.invalidateAPIKeyCache(r.Context(), existing.KeyHash)
	writeOK(w, rotateAPIKeyResponse{PlaintextKey: newKey, Key: apiKeyDTOFromRotate(row)})
}

// ============================================================================
// /tenants/me/user-prices/* routes - 租户售价自管理
// ============================================================================

// handleTenantsMeUserPricesUpsert - 租户设置售价
func (s *Server) handleTenantsMeUserPricesUpsert(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can manage user prices")
		return
	}

	modelID, ok2 := parseAPIUUIDParam(w, r, "modelID")
	if !ok2 {
		return
	}

	var req upsertTenantUserPriceRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}

	imagePrices := req.ImagePrices
	if len(imagePrices) == 0 {
		imagePrices = json.RawMessage("[]")
	}
	videoPrices := req.VideoPrices
	if len(videoPrices) == 0 {
		videoPrices = json.RawMessage("[]")
	}
	row, err := s.queries.UpsertTenantUserPrice(r.Context(), dbgen.UpsertTenantUserPriceParams{
		TenantID:                ac.TenantID,
		ModelID:                 modelID,
		InputPricePer1m:         req.InputPricePer1m,
		OutputPricePer1m:        req.OutputPricePer1m,
		ImagePrices:             imagePrices,
		VideoPrices:             videoPrices,
		AudioTtsPricePer1mChars: req.AudioTtsPricePer1mChars,
		AudioSttPricePerMinute:  req.AudioSttPricePerMinute,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpsertTenantUserPrice(row))
}

// handleTenantsMeUserPricesDelete - 租户删除售价
func (s *Server) handleTenantsMeUserPricesDelete(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can manage user prices")
		return
	}

	modelID, ok2 := parseAPIUUIDParam(w, r, "modelID")
	if !ok2 {
		return
	}

	err := s.queries.DeleteTenantUserPrice(r.Context(), dbgen.DeleteTenantUserPriceParams{
		TenantID: ac.TenantID,
		ModelID:  modelID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, nil)
}

// ============================================================================
// /users/me/* routes - 用户自管理路由
// ============================================================================

// handleUsersMeAPIKeysCreate - 用户创建自己的 API Key
func (s *Server) handleUsersMeAPIKeysCreate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can create own api keys")
		return
	}
	var req createAPIKeyRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	key, err := apikey.Generate()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "key generation failed")
		return
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid allowed_models")
		return
	}
	row, err := s.queries.CreateAPIKey(r.Context(), dbgen.CreateAPIKeyParams{
		OwnerType:     "user",
		TenantID:      ac.TenantID,
		UserID:        pgtype.Text{String: ac.UserID, Valid: true},
		KeyHash:       apikey.Hash(key),
		LastFour:      pgtype.Text{String: apikey.LastFour(key), Valid: true},
		Name:          req.Name,
		QuotaLimit:    pgtype.Int8{Int64: req.QuotaLimit, Valid: req.QuotaLimit > 0},
		AllowedModels: allowedModels,
		Status:        defaultStatus,
		CreatedBy:     pgtype.Text{String: ac.UserID, Valid: true},
	})
	if err != nil {
		s.logger.Error("create user api key failed", "error", err)
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "create failed")
		return
	}
	writeOK(w, createAPIKeyResponse{PlaintextKey: key, Key: apiKeyDTOFromCreate(row)})
}

// handleUsersMeAPIKeysUpdate - 用户更新自己的 API Key
func (s *Server) handleUsersMeAPIKeysUpdate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can update own api keys")
		return
	}
	apiKeyID, ok := parseAPIUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	var req updateAPIKeyRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid allowed_models")
		return
	}
	row, err := s.queries.UpdateAPIKey(r.Context(), dbgen.UpdateAPIKeyParams{
		TenantID:      ac.TenantID,
		ID:            apiKeyID,
		Name:          req.Name,
		QuotaLimit:    pgtype.Int8{Int64: req.QuotaLimit, Valid: req.QuotaLimit > 0},
		AllowedModels: allowedModels,
		Status:        defaultStatus,
	})
	if err != nil {
		s.logger.Error("update user api key failed", "error", err)
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "update failed")
		return
	}
	s.invalidateAPIKeyCache(r.Context(), row.KeyHash)
	writeOK(w, apiKeyDTOFromUpdate(row))
}

// handleUsersMeAPIKeysStatus - 用户更新自己的 API Key 状态
func (s *Server) handleUsersMeAPIKeysStatus(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can update own api keys")
		return
	}
	apiKeyID, ok := parseAPIUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	var req updateStatusRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	row, err := s.queries.UpdateAPIKeyStatus(r.Context(), dbgen.UpdateAPIKeyStatusParams{
		TenantID: ac.TenantID,
		ID:       apiKeyID,
		Status:   req.Status,
	})
	if err != nil {
		s.logger.Error("update user api key status failed", "error", err)
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "update failed")
		return
	}
	s.invalidateAPIKeyCache(r.Context(), row.KeyHash)
	writeOK(w, apiKeyDTOFromUpdateStatus(row))
}

// handleTenantsMeAPIKeysDelete - 租户删除自己的 API Key
func (s *Server) handleTenantsMeAPIKeysDelete(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can delete own api keys")
		return
	}
	apiKeyID, ok := parseAPIUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	keyHash, err := s.queries.DeleteAPIKey(r.Context(), dbgen.DeleteAPIKeyParams{
		ID: apiKeyID, TenantID: ac.TenantID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), keyHash)
	writeOK(w, nil)
}

// handleUsersMeAPIKeysRotate - 用户 Rotate 自己的 API Key
func (s *Server) handleUsersMeAPIKeysRotate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can rotate own api keys")
		return
	}
	apiKeyID, ok := parseAPIUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	existing, err := s.queries.GetAPIKeyByID(r.Context(), dbgen.GetAPIKeyByIDParams{
		ID: apiKeyID, TenantID: ac.TenantID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	newKey, err := apikey.Generate()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "key generation failed")
		return
	}
	row, err := s.queries.RotateAPIKey(r.Context(), dbgen.RotateAPIKeyParams{
		ID: apiKeyID, TenantID: ac.TenantID,
		KeyHash:  apikey.Hash(newKey),
		LastFour: pgtype.Text{String: apikey.LastFour(newKey), Valid: true},
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "rotate failed")
		return
	}
	s.invalidateAPIKeyCache(r.Context(), existing.KeyHash)
	writeOK(w, rotateAPIKeyResponse{PlaintextKey: newKey, Key: apiKeyDTOFromRotate(row)})
}

// handleUsersMeAPIKeysDelete - 用户删除自己的 API Key
func (s *Server) handleUsersMeAPIKeysDelete(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can delete own api keys")
		return
	}
	apiKeyID, ok := parseAPIUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	keyHash, err := s.queries.DeleteAPIKey(r.Context(), dbgen.DeleteAPIKeyParams{
		ID: apiKeyID, TenantID: ac.TenantID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	s.invalidateAPIKeyCache(r.Context(), keyHash)
	writeOK(w, nil)
}

// ============================================================================
// 辅助函数和请求结构
// ============================================================================

type createAPIKeyRequest struct {
	Name          string   `json:"name"`
	QuotaLimit    int64    `json:"quota_limit"`
	AllowedModels []string `json:"allowed_models"`
}

type updateAPIKeyRequest struct {
	Name          string   `json:"name"`
	QuotaLimit    int64    `json:"quota_limit"`
	AllowedModels []string `json:"allowed_models"`
}

type upsertTenantUserPriceRequest struct {
	InputPricePer1m         int64           `json:"input_price_per_1m"`
	OutputPricePer1m        int64           `json:"output_price_per_1m"`
	ImagePrices             json.RawMessage `json:"image_prices"`
	VideoPrices             json.RawMessage `json:"video_prices"`
	AudioTtsPricePer1mChars int64           `json:"audio_tts_price_per_1m_chars"`
	AudioSttPricePerMinute  int64           `json:"audio_stt_price_per_minute"`
}

func decodeAPIJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid json")
		return false
	}
	return true
}

func parseInt32(s string) (int32, error) {
	n, err := json.Number(s).Int64()
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func parseAPIUUIDParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	value := chi.URLParam(r, param)
	if value == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, param+" is required")
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid "+param)
		return pgtype.UUID{}, false
	}
	return id, true
}

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
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	var rows interface{}
	var err error

	if ac.Role == apiRolePlatform {
		// 平台可以查看所有，或通过查询参数过滤
		tenantID := r.URL.Query().Get("tenant_id")
		if tenantID != "" {
			rows, err = s.queries.ListTenantAPIKeys(r.Context(), tenantID)
		} else {
			rows, err = s.queries.ListAllTenantAPIKeys(r.Context())
		}
	} else if ac.Role == apiRoleTenant {
		rows, err = s.queries.ListTenantAPIKeys(r.Context(), ac.TenantID)
	} else {
		writeAPIError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err != nil {
		s.logger.Error("list tenant api keys failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, rows)
}

// handleTenantModelGrantsSelf - 租户查看自己的模型授权
func (s *Server) handleTenantModelGrantsSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	var tenantID string
	if ac.Role == apiRolePlatform {
		// 平台可以指定 tenant_id 查询
		tenantID = r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			writeAPIError(w, http.StatusBadRequest, "tenant_id required for platform")
			return
		}
	} else if ac.Role == apiRoleTenant {
		tenantID = ac.TenantID
	} else {
		writeAPIError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := s.queries.ListTenantModelGrants(r.Context(), tenantID)
	if err != nil {
		s.logger.Error("list tenant model grants failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, rows)
}

// ============================================================================
// 用户自己的资源（根据 token 中的 userID 自动过滤）
// ============================================================================

// handleUserAPIKeysSelf - 用户查看自己的 API Key 列表
func (s *Server) handleUserAPIKeysSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	// 根据角色过滤
	if ac.Role == apiRoleUser {
		// 用户只能查看自己的
		rows, err := s.queries.ListUserAPIKeys(r.Context(), dbgen.ListUserAPIKeysParams{
			TenantID: ac.TenantID,
			UserID:   pgtype.Text{String: ac.UserID, Valid: true},
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "query failed")
			return
		}
		writeAPIJSON(w, http.StatusOK, rows)
	} else if ac.Role == apiRoleTenant {
		// 租户可以查看其租户下指定用户的
		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			writeAPIError(w, http.StatusBadRequest, "user_id required for tenant")
			return
		}
		rows, err := s.queries.ListUserAPIKeys(r.Context(), dbgen.ListUserAPIKeysParams{
			TenantID: ac.TenantID,
			UserID:   pgtype.Text{String: userID, Valid: true},
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "query failed")
			return
		}
		writeAPIJSON(w, http.StatusOK, rows)
	} else if ac.Role == apiRolePlatform {
		// 平台需要指定 tenant_id 和 user_id
		tenantID := r.URL.Query().Get("tenant_id")
		userID := r.URL.Query().Get("user_id")
		if tenantID == "" || userID == "" {
			writeAPIError(w, http.StatusBadRequest, "tenant_id and user_id required for platform")
			return
		}
		rows, err := s.queries.ListUserAPIKeys(r.Context(), dbgen.ListUserAPIKeysParams{
			TenantID: tenantID,
			UserID:   pgtype.Text{String: userID, Valid: true},
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "query failed")
			return
		}
		writeAPIJSON(w, http.StatusOK, rows)
	} else {
		writeAPIError(w, http.StatusForbidden, "forbidden")
	}
}

// handleUserModelGrantsSelf - 用户查看自己可用的模型授权
func (s *Server) handleUserModelGrantsSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeAPIError(w, http.StatusForbidden, "only for end users")
		return
	}

	// 用户可用的模型 = 租户授权的模型
	rows, err := s.queries.ListUserAvailableModels(r.Context(), ac.TenantID)
	if err != nil {
		s.logger.Error("list user available models failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, rows)
}

// ============================================================================
// Dashboard（根据角色返回不同维度数据）
// ============================================================================

// handleDashboardSummaryByRole - Dashboard 概览，根据角色返回不同数据
func (s *Server) handleDashboardSummaryByRole(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	// 根据角色设置查询参数
	params := dbgen.GetDashboardSummaryParams{}
	if ac.Role == apiRoleTenant {
		params.TenantID = pgtype.Text{String: ac.TenantID, Valid: true}
	} else if ac.Role == apiRoleUser {
		params.TenantID = pgtype.Text{String: ac.TenantID, Valid: true}
		params.UserID = pgtype.Text{String: ac.UserID, Valid: true}
	}

	summary, err := s.queries.GetDashboardSummary(r.Context(), params)
	if err != nil {
		s.logger.Error("dashboard summary failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, summary)
}

// ============================================================================
// Usage Logs（根据角色返回不同范围数据）
// ============================================================================

// handleUsageLogsByRole - 使用日志，根据角色返回不同范围
func (s *Server) handleUsageLogsByRole(w http.ResponseWriter, r *http.Request) {
	// 复用现有的 handler，根据角色调整数据范围
	s.handleAdminListUsageLogs(w, r)
}

// ============================================================================
// 租户售价（仅租户可管理）
// ============================================================================

// handleUserPricesSelf - 租户管理自己的售价
func (s *Server) handleUserPricesSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeAPIError(w, http.StatusForbidden, "only tenant can manage user prices")
		return
	}

	// 获取租户售价列表
	rows, err := s.queries.ListTenantUserPrices(r.Context(), ac.TenantID)
	if err != nil {
		s.logger.Error("list tenant user prices failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, rows)
}

// ============================================================================
// 用户使用记录（仅用户可查看）
// ============================================================================

// handleUserUsageLogsSelf - 用户查看自己的使用记录
func (s *Server) handleUserUsageLogsSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeAPIError(w, http.StatusForbidden, "only for end users")
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
		writeAPIError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, rows)
}

// handleUserUsageSummarySelf - 用户查看自己的使用汇总
func (s *Server) handleUserUsageSummarySelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeAPIError(w, http.StatusForbidden, "only for end users")
		return
	}

	summary, err := s.queries.ListUsageSummaryByTenantUser(r.Context(), dbgen.ListUsageSummaryByTenantUserParams{
		TenantID: ac.TenantID,
		UserID:   pgtype.Text{String: ac.UserID, Valid: true},
	})
	if err != nil {
		s.logger.Error("list user usage summary failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, summary)
}

// ============================================================================
// /tenants/me/* routes - 租户自管理路由
// ============================================================================

// handleTenantsMeAPIKeysCreate - 租户创建自己的 API Key
func (s *Server) handleTenantsMeAPIKeysCreate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeAPIError(w, http.StatusForbidden, "only tenant can create own api keys")
		return
	}

	var req createAPIKeyRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}

	// 生成 API Key
	key, err := apikey.Generate()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "key generation failed")
		return
	}
	hash := apikey.Hash(key)
	prefix := apikey.PrefixForDisplay(key)

	// 处理 allowed_models
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid allowed_models")
		return
	}

	row, err := s.queries.CreateTenantAPIKeySelf(r.Context(), dbgen.CreateTenantAPIKeySelfParams{
		TenantID:     ac.TenantID,
		KeyHash:      hash,
		KeyPrefix:    prefix,
		Name:         req.Name,
		QuotaLimit:   pgtype.Int8{Int64: req.QuotaLimit, Valid: req.QuotaLimit > 0},
		AllowedModels: allowedModels,
		Status:       defaultStatus,
		ExpiresAt:    pgtype.Timestamptz{},
		CreatedBy:    pgtype.Text{String: ac.UserID, Valid: true},
	})
	if err != nil {
		s.logger.Error("create tenant api key failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "create failed")
		return
	}

	// 返回创建结果 + 生成的完整 Key
	writeAPIJSON(w, http.StatusCreated, map[string]interface{}{
		"key":         key,
		"id":          row.ID,
		"key_prefix":  row.KeyPrefix,
		"name":        row.Name,
		"status":      row.Status,
		"created_at":  row.CreatedAt,
	})
}

// handleTenantsMeAPIKeysUpdate - 租户更新自己的 API Key
func (s *Server) handleTenantsMeAPIKeysUpdate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeAPIError(w, http.StatusForbidden, "only tenant can update own api keys")
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
		writeAPIError(w, http.StatusBadRequest, "invalid allowed_models")
		return
	}

	row, err := s.queries.UpdateTenantAPIKeySelf(r.Context(), dbgen.UpdateTenantAPIKeySelfParams{
		TenantID:     ac.TenantID,
		ID:           apiKeyID,
		Name:         req.Name,
		QuotaLimit:   pgtype.Int8{Int64: req.QuotaLimit, Valid: req.QuotaLimit > 0},
		AllowedModels: allowedModels,
	})
	if err != nil {
		s.logger.Error("update tenant api key failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, row)
}

// handleTenantsMeAPIKeysStatus - 租户更新自己的 API Key 状态
func (s *Server) handleTenantsMeAPIKeysStatus(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeAPIError(w, http.StatusForbidden, "only tenant can update own api keys")
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

	row, err := s.queries.UpdateTenantAPIKeyStatusSelf(r.Context(), dbgen.UpdateTenantAPIKeyStatusSelfParams{
		TenantID: ac.TenantID,
		ID:       apiKeyID,
		Status:   req.Status,
	})
	if err != nil {
		s.logger.Error("update tenant api key status failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, row)
}

// ============================================================================
// /tenants/me/user-prices/* routes - 租户售价自管理
// ============================================================================

// handleTenantsMeUserPricesUpsert - 租户设置售价
func (s *Server) handleTenantsMeUserPricesUpsert(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeAPIError(w, http.StatusForbidden, "only tenant can manage user prices")
		return
	}

	modelID := chi.URLParam(r, "modelID")
	if modelID == "" {
		writeAPIError(w, http.StatusBadRequest, "model_id required")
		return
	}

	var req upsertTenantUserPriceRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}

	// 复用现有的 handler 逻辑
	s.handleAdminUpsertTenantUserPrice(w, r)
}

// handleTenantsMeUserPricesDelete - 租户删除售价
func (s *Server) handleTenantsMeUserPricesDelete(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleTenant {
		writeAPIError(w, http.StatusForbidden, "only tenant can manage user prices")
		return
	}

	// 复用现有的 handler 逻辑
	s.handleAdminDeleteTenantUserPrice(w, r)
}

// ============================================================================
// /users/me/* routes - 用户自管理路由
// ============================================================================

// handleUsersMeAPIKeysCreate - 用户创建自己的 API Key
func (s *Server) handleUsersMeAPIKeysCreate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeAPIError(w, http.StatusForbidden, "only user can create own api keys")
		return
	}

	var req createAPIKeyRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}

	// 生成 API Key
	key, err := apikey.Generate()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "key generation failed")
		return
	}
	hash := apikey.Hash(key)
	prefix := apikey.PrefixForDisplay(key)

	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid allowed_models")
		return
	}

	row, err := s.queries.CreateUserAPIKeySelf(r.Context(), dbgen.CreateUserAPIKeySelfParams{
		TenantID:     ac.TenantID,
		UserID:       pgtype.Text{String: ac.UserID, Valid: true},
		KeyHash:      hash,
		KeyPrefix:    prefix,
		Name:         req.Name,
		QuotaLimit:   pgtype.Int8{Int64: req.QuotaLimit, Valid: req.QuotaLimit > 0},
		AllowedModels: allowedModels,
		Status:       defaultStatus,
		ExpiresAt:    pgtype.Timestamptz{},
		CreatedBy:    pgtype.Text{String: ac.UserID, Valid: true},
	})
	if err != nil {
		s.logger.Error("create user api key failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeAPIJSON(w, http.StatusCreated, map[string]interface{}{
		"key":         key,
		"id":          row.ID,
		"key_prefix":  row.KeyPrefix,
		"name":        row.Name,
		"status":      row.Status,
		"created_at":  row.CreatedAt,
	})
}

// handleUsersMeAPIKeysUpdate - 用户更新自己的 API Key
func (s *Server) handleUsersMeAPIKeysUpdate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeAPIError(w, http.StatusForbidden, "only user can update own api keys")
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
		writeAPIError(w, http.StatusBadRequest, "invalid allowed_models")
		return
	}

	row, err := s.queries.UpdateUserAPIKeySelf(r.Context(), dbgen.UpdateUserAPIKeySelfParams{
		TenantID:     ac.TenantID,
		UserID:       pgtype.Text{String: ac.UserID, Valid: true},
		ID:           apiKeyID,
		Name:         req.Name,
		QuotaLimit:   pgtype.Int8{Int64: req.QuotaLimit, Valid: req.QuotaLimit > 0},
		AllowedModels: allowedModels,
	})
	if err != nil {
		s.logger.Error("update user api key failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, row)
}

// handleUsersMeAPIKeysStatus - 用户更新自己的 API Key 状态
func (s *Server) handleUsersMeAPIKeysStatus(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeAPIError(w, http.StatusForbidden, "only user can update own api keys")
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

	row, err := s.queries.UpdateUserAPIKeyStatusSelf(r.Context(), dbgen.UpdateUserAPIKeyStatusSelfParams{
		TenantID: ac.TenantID,
		UserID:   pgtype.Text{String: ac.UserID, Valid: true},
		ID:       apiKeyID,
		Status:   req.Status,
	})
	if err != nil {
		s.logger.Error("update user api key status failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeAPIJSON(w, http.StatusOK, row)
}

// ============================================================================
// 辅助函数和请求结构
// ============================================================================

type createAPIKeyRequest struct {
	Name         string   `json:"name"`
	QuotaLimit   int64    `json:"quota_limit"`
	AllowedModels []string `json:"allowed_models"`
}

type updateAPIKeyRequest struct {
	Name         string   `json:"name"`
	QuotaLimit   int64    `json:"quota_limit"`
	AllowedModels []string `json:"allowed_models"`
}

type upsertTenantUserPriceRequest struct {
	InputPricePer1m  int64 `json:"input_price_per_1m"`
	OutputPricePer1m int64 `json:"output_price_per_1m"`
	RequestCost      int64 `json:"request_cost"`
	ImageCost        int64 `json:"image_cost"`
	ImageSizePrices  string `json:"image_size_prices"`
}

func decodeAPIJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func parseInt32(s string) (int32, error) {
	var i int32
	_, err := json.Number(s).Int64()
	if err != nil {
		return 0, err
	}
	return i, nil
}

func parseAPIUUIDParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	value := chi.URLParam(r, param)
	if value == "" {
		writeAPIError(w, http.StatusBadRequest, param+" is required")
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid "+param)
		return pgtype.UUID{}, false
	}
	return id, true
}
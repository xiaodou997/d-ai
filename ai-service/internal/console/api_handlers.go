package console

import (
	"encoding/json"
	"go.uber.org/zap"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	apikeysvc "xiaodou/unihub/ai-service/internal/service/apikey"
)

// ============================================================================
// 租户自己的资源（根据 token 中的 tenantID 自动过滤）
// ============================================================================

// handleTenantAPIKeysSelf - 租户查看自己的 API Key 列表
func (s *Console) handleTenantAPIKeysSelf(w http.ResponseWriter, r *http.Request) {
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

	keys, err := s.apiKeySvc.ListForTenant(r.Context(), tenantID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeysFromDomain(keys))
}

// handleTenantModelGrantsSelf - 租户查看自己的模型授权
func (s *Console) handleTenantModelGrantsSelf(w http.ResponseWriter, r *http.Request) {
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

	grants, err := s.grantSvc.ListForTenant(r.Context(), tenantID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}

	writeOK(w, listTenantModelGrantsFromDomain(grants))
}

// ============================================================================
// 用户自己的资源（根据 token 中的 userID 自动过滤）
// ============================================================================

// handleUserAPIKeysSelf - 用户查看自己的 API Key 列表
func (s *Console) handleUserAPIKeysSelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	var tenantID, userID string
	switch ac.Role {
	case apiRoleUser:
		tenantID = ac.TenantID
		userID = ac.UserID
	case apiRoleTenant:
		userID = r.URL.Query().Get("user_id")
		if userID == "" {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "user_id required for tenant")
			return
		}
		tenantID = ac.TenantID
	case apiRolePlatform:
		tenantID = r.URL.Query().Get("tenant_id")
		userID = r.URL.Query().Get("user_id")
		if tenantID == "" || userID == "" {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenant_id and user_id required for platform")
			return
		}
	default:
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return
	}

	keys, err := s.apiKeySvc.ListForUser(r.Context(), tenantID, userID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeysFromDomain(keys))
}

// handleUserModelGrantsSelf - 用户查看自己可用的模型授权
func (s *Console) handleUserModelGrantsSelf(w http.ResponseWriter, r *http.Request) {
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
		s.logger.Error("list user available models failed", zap.Error(err))
		writeDBErr(w, err)
		return
	}

	writeOK(w, fromListUserAvailableModels(rows))
}

// ============================================================================
// Dashboard（根据角色返回不同维度数据）
// ============================================================================

// handleDashboardSummaryByRole - Dashboard 概览，根据角色返回不同数据
func (s *Console) handleDashboardSummaryByRole(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	since, ok := parseDashboardSince(w, r)
	if !ok {
		return
	}
	filter := domain.DashboardFilter{Since: pgTimestamptzToPtr(since)}
	if ac.Role == apiRoleTenant {
		filter.TenantID = ac.TenantID
	} else if ac.Role == apiRoleUser {
		filter.TenantID = ac.TenantID
		filter.UserID = ac.UserID
	}

	summary, err := s.dashboardSvc.Summary(r.Context(), filter)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}

	writeOK(w, dashboardSummaryFromDomain(summary))
}

// ============================================================================
// Usage Logs（根据角色返回不同范围数据）
// ============================================================================

// handleUsageLogsByRole - 使用日志，根据角色返回不同范围和不同字段可见性
func (s *Console) handleUsageLogsByRole(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if ok && ac.Role == apiRoleTenant {
		s.handleTenantListUsageLogs(w, r)
		return
	}
	s.handleAdminListUsageLogs(w, r)
}

// ============================================================================
// 租户售价（仅租户可管理）
// ============================================================================

// ============================================================================
// 用户使用记录（仅用户可查看）
// ============================================================================

// handleUserUsageLogsSelf - 用户查看自己的使用记录
func (s *Console) handleUserUsageLogsSelf(w http.ResponseWriter, r *http.Request) {
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
		TenantID:      ac.TenantID,
		UserID:        pgtype.Text{String: ac.UserID, Valid: true},
		Limit:         limit,
		RequestSource: optionalTextValue(r.URL.Query().Get("request_source")),
	})
	if err != nil {
		s.logger.Error("list user usage logs failed", zap.Error(err))
		writeDBErr(w, err)
		return
	}

	writeOK(w, fromListUsageLogsByUser(rows))
}

// handleUserUsageSummarySelf - 用户查看自己的使用汇总
func (s *Console) handleUserUsageSummarySelf(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}

	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only for end users")
		return
	}

	summary, err := s.usageSvc.UserSummary(r.Context(), ac.TenantID, ac.UserID, r.URL.Query().Get("request_source"))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}

	writeOK(w, userUsageSummaryFromDomain(summary))
}

// ============================================================================
// /tenants/me/* routes - 租户自管理路由
// ============================================================================

// handleTenantsMeAPIKeysCreate - 租户创建自己的 API Key
func (s *Console) handleTenantsMeAPIKeysCreate(w http.ResponseWriter, r *http.Request) {
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
	if message := validateOptionalCreditAmount("quota_limit_credits", req.QuotaLimitCredits); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	created, err := s.apiKeySvc.Create(r.Context(), apikeysvc.CreateInput{
		OwnerType:         domain.OwnerTenant,
		TenantID:          ac.TenantID,
		Name:              req.Name,
		QuotaLimitCredits: req.QuotaLimitCredits,
		AllowedModels:     req.AllowedModels,
		CreatedBy:         ac.UserID,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, createAPIKeyResponse{PlaintextKey: created.PlaintextKey, Key: apiKeyFromDomain(created.Key)})
}

// handleTenantsMeAPIKeysUpdate - 租户更新自己的 API Key
func (s *Console) handleTenantsMeAPIKeysUpdate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can update own api keys")
		return
	}
	if _, ok := parseAPIUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	var req updateAPIKeyRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	if message := validateOptionalCreditAmount("quota_limit_credits", req.QuotaLimitCredits); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	key, err := s.apiKeySvc.Update(r.Context(), apikeysvc.UpdateInput{
		ID:                chi.URLParam(r, "apiKeyID"),
		TenantID:          ac.TenantID,
		Name:              req.Name,
		QuotaLimitCredits: req.QuotaLimitCredits,
		AllowedModels:     req.AllowedModels,
		Status:            defaultStatus,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeyFromDomain(key))
}

// handleTenantsMeAPIKeysStatus - 租户更新自己的 API Key 状态
func (s *Console) handleTenantsMeAPIKeysStatus(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can update own api keys")
		return
	}
	if _, ok := parseAPIUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	var req updateStatusRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	key, err := s.apiKeySvc.UpdateStatus(r.Context(), chi.URLParam(r, "apiKeyID"), ac.TenantID, req.Status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeyFromDomain(key))
}

// handleTenantsMeAPIKeysRotate - 租户 Rotate 自己的 API Key
func (s *Console) handleTenantsMeAPIKeysRotate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can rotate own api keys")
		return
	}
	if _, ok := parseAPIUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	created, err := s.apiKeySvc.Rotate(r.Context(), chi.URLParam(r, "apiKeyID"), ac.TenantID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, rotateAPIKeyResponse{PlaintextKey: created.PlaintextKey, Key: apiKeyFromDomain(created.Key)})
}

// ============================================================================
// /tenants/me/user-prices/* routes - 租户售价自管理
// ============================================================================

// ============================================================================
// /users/me/* routes - 用户自管理路由
// ============================================================================

// handleUsersMeAPIKeysCreate - 用户创建自己的 API Key
func (s *Console) handleUsersMeAPIKeysCreate(w http.ResponseWriter, r *http.Request) {
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
	if message := validateOptionalCreditAmount("quota_limit_credits", req.QuotaLimitCredits); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	created, err := s.apiKeySvc.Create(r.Context(), apikeysvc.CreateInput{
		OwnerType:         domain.OwnerUser,
		TenantID:          ac.TenantID,
		UserID:            ac.UserID,
		Name:              req.Name,
		QuotaLimitCredits: req.QuotaLimitCredits,
		AllowedModels:     req.AllowedModels,
		CreatedBy:         ac.UserID,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, createAPIKeyResponse{PlaintextKey: created.PlaintextKey, Key: apiKeyFromDomain(created.Key)})
}

// handleUsersMeAPIKeysUpdate - 用户更新自己的 API Key
func (s *Console) handleUsersMeAPIKeysUpdate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can update own api keys")
		return
	}
	if _, ok := parseAPIUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	var req updateAPIKeyRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	if message := validateOptionalCreditAmount("quota_limit_credits", req.QuotaLimitCredits); message != "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, message)
		return
	}
	key, err := s.apiKeySvc.Update(r.Context(), apikeysvc.UpdateInput{
		ID:                chi.URLParam(r, "apiKeyID"),
		TenantID:          ac.TenantID,
		Name:              req.Name,
		QuotaLimitCredits: req.QuotaLimitCredits,
		AllowedModels:     req.AllowedModels,
		Status:            defaultStatus,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeyFromDomain(key))
}

// handleUsersMeAPIKeysStatus - 用户更新自己的 API Key 状态
func (s *Console) handleUsersMeAPIKeysStatus(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can update own api keys")
		return
	}
	if _, ok := parseAPIUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	var req updateStatusRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	key, err := s.apiKeySvc.UpdateStatus(r.Context(), chi.URLParam(r, "apiKeyID"), ac.TenantID, req.Status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, apiKeyFromDomain(key))
}

// handleTenantsMeAPIKeysDelete - 租户删除自己的 API Key
func (s *Console) handleTenantsMeAPIKeysDelete(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleTenant {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only tenant can delete own api keys")
		return
	}
	if _, ok := parseAPIUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	if err := s.apiKeySvc.Delete(r.Context(), chi.URLParam(r, "apiKeyID"), ac.TenantID); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

// handleUsersMeAPIKeysRotate - 用户 Rotate 自己的 API Key
func (s *Console) handleUsersMeAPIKeysRotate(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can rotate own api keys")
		return
	}
	if _, ok := parseAPIUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	created, err := s.apiKeySvc.Rotate(r.Context(), chi.URLParam(r, "apiKeyID"), ac.TenantID)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, rotateAPIKeyResponse{PlaintextKey: created.PlaintextKey, Key: apiKeyFromDomain(created.Key)})
}

// handleUsersMeAPIKeysDelete - 用户删除自己的 API Key
func (s *Console) handleUsersMeAPIKeysDelete(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	if ac.Role != apiRoleUser {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "only user can delete own api keys")
		return
	}
	if _, ok := parseAPIUUIDParam(w, r, "apiKeyID"); !ok {
		return
	}
	if err := s.apiKeySvc.Delete(r.Context(), chi.URLParam(r, "apiKeyID"), ac.TenantID); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

// ============================================================================
// 辅助函数和请求结构
// ============================================================================

type createAPIKeyRequest struct {
	Name              string   `json:"name"`
	QuotaLimitCredits *int64   `json:"quota_limit_credits"` // 积分，nil=无限制
	AllowedModels     []string `json:"allowed_models"`
}

type updateAPIKeyRequest struct {
	Name              string   `json:"name"`
	QuotaLimitCredits *int64   `json:"quota_limit_credits"` // 积分，nil=无限制
	AllowedModels     []string `json:"allowed_models"`
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

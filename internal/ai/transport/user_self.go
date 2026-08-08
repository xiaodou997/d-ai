package transport

// user_self.go registers the end-user self-service (userType=4) flat Huma
// endpoints. Every handler derives the tenant+user scope from the JWT claims
// (tenantIDFromContext / userIDFromContext) — never from a path/query param —
// so an end user can only ever read/write its own resources. These mirror the
// role-dispatching console envelope routes (handleUserAPIKeysSelf,
// handleUsersMeAPIKeys*, handleUserModelGrantsSelf, handleUsersMeEffectivePrices,
// handleUserUsageLogsSelf, handleUserUsageSummarySelf) but speak the flat Huma
// contract and bind identity from claims.

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/dai/internal/ai/commercial"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/libs/go/httpx"
)

// ---------------------------------------------------------------------------
// inputs
// ---------------------------------------------------------------------------

type userSelfAPIKeyWriteInput struct {
	Body apiKeyWriteRequest
}

type userSelfUpdateAPIKeyInput struct {
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
	Body     apiKeyWriteRequest
}

type userSelfUpdateAPIKeyStatusInput struct {
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
	Body     apiKeyStatusRequest
}

type userSelfAPIKeyIDInput struct {
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
}

type userSelfUsageLogsInput struct {
	RequestSource string `query:"request_source" doc:"请求来源过滤"`
	Limit         int32  `query:"limit" default:"100" doc:"返回条数；默认 100，最大 100"`
}

type userSelfUsageSummaryInput struct {
	RequestSource string `query:"request_source" doc:"请求来源过滤"`
}

// ---------------------------------------------------------------------------
// user available models DTO (claims-scoped). Mirrors console
// fromListUserAvailableModels but returns flat Huma fields.
// ---------------------------------------------------------------------------

type userAvailableModelDTO struct {
	ModelCode             string                  `json:"model_code" doc:"模型编码"`
	ModelName             string                  `json:"model_name" doc:"模型名称"`
	CapabilityType        string                  `json:"capability_type" doc:"能力类型"`
	InputPer1MUSDMin      float64                 `json:"input_per_1m_usd_min" doc:"输入每 100 万 token 的原始 USD 最低价"`
	InputPer1MUSDMax      float64                 `json:"input_per_1m_usd_max" doc:"输入每 100 万 token 的原始 USD 最高价"`
	OutputPer1MUSDMin     float64                 `json:"output_per_1m_usd_min" doc:"输出每 100 万 token 的原始 USD 最低价"`
	OutputPer1MUSDMax     float64                 `json:"output_per_1m_usd_max" doc:"输出每 100 万 token 的原始 USD 最高价"`
	CacheWritePer1MUSDMin float64                 `json:"cache_write_per_1m_usd_min" doc:"缓存写入每 100 万 token 的原始 USD 最低价"`
	CacheWritePer1MUSDMax float64                 `json:"cache_write_per_1m_usd_max" doc:"缓存写入每 100 万 token 的原始 USD 最高价"`
	CacheReadPer1MUSDMin  float64                 `json:"cache_read_per_1m_usd_min" doc:"缓存读取每 100 万 token 的原始 USD 最低价"`
	CacheReadPer1MUSDMax  float64                 `json:"cache_read_per_1m_usd_max" doc:"缓存读取每 100 万 token 的原始 USD 最高价"`
	HasContextTiers       bool                    `json:"has_context_tiers" doc:"是否存在按输入上下文切换的价格档位"`
	ImageDefaultPriceMin  float64                 `json:"image_default_price_usd_min" doc:"图片默认原始 USD 最低价（每张）"`
	ImageDefaultPriceMax  float64                 `json:"image_default_price_usd_max" doc:"图片默认原始 USD 最高价（每张）"`
	VideoDefaultPriceMin  float64                 `json:"video_default_price_usd_min" doc:"视频默认原始 USD 最低价（每秒）"`
	VideoDefaultPriceMax  float64                 `json:"video_default_price_usd_max" doc:"视频默认原始 USD 最高价（每秒）"`
	ImagePrices           []resolutionUSDRangeDTO `json:"image_prices,omitempty" doc:"图片按 1k、2k、4k 尺寸档位的原始 USD 价格区间（每张）"`
	VideoPrices           []resolutionUSDRangeDTO `json:"video_prices,omitempty" doc:"视频按规格的原始 USD 价格区间（每秒）"`
}

type resolutionUSDRangeDTO struct {
	Resolution  string  `json:"resolution" doc:"分辨率/规格"`
	PriceUSDMin float64 `json:"price_usd_min" doc:"该规格的原始 USD 最低价"`
	PriceUSDMax float64 `json:"price_usd_max" doc:"该规格的原始 USD 最高价"`
}

type userAvailableModelsOutput struct {
	Body struct {
		Items []userAvailableModelDTO `json:"items"`
		Total int                     `json:"total"`
	}
}

type userGroupEffectivePricesOutput struct {
	Body struct {
		GroupID                 string                         `json:"group_id"`
		EffectiveUserMultiplier float64                        `json:"effective_user_multiplier"`
		Items                   []tenantGroupEffectivePriceDTO `json:"items"`
		Total                   int                            `json:"total"`
	}
}

// registerUserSelf mounts the end-user self-service endpoints under the end-user
// auth group (endUserAuth → userType=4).
func registerUserSelf(api huma.API, d AIDeps) {
	registerUserSelfGroups(api, d)
	registerUserSelfAPIKeys(api, d)
	registerUserSelfModelGrants(api, d)
	registerUserSelfLimits(api, d)
	registerUserSelfUsage(api, d)
}

func registerUserSelfGroups(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-groups",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/groups",
		Summary:     "终端用户自助可见分组列表",
		Description: "按当前用户 token 返回该用户可见的分组及生效售价倍率。",
		Tags:        []string{"groups"},
	}, func(ctx context.Context, _ *struct{}) (*userVisibleGroupsOutput, error) {
		if d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		groups, err := d.CommercialSvc.ListVisibleGroupsForUser(ctx, tenantID, userID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &userVisibleGroupsOutput{}
		out.Body.Items = make([]userVisibleGroupDTO, 0, len(groups))
		for _, item := range groups {
			out.Body.Items = append(out.Body.Items, userVisibleGroupToDTO(item))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-group-effective-prices",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/groups/{groupID}/effective-prices",
		Summary:     "终端用户自助分组生效售价",
		Description: "返回当前用户在某可见分组下可使用的模型及其生效 USD 单价（售价表 USD 单价 × 终端用户生效倍率）。",
		Tags:        []string{"groups"},
	}, func(ctx context.Context, in *tenantSelfGroupIDInput) (*userGroupEffectivePricesOutput, error) {
		if d.CommercialSvc == nil || d.PriceBookSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial/pricebook service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		groups, err := d.CommercialSvc.ListVisibleGroupsForUser(ctx, tenantID, userID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		var matched *commercial.AccessibleGroup
		for _, item := range groups {
			if item.Group.ID == in.GroupID {
				matchedGroup := item
				matched = &matchedGroup
				break
			}
		}
		if matched == nil {
			return nil, httpx.ErrNotFound.WithDetail("group is not visible to this user")
		}

		out := &userGroupEffectivePricesOutput{}
		out.Body.GroupID = matched.Group.ID
		out.Body.EffectiveUserMultiplier = matched.EffectiveUserMultiplier
		out.Body.Items = make([]tenantGroupEffectivePriceDTO, 0)

		entries, err := listRoutedGroupPriceEntries(ctx, d, matched.Group.ID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		factor := matched.EffectiveUserMultiplier
		for _, e := range entries {
			out.Body.Items = append(out.Body.Items, tenantGroupEffectivePriceDTO{
				ModelCode:             e.ModelCode,
				CapabilityType:        e.CapabilityType,
				TokenPriceTiers:       effectiveTokenPriceTiers(e.TokenPriceTiers, factor),
				ImageDefaultPriceUSD:  e.ImageDefaultPrice * factor,
				VideoDefaultPriceUSD:  e.VideoDefaultPrice * factor,
				ImagePrices:           resolutionUSDPrices(e.ImagePrices, factor),
				VideoPrices:           resolutionUSDPrices(e.VideoPrices, factor),
				AudioTTSPer1MCharsUSD: e.AudioTTSPerChar * pricebookPerMillion * factor,
				AudioSTTPerMinuteUSD:  e.AudioSTTPerMinute * factor,
			})
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

// ---------------------------------------------------------------------------
// API keys self (console: handleUserAPIKeysSelf / handleUsersMeAPIKeys*)
//   GET    /api/v1/user-api-keys
//   POST   /api/v1/users/me/api-keys
//   PATCH  /api/v1/users/me/api-keys/{apiKeyID}
//   PATCH  /api/v1/users/me/api-keys/{apiKeyID}/status
//   POST   /api/v1/users/me/api-keys/{apiKeyID}/rotate
//   DELETE /api/v1/users/me/api-keys/{apiKeyID}
// ---------------------------------------------------------------------------

func registerUserSelfAPIKeys(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-api-keys",
		Method:      http.MethodGet,
		Path:        "/api/v1/user-api-keys",
		Summary:     "终端用户自助 API key 列表",
		Description: "按当前用户 token 查询本用户拥有的 API key。仅返回 last_four，不返回明文 key/hash/密文。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, _ *struct{}) (*apiKeysOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		keys, err := d.APIKeySvc.ListForUser(ctx, tenantID, userID)
		return apiKeysResponse(ctx, d.CommercialSvc, keys, err)
	})

	huma.Register(api, huma.Operation{
		OperationID:   "ai-create-user-self-api-key",
		Method:        http.MethodPost,
		Path:          "/api/v1/users/me/api-keys",
		Summary:       "创建终端用户自助 API key",
		Description:   "按当前用户 token 创建本用户拥有的 API key。响应返回新密钥明文，后续也可再次复制。",
		Tags:          []string{"api-keys"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *userSelfAPIKeyWriteInput) (*apiKeyCreatedOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		write, err := userAPIKeyCreateInput(tenantID, userID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureAPIKeyGroupAccessible(ctx, d.CommercialSvc, tenantID, userID, in.Body.GroupID); err != nil {
			return nil, mapServiceError(err)
		}
		if strings.TrimSpace(write.CreatedBy) == "" {
			write.CreatedBy = userID
		}
		created, err := d.APIKeySvc.Create(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		limitPolicy, err := syncAPIKeyLimitPolicy(ctx, d.CommercialSvc, created.Key.ID, in.Body.LimitPolicy, write.CreatedBy)
		if err != nil {
			_ = d.APIKeySvc.Delete(ctx, created.Key.ID, tenantID)
			return nil, mapServiceError(err)
		}
		out := &apiKeyCreatedOutput{}
		out.Body.PlaintextKey = created.PlaintextKey
		out.Body.Key = apiKeyToDTO(created.Key, limitPolicy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-user-self-api-key",
		Method:      http.MethodPatch,
		Path:        "/api/v1/users/me/api-keys/{apiKeyID}",
		Summary:     "更新终端用户自助 API key",
		Description: "按当前用户 token 更新本用户 API key 的基础信息与独立限流策略。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *userSelfUpdateAPIKeyInput) (*apiKeyOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		write, err := apiKeyUpdateInput(tenantID, in.APIKeyID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, tenantID, userID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureAPIKeyGroupAccessible(ctx, d.CommercialSvc, tenantID, userID, in.Body.GroupID); err != nil {
			return nil, mapServiceError(err)
		}
		key, err := d.APIKeySvc.Update(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		limitPolicy, err := syncAPIKeyLimitPolicy(ctx, d.CommercialSvc, key.ID, in.Body.LimitPolicy, userID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyOutput{}
		out.Body = apiKeyToDTO(key, limitPolicy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-user-self-api-key-status",
		Method:      http.MethodPatch,
		Path:        "/api/v1/users/me/api-keys/{apiKeyID}/status",
		Summary:     "更新终端用户自助 API key 状态",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *userSelfUpdateAPIKeyStatusInput) (*apiKeyOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		status, err := apiKeyStatusInput(in.Body.Status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, tenantID, userID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		key, err := d.APIKeySvc.UpdateStatus(ctx, in.APIKeyID, tenantID, status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyOutput{}
		out.Body = apiKeyToDTO(key, nil)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-reveal-user-self-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/users/me/api-keys/{apiKeyID}/reveal",
		Summary:     "回显终端用户自助 API key 明文",
		Description: "按当前用户 token 读取本用户 API key 的当前完整明文，用于再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *userSelfAPIKeyIDInput) (*apiKeyRevealOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, tenantID, userID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		plaintext, err := d.APIKeySvc.Reveal(ctx, in.APIKeyID, tenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyRevealOutput{}
		out.Body.PlaintextKey = plaintext
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-rotate-user-self-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/users/me/api-keys/{apiKeyID}/rotate",
		Summary:     "轮换终端用户自助 API key",
		Description: "按当前用户 token 为本用户 API key 生成新密钥并失效旧密钥缓存。响应返回新密钥明文，后续也可再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *userSelfAPIKeyIDInput) (*apiKeyCreatedOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, tenantID, userID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		created, err := d.APIKeySvc.Rotate(ctx, in.APIKeyID, tenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyCreatedOutput{}
		out.Body.PlaintextKey = created.PlaintextKey
		out.Body.Key = apiKeyToDTO(created.Key, nil)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-user-self-api-key",
		Method:      http.MethodDelete,
		Path:        "/api/v1/users/me/api-keys/{apiKeyID}",
		Summary:     "删除终端用户自助 API key",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *userSelfAPIKeyIDInput) (*deleteAPIKeyOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, tenantID, userID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := d.APIKeySvc.Delete(ctx, in.APIKeyID, tenantID); err != nil {
			return nil, mapServiceError(err)
		}
		if d.CommercialSvc != nil {
			_ = d.CommercialSvc.DeleteLimitPolicies(ctx, commercial.LimitPolicyFilter{
				ScopeType: commercial.LimitScopeAPIKey,
				ScopeID:   in.APIKeyID,
			})
		}
		out := &deleteAPIKeyOutput{}
		out.Body.Deleted = true
		return out, nil
	})

}

// ---------------------------------------------------------------------------
// available models self (console: handleUserModelGrantsSelf)
//   GET /api/v1/user-model-grants
// 用户可用的模型 = 当前用户所属租户的 active 模型授权。
// ---------------------------------------------------------------------------

func registerUserSelfModelGrants(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-model-grants",
		Method:      http.MethodGet,
		Path:        "/api/v1/user-model-grants",
		Summary:     "终端用户自助可用模型列表",
		Description: "按当前用户 token 返回本用户可用的模型（租户默认公开分组 ∪ 用户例外分组）。价格表只提供计价，不扩大可用范围。",
		Tags:        []string{"model-grants"},
	}, func(ctx context.Context, _ *struct{}) (*userAvailableModelsOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres dependency is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		userID := userIDFromContext(ctx)
		if userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("user id is required")
		}
		rows, err := listAvailableModelsForScope(ctx, d.Postgres, tenantID, userID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &userAvailableModelsOutput{}
		out.Body.Items = make([]userAvailableModelDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, row)
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

// ---------------------------------------------------------------------------
// usage self (console: handleUserUsageLogsSelf / handleUserUsageSummarySelf)
//   GET /api/v1/user-usage-logs
//   GET /api/v1/user-usage-summary
// ---------------------------------------------------------------------------

func registerUserSelfUsage(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-usage-logs",
		Method:      http.MethodGet,
		Path:        "/api/v1/user-usage-logs",
		Summary:     "终端用户自助用量日志列表",
		Description: "按当前用户 token 返回本用户的用量日志。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *userSelfUsageLogsInput) (*userUsageLogsOutput, error) {
		if d.Queries == nil {
			return nil, httpx.ErrUnavailable.WithDetail("queries dependency is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		limit, err := userUsageLimitFromInput(in.Limit)
		if err != nil {
			return nil, err
		}
		rows, err := d.Queries.ListUsageLogsByTenantUser(ctx, dbgen.ListUsageLogsByTenantUserParams{
			TenantID:      tenantID,
			UserID:        pgtype.Text{String: userID, Valid: true},
			Limit:         limit,
			RequestSource: stringToText(in.RequestSource),
		})
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &userUsageLogsOutput{}
		out.Body.Items = make([]userUsageLogDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, userUsageLogToDTO(row))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-user-self-usage-summary",
		Method:      http.MethodGet,
		Path:        "/api/v1/user-usage-summary",
		Summary:     "终端用户自助用量汇总",
		Description: "按当前用户 token 返回本用户的用量汇总。",
		Tags:        []string{"usage"},
	}, func(ctx context.Context, in *userSelfUsageSummaryInput) (*userUsageSummaryOutput, error) {
		if d.UsageSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("usage service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		summary, err := d.UsageSvc.UserSummary(ctx, tenantID, userID, in.RequestSource)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &userUsageSummaryOutput{}
		out.Body = userUsageSummaryToDTO(summary)
		return out, nil
	})
}

func transportNumericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, _ := n.Float64Value()
	return f.Float64
}

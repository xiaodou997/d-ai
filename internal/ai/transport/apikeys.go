package transport

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/commercial"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/libs/go/httpx"
)

type tenantAPIKeysInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
}

type userAPIKeysInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	UserID   string `path:"userID" doc:"用户 ID"`
}

type createTenantAPIKeyInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	Body     apiKeyWriteRequest
}

type updateTenantAPIKeyInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
	Body     apiKeyWriteRequest
}

type updateTenantAPIKeyStatusInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
	Body     apiKeyStatusRequest
}

type rotateTenantAPIKeyInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
}

type deleteTenantAPIKeyInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
}

type createUserAPIKeyInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	UserID   string `path:"userID" doc:"用户 ID"`
	Body     apiKeyWriteRequest
}

type updateUserAPIKeyInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	UserID   string `path:"userID" doc:"用户 ID"`
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
	Body     apiKeyWriteRequest
}

type updateUserAPIKeyStatusInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	UserID   string `path:"userID" doc:"用户 ID"`
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
	Body     apiKeyStatusRequest
}

type rotateUserAPIKeyInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	UserID   string `path:"userID" doc:"用户 ID"`
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
}

type deleteUserAPIKeyInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	UserID   string `path:"userID" doc:"用户 ID"`
	APIKeyID string `path:"apiKeyID" doc:"API key ID"`
}

type apiKeyDTO struct {
	ID                 string                 `json:"id" doc:"API key ID"`
	OwnerType          string                 `json:"owner_type" enum:"tenant,user" doc:"归属主体类型"`
	TenantID           string                 `json:"tenant_id" doc:"租户 ID"`
	UserID             *string                `json:"user_id,omitempty" doc:"用户 ID；租户 key 为空"`
	GroupID            string                 `json:"group_id" doc:"密钥唯一绑定的分组 ID"`
	LastFour           *string                `json:"last_four,omitempty" doc:"API key 后四位，仅用于展示"`
	Name               string                 `json:"name" doc:"名称"`
	QuotaLimitMicroUSD *int64                 `json:"quota_limit_micro_usd,omitempty" doc:"额度上限，单位 micro-USD；为空表示无限制"`
	QuotaUsedMicroUSD  int64                  `json:"quota_used_micro_usd" doc:"已使用额度，单位 micro-USD"`
	Status             string                 `json:"status" enum:"active,disabled" doc:"状态"`
	ExpiresAt          *int64                 `json:"expires_at,omitempty" doc:"过期时间，Unix 毫秒"`
	LastUsedAt         *int64                 `json:"last_used_at,omitempty" doc:"最后使用时间，Unix 毫秒"`
	LimitPolicy        *runtimeLimitPolicyDTO `json:"limit_policy,omitempty" doc:"API key 独立限流策略"`
	CreatedBy          *string                `json:"created_by,omitempty" doc:"创建人"`
	CreatedAt          *int64                 `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt          *int64                 `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type apiKeyWriteRequest struct {
	Name               string                         `json:"name" doc:"名称"`
	GroupID            string                         `json:"group_id" doc:"密钥唯一绑定的分组 ID"`
	QuotaLimitMicroUSD *int64                         `json:"quota_limit_micro_usd,omitempty" minimum:"0" doc:"额度上限，单位 micro-USD；为空表示无限制"`
	Status             string                         `json:"status,omitempty" enum:"active,disabled" doc:"状态；为空默认 active"`
	ExpiresAt          *int64                         `json:"expires_at,omitempty" doc:"过期时间，Unix 毫秒；为空表示不过期"`
	LimitPolicy        *scopedLimitPolicyWriteRequest `json:"limit_policy,omitempty" doc:"API key 独立限流策略"`
	CreatedBy          string                         `json:"created_by,omitempty" doc:"创建人；仅创建时使用"`
}

type apiKeyStatusRequest struct {
	Status string `json:"status" enum:"active,disabled" doc:"状态"`
}

type apiKeysOutput struct {
	Body struct {
		Items []apiKeyDTO `json:"items"`
		Total int         `json:"total"`
	}
}

type apiKeyOutput struct {
	Body apiKeyDTO
}

type apiKeyCreatedOutput struct {
	Body struct {
		PlaintextKey string    `json:"plaintext_key" doc:"新生成的 API key 明文；创建/轮换响应返回"`
		Key          apiKeyDTO `json:"key"`
	}
}

type deleteAPIKeyOutput struct {
	Body struct {
		Deleted bool `json:"deleted" doc:"是否已删除"`
	}
}

type apiKeyRevealOutput struct {
	Body struct {
		PlaintextKey string `json:"plaintext_key" doc:"当前 API key 的完整明文"`
	}
}

func registerAPIKeys(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-api-keys",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/{tenantID}/api-keys",
		Summary:     "租户 API key 列表",
		Description: "平台代管视图：返回指定租户拥有的 API key 列表，含唯一绑定分组、额度和独立限流摘要。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *tenantAPIKeysInput) (*apiKeysOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID is required")
		}
		keys, err := d.APIKeySvc.ListForTenant(ctx, in.TenantID)
		return apiKeysResponse(ctx, d.CommercialSvc, keys, err)
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-create-tenant-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/{tenantID}/api-keys",
		Summary:     "创建租户 API key",
		Description: "平台代管视图：创建租户拥有的 API key。响应返回新密钥明文，后续也可通过 reveal 接口再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *createTenantAPIKeyInput) (*apiKeyCreatedOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID is required")
		}
		write, err := tenantAPIKeyCreateInput(in.TenantID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureAPIKeyGroupAccessible(ctx, d.CommercialSvc, in.TenantID, "", in.Body.GroupID); err != nil {
			return nil, mapServiceError(err)
		}
		created, err := d.APIKeySvc.Create(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		limitPolicy, err := syncAPIKeyLimitPolicy(ctx, d.CommercialSvc, created.Key.ID, in.Body.LimitPolicy, write.CreatedBy)
		if err != nil {
			_ = d.APIKeySvc.Delete(ctx, created.Key.ID, in.TenantID)
			return nil, mapServiceError(err)
		}
		out := &apiKeyCreatedOutput{}
		out.Body.PlaintextKey = created.PlaintextKey
		out.Body.Key = apiKeyToDTO(created.Key, limitPolicy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-tenant-api-key",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tenants/{tenantID}/api-keys/{apiKeyID}",
		Summary:     "更新租户 API key",
		Description: "平台代管视图：更新租户 API key 的基础信息与独立限流策略。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *updateTenantAPIKeyInput) (*apiKeyOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID is required")
		}
		write, err := apiKeyUpdateInput(in.TenantID, in.APIKeyID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureAPIKeyGroupAccessible(ctx, d.CommercialSvc, in.TenantID, "", in.Body.GroupID); err != nil {
			return nil, mapServiceError(err)
		}
		key, err := d.APIKeySvc.Update(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		limitPolicy, err := syncAPIKeyLimitPolicy(ctx, d.CommercialSvc, key.ID, in.Body.LimitPolicy, userIDFromContext(ctx))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyOutput{}
		out.Body = apiKeyToDTO(key, limitPolicy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-tenant-api-key-status",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tenants/{tenantID}/api-keys/{apiKeyID}/status",
		Summary:     "更新租户 API key 状态",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *updateTenantAPIKeyStatusInput) (*apiKeyOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID is required")
		}
		status, err := apiKeyStatusInput(in.Body.Status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		key, err := d.APIKeySvc.UpdateStatus(ctx, in.APIKeyID, in.TenantID, status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyOutput{}
		out.Body = apiKeyToDTO(key, nil)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-reveal-tenant-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/{tenantID}/api-keys/{apiKeyID}/reveal",
		Summary:     "回显租户 API key 明文",
		Description: "平台代管视图：读取租户 API key 的当前完整明文，用于再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *rotateTenantAPIKeyInput) (*apiKeyRevealOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID is required")
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		plaintext, err := d.APIKeySvc.Reveal(ctx, in.APIKeyID, in.TenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyRevealOutput{}
		out.Body.PlaintextKey = plaintext
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-rotate-tenant-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/{tenantID}/api-keys/{apiKeyID}/rotate",
		Summary:     "轮换租户 API key",
		Description: "平台代管视图：为租户 API key 生成新密钥并失效旧密钥缓存。响应返回新密钥明文，后续也可再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *rotateTenantAPIKeyInput) (*apiKeyCreatedOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID is required")
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		created, err := d.APIKeySvc.Rotate(ctx, in.APIKeyID, in.TenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyCreatedOutput{}
		out.Body.PlaintextKey = created.PlaintextKey
		out.Body.Key = apiKeyToDTO(created.Key, nil)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-tenant-api-key",
		Method:      http.MethodDelete,
		Path:        "/api/v1/tenants/{tenantID}/api-keys/{apiKeyID}",
		Summary:     "删除租户 API key",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *deleteTenantAPIKeyInput) (*deleteAPIKeyOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		if in.TenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID is required")
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := d.APIKeySvc.Delete(ctx, in.APIKeyID, in.TenantID); err != nil {
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

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-api-keys",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/{tenantID}/users/{userID}/api-keys",
		Summary:     "用户 API key 列表",
		Description: "平台代管视图：返回指定租户下指定用户拥有的 API key 列表，含唯一绑定分组、额度和独立限流摘要。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *userAPIKeysInput) (*apiKeysOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		if in.TenantID == "" || in.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID and userID are required")
		}
		keys, err := d.APIKeySvc.ListForUser(ctx, in.TenantID, in.UserID)
		return apiKeysResponse(ctx, d.CommercialSvc, keys, err)
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-create-user-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/{tenantID}/users/{userID}/api-keys",
		Summary:     "创建用户 API key",
		Description: "平台代管视图：创建指定租户下用户拥有的 API key。响应返回新密钥明文，后续也可通过 reveal 接口再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *createUserAPIKeyInput) (*apiKeyCreatedOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		if in.TenantID == "" || in.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID and userID are required")
		}
		write, err := userAPIKeyCreateInput(in.TenantID, in.UserID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureAPIKeyGroupAccessible(ctx, d.CommercialSvc, in.TenantID, in.UserID, in.Body.GroupID); err != nil {
			return nil, mapServiceError(err)
		}
		created, err := d.APIKeySvc.Create(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		limitPolicy, err := syncAPIKeyLimitPolicy(ctx, d.CommercialSvc, created.Key.ID, in.Body.LimitPolicy, write.CreatedBy)
		if err != nil {
			_ = d.APIKeySvc.Delete(ctx, created.Key.ID, in.TenantID)
			return nil, mapServiceError(err)
		}
		out := &apiKeyCreatedOutput{}
		out.Body.PlaintextKey = created.PlaintextKey
		out.Body.Key = apiKeyToDTO(created.Key, limitPolicy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-user-api-key",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}",
		Summary:     "更新用户 API key",
		Description: "平台代管视图：更新用户 API key 的基础信息与独立限流策略。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *updateUserAPIKeyInput) (*apiKeyOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		if in.TenantID == "" || in.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID and userID are required")
		}
		write, err := apiKeyUpdateInput(in.TenantID, in.APIKeyID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.UserID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureAPIKeyGroupAccessible(ctx, d.CommercialSvc, in.TenantID, in.UserID, in.Body.GroupID); err != nil {
			return nil, mapServiceError(err)
		}
		key, err := d.APIKeySvc.Update(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		limitPolicy, err := syncAPIKeyLimitPolicy(ctx, d.CommercialSvc, key.ID, in.Body.LimitPolicy, userIDFromContext(ctx))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyOutput{}
		out.Body = apiKeyToDTO(key, limitPolicy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-user-api-key-status",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}/status",
		Summary:     "更新用户 API key 状态",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *updateUserAPIKeyStatusInput) (*apiKeyOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		if in.TenantID == "" || in.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID and userID are required")
		}
		status, err := apiKeyStatusInput(in.Body.Status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.UserID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		key, err := d.APIKeySvc.UpdateStatus(ctx, in.APIKeyID, in.TenantID, status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyOutput{}
		out.Body = apiKeyToDTO(key, nil)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-reveal-user-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}/reveal",
		Summary:     "回显用户 API key 明文",
		Description: "平台代管视图：读取用户 API key 的当前完整明文，用于再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *rotateUserAPIKeyInput) (*apiKeyRevealOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		if in.TenantID == "" || in.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID and userID are required")
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.UserID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		plaintext, err := d.APIKeySvc.Reveal(ctx, in.APIKeyID, in.TenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyRevealOutput{}
		out.Body.PlaintextKey = plaintext
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-rotate-user-api-key",
		Method:      http.MethodPost,
		Path:        "/api/v1/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}/rotate",
		Summary:     "轮换用户 API key",
		Description: "平台代管视图：为用户 API key 生成新密钥并失效旧密钥缓存。响应返回新密钥明文，后续也可再次复制。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *rotateUserAPIKeyInput) (*apiKeyCreatedOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		if in.TenantID == "" || in.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID and userID are required")
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.UserID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		created, err := d.APIKeySvc.Rotate(ctx, in.APIKeyID, in.TenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &apiKeyCreatedOutput{}
		out.Body.PlaintextKey = created.PlaintextKey
		out.Body.Key = apiKeyToDTO(created.Key, nil)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-user-api-key",
		Method:      http.MethodDelete,
		Path:        "/api/v1/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}",
		Summary:     "删除用户 API key",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, in *deleteUserAPIKeyInput) (*deleteAPIKeyOutput, error) {
		if d.APIKeySvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key service is not configured")
		}
		if in.TenantID == "" || in.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenantID and userID are required")
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeySvc, in.TenantID, in.UserID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := d.APIKeySvc.Delete(ctx, in.APIKeyID, in.TenantID); err != nil {
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

func registerTenantSelfAPIKeys(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-api-keys",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenant-api-keys",
		Summary:     "租户自助 API key 列表",
		Description: "按当前租户 token 查询租户 API key 列表，含唯一绑定分组、额度和独立限流摘要。",
		Tags:        []string{"api-keys"},
	}, func(ctx context.Context, _ *struct{}) (*apiKeysOutput, error) {
		if d.APIKeySvc == nil || d.CommercialSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("api key or commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		keys, err := d.APIKeySvc.ListForTenant(ctx, tenantID)
		return apiKeysResponse(ctx, d.CommercialSvc, keys, err)
	})

}

func tenantAPIKeyCreateInput(tenantID string, req apiKeyWriteRequest) (identitycontrol.CreateInput, error) {
	if err := validateAPIKeyWriteRequest(req); err != nil {
		return identitycontrol.CreateInput{}, err
	}
	expiresAt, err := apiKeyExpiresAt(req.ExpiresAt)
	if err != nil {
		return identitycontrol.CreateInput{}, err
	}
	status, err := apiKeyStatusOrDefault(req.Status)
	if err != nil {
		return identitycontrol.CreateInput{}, err
	}
	return identitycontrol.CreateInput{
		OwnerScope:         coreidentity.ScopeTenant,
		TenantID:           tenantID,
		GroupID:            strings.TrimSpace(req.GroupID),
		Name:               strings.TrimSpace(req.Name),
		QuotaLimitMicroUSD: req.QuotaLimitMicroUSD,
		Status:             status,
		ExpiresAt:          expiresAt,
		CreatedBy:          strings.TrimSpace(req.CreatedBy),
	}, nil
}

func userAPIKeyCreateInput(tenantID, userID string, req apiKeyWriteRequest) (identitycontrol.CreateInput, error) {
	write, err := tenantAPIKeyCreateInput(tenantID, req)
	if err != nil {
		return identitycontrol.CreateInput{}, err
	}
	write.OwnerScope = coreidentity.ScopeUser
	write.UserID = userID
	return write, nil
}

func apiKeyUpdateInput(tenantID, apiKeyID string, req apiKeyWriteRequest) (identitycontrol.UpdateInput, error) {
	if err := validateAPIKeyWriteRequest(req); err != nil {
		return identitycontrol.UpdateInput{}, err
	}
	expiresAt, err := apiKeyExpiresAt(req.ExpiresAt)
	if err != nil {
		return identitycontrol.UpdateInput{}, err
	}
	status, err := apiKeyStatusOrDefault(req.Status)
	if err != nil {
		return identitycontrol.UpdateInput{}, err
	}
	return identitycontrol.UpdateInput{
		ID:                 apiKeyID,
		TenantID:           tenantID,
		GroupID:            strings.TrimSpace(req.GroupID),
		Name:               strings.TrimSpace(req.Name),
		QuotaLimitMicroUSD: req.QuotaLimitMicroUSD,
		Status:             status,
		ExpiresAt:          expiresAt,
	}, nil
}

func validateAPIKeyWriteRequest(req apiKeyWriteRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return domain.NewValidationError("name", "name is required")
	}
	if strings.TrimSpace(req.GroupID) == "" {
		return domain.NewValidationError("group_id", "group_id is required")
	}
	if req.QuotaLimitMicroUSD != nil {
		if *req.QuotaLimitMicroUSD < 0 {
			return domain.NewValidationError("quota_limit_micro_usd", "quota_limit_micro_usd must be non-negative")
		}
	}
	return nil
}

func apiKeyStatusInput(status string) (string, error) {
	status = strings.TrimSpace(status)
	switch status {
	case "active", "disabled":
		return status, nil
	default:
		return "", domain.NewValidationError("status", "status must be active or disabled")
	}
}

func apiKeyStatusOrDefault(status string) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return domain.APIKeyStatusActive, nil
	}
	return apiKeyStatusInput(status)
}

func apiKeyExpiresAt(ms *int64) (*time.Time, error) {
	if ms == nil || *ms == 0 {
		return nil, nil
	}
	if *ms < 0 {
		return nil, domain.NewValidationError("expires_at", "expires_at must be a positive Unix millisecond timestamp")
	}
	t := time.UnixMilli(*ms).UTC()
	return &t, nil
}

func ensureTenantAPIKeyScope(ctx context.Context, svc *identitycontrol.Service, tenantID, apiKeyID string) error {
	keys, err := svc.ListForTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	return ensureAPIKeyInList(keys, apiKeyID)
}

func ensureUserAPIKeyScope(ctx context.Context, svc *identitycontrol.Service, tenantID, userID, apiKeyID string) error {
	keys, err := svc.ListForUser(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	return ensureAPIKeyInList(keys, apiKeyID)
}

func ensureAPIKeyInList(keys []coreidentity.APIKey, apiKeyID string) error {
	for _, key := range keys {
		if key.ID == apiKeyID {
			return nil
		}
	}
	return domain.ErrNotFound
}

func ensureAPIKeyGroupAccessible(ctx context.Context, svc *commercial.Service, tenantID, userID, groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return domain.NewValidationError("group_id", "group_id is required")
	}
	groups, err := svc.ResolveAccessibleGroups(ctx, coreidentity.Subject{TenantID: tenantID, UserID: userID})
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.Group.ID == groupID {
			return nil
		}
	}
	return domain.NewValidationError("group_id", "group is not active or accessible to the key owner")
}

func apiKeysResponse(ctx context.Context, svc *commercial.Service, keys []coreidentity.APIKey, err error) (*apiKeysOutput, error) {
	if err != nil {
		return nil, mapServiceError(err)
	}
	limitMap, lerr := loadAPIKeyLimitPolicyMap(ctx, svc, keys)
	if lerr != nil {
		return nil, mapServiceError(lerr)
	}
	out := &apiKeysOutput{}
	out.Body.Items = make([]apiKeyDTO, 0, len(keys))
	for _, key := range keys {
		out.Body.Items = append(out.Body.Items, apiKeyToDTO(key, limitMap[key.ID]))
	}
	out.Body.Total = len(out.Body.Items)
	return out, nil
}

func apiKeyToDTO(key coreidentity.APIKey, limit *commercial.LimitPolicy) apiKeyDTO {
	var limitDTO *runtimeLimitPolicyDTO
	if limit != nil {
		dto := runtimeLimitPolicyToDTO(*limit)
		limitDTO = &dto
	}
	return apiKeyDTO{
		ID:                 key.ID,
		OwnerType:          apiKeyOwnerType(key.OwnerScope),
		TenantID:           key.TenantID,
		UserID:             stringPtrOrNil(key.UserID),
		GroupID:            key.GroupID,
		LastFour:           stringPtrOrNil(key.LastFour),
		Name:               key.Name,
		QuotaLimitMicroUSD: cloneOptionalInt64(key.QuotaLimitMicro),
		QuotaUsedMicroUSD:  key.QuotaUsedMicro,
		Status:             key.Status,
		ExpiresAt:          timePtrToMillis(key.ExpiresAt),
		LastUsedAt:         timePtrToMillis(key.LastUsedAt),
		LimitPolicy:        limitDTO,
		CreatedBy:          stringPtrOrNil(key.CreatedBy),
		CreatedAt:          timeToMillisPtr(key.CreatedAt),
		UpdatedAt:          timeToMillisPtr(key.UpdatedAt),
	}
}

func loadAPIKeyLimitPolicyMap(ctx context.Context, svc *commercial.Service, keys []coreidentity.APIKey) (map[string]*commercial.LimitPolicy, error) {
	out := make(map[string]*commercial.LimitPolicy, len(keys))
	if svc == nil || len(keys) == 0 {
		return out, nil
	}
	policies, err := svc.ListLimitPolicies(ctx, commercial.LimitPolicyFilter{ScopeType: commercial.LimitScopeAPIKey})
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key.ID] = struct{}{}
	}
	for i := range policies {
		policy := policies[i]
		if _, ok := allowed[policy.ScopeID]; !ok {
			continue
		}
		out[policy.ScopeID] = &policy
	}
	return out, nil
}

func syncAPIKeyLimitPolicy(
	ctx context.Context,
	svc *commercial.Service,
	apiKeyID string,
	req *scopedLimitPolicyWriteRequest,
	createdBy string,
) (*commercial.LimitPolicy, error) {
	if svc == nil {
		if req == nil {
			return nil, nil
		}
		return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
	}
	if req == nil {
		return nil, nil
	}
	existing, err := svc.ListLimitPolicies(ctx, commercial.LimitPolicyFilter{
		ScopeType: commercial.LimitScopeAPIKey,
		ScopeID:   apiKeyID,
	})
	if err != nil {
		return nil, err
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = string(commercial.StatusActive)
	}
	write := commercial.LimitPolicyWrite{
		ScopeType:        commercial.LimitScopeAPIKey,
		ScopeID:          apiKeyID,
		ConcurrencyLimit: int32PtrToIntPtr(req.ConcurrencyLimit),
		Status:           commercial.Status(status),
		CreatedBy:        createdBy,
	}
	if len(existing) > 0 {
		policy, err := svc.UpdateLimitPolicy(ctx, existing[0].ID, write)
		if err != nil {
			return nil, err
		}
		return &policy, nil
	}
	policy, err := svc.CreateLimitPolicy(ctx, write)
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func apiKeyOwnerType(scope coreidentity.Scope) string {
	switch scope {
	case coreidentity.ScopeUser:
		return "user"
	case coreidentity.ScopeTenant:
		return "tenant"
	default:
		return ""
	}
}

func cloneOptionalInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func timePtrToMillis(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.UnixMilli()
	return &v
}

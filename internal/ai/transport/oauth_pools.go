package transport

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/libs/go/httpx"
)

type credentialPoolDTO struct {
	ID                string  `json:"id" doc:"凭证池 ID"`
	Name              string  `json:"name" doc:"凭证池名称"`
	TenantDisplayName string  `json:"tenant_display_name" doc:"租户侧展示名称"`
	TenantAccessMode  string  `json:"tenant_access_mode" enum:"public,restricted" doc:"租户访问范围"`
	FixedProviderType string  `json:"fixed_provider_type" enum:"codex,claude_oauth,gemini_cli,antigravity" doc:"固定供应商类型"`
	OAuthStrategy     string  `json:"oauth_strategy" enum:"round_robin,weighted" doc:"OAuth 凭证选择策略"`
	Notes             string  `json:"notes,omitempty" doc:"备注"`
	Status            string  `json:"status" enum:"active,disabled" doc:"状态"`
	PriceBookID       string  `json:"price_book_id,omitempty"`
	TenantMultiplier  float64 `json:"tenant_multiplier"`
	CreatedAt         *int64  `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt         *int64  `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type credentialPoolsOutput struct {
	Body struct {
		Items []credentialPoolDTO `json:"items"`
		Total int                 `json:"total"`
	}
}

type createCredentialPoolInput struct {
	Body credentialPoolWriteRequest
}

type updateCredentialPoolInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	Body   credentialPoolWriteRequest
}

type updateCredentialPoolStatusInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	Body   struct {
		Status string `json:"status" enum:"active,disabled" doc:"状态"`
	}
}

type deleteCredentialPoolInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
}

type credentialPoolWriteRequest struct {
	Name              string   `json:"name" doc:"凭证池名称"`
	TenantDisplayName string   `json:"tenant_display_name,omitempty" doc:"租户侧展示名称；为空时使用凭证池名称"`
	TenantAccessMode  string   `json:"tenant_access_mode,omitempty" enum:"public,restricted" doc:"租户访问范围；为空默认 public"`
	FixedProviderType string   `json:"fixed_provider_type,omitempty" enum:"codex,claude_oauth,gemini_cli,antigravity" doc:"固定供应商类型；创建必填，更新时忽略"`
	OAuthStrategy     string   `json:"oauth_strategy,omitempty" enum:"round_robin,weighted" doc:"OAuth 凭证选择策略；为空默认/保留 round_robin"`
	Notes             string   `json:"notes,omitempty" doc:"备注"`
	PriceBookID       string   `json:"price_book_id,omitempty"`
	TenantMultiplier  *float64 `json:"tenant_multiplier,omitempty"`
}

type credentialPoolOutput struct {
	Body credentialPoolDTO
}

type deleteCredentialPoolOutput struct {
	Body struct {
		Deleted bool `json:"deleted" doc:"是否已删除"`
	}
}

type poolCredentialsInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
}

type createPoolCredentialInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	Body   poolCredentialWriteRequest
}

type updatePoolCredentialInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	CredID string `path:"credID" doc:"凭证 ID"`
	Body   poolCredentialPatchRequest
}

type deletePoolCredentialInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	CredID string `path:"credID" doc:"凭证 ID"`
}

type refreshPoolCredentialInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	CredID string `path:"credID" doc:"凭证 ID"`
}

type poolCredentialDTO struct {
	ID                   string         `json:"id" doc:"凭证 ID"`
	PoolID               string         `json:"pool_id" doc:"凭证池 ID"`
	Name                 string         `json:"name" doc:"凭证名称"`
	ProviderType         string         `json:"provider_type" enum:"codex,claude_oauth,gemini_cli,antigravity" doc:"供应商类型"`
	Email                string         `json:"email" doc:"账号邮箱"`
	TokenType            string         `json:"token_type" doc:"Token 类型"`
	Scope                string         `json:"scope" doc:"授权范围"`
	ExpiresAt            *int64         `json:"expires_at,omitempty" doc:"过期时间，Unix 毫秒"`
	AuthMetadata         map[string]any `json:"auth_metadata,omitempty" doc:"允许公开的账户身份元数据；仅包含已知 ID 和套餐字段"`
	Weight               int            `json:"weight" doc:"权重"`
	Status               string         `json:"status" enum:"active,invalid,disabled" doc:"状态"`
	InvalidReason        *string        `json:"invalid_reason,omitempty" doc:"失效原因"`
	CooldownUntil        *int64         `json:"cooldown_until,omitempty" doc:"临时冷却截止时间，Unix 毫秒"`
	LastUsedAt           *int64         `json:"last_used_at,omitempty" doc:"最后使用时间，Unix 毫秒"`
	LastRefreshedAt      *int64         `json:"last_refreshed_at,omitempty" doc:"最后刷新时间，Unix 毫秒"`
	LastFailedAt         *int64         `json:"last_failed_at,omitempty" doc:"最后失败时间，Unix 毫秒"`
	ConsecutiveFailCount int            `json:"consecutive_fail_count" doc:"连续失败次数"`
	SuccessCount         int64          `json:"success_count" doc:"成功次数"`
	FailCount            int64          `json:"fail_count" doc:"失败次数"`
	CreatedAt            *int64         `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt            *int64         `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type poolCredentialsOutput struct {
	Body struct {
		Items []poolCredentialDTO `json:"items"`
		Total int                 `json:"total"`
	}
}

type poolCredentialWriteRequest struct {
	Name          string         `json:"name,omitempty" doc:"凭证展示名；为空时依次回退 email/provider_type"`
	ProviderType  string         `json:"provider_type,omitempty" enum:"codex,claude_oauth,gemini_cli,antigravity" doc:"供应商类型；为空时继承凭证池 fixed_provider_type"`
	Email         string         `json:"email,omitempty" doc:"账号邮箱"`
	AccessToken   string         `json:"access_token" doc:"OAuth access token；只写不读"`
	RefreshToken  string         `json:"refresh_token,omitempty" doc:"OAuth refresh token；只写不读"`
	TokenType     string         `json:"token_type,omitempty" doc:"Token 类型；为空默认 bearer"`
	Scope         string         `json:"scope,omitempty" doc:"授权范围"`
	ExpiresAt     *int64         `json:"expires_at,omitempty" doc:"过期时间，Unix 秒；兼容 Codex OAuth 导出"`
	Weight        *int           `json:"weight,omitempty" minimum:"0" doc:"权重；为空或 0 时默认 100"`
	AuthMetadata  map[string]any `json:"auth_metadata,omitempty" doc:"运行时账户元数据；响应仅返回已知的非敏感账户身份字段"`
	AccountID     string         `json:"account_id,omitempty" doc:"Codex 导出 account_id，会合并到 auth_metadata"`
	PlanType      string         `json:"plan_type,omitempty" doc:"Codex 导出 plan_type，会合并到 auth_metadata"`
	UserID        string         `json:"user_id,omitempty" doc:"Codex 导出 user_id，会合并到 auth_metadata"`
	AccountUserID string         `json:"account_user_id,omitempty" doc:"Codex 导出 account_user_id，会合并到 auth_metadata"`
}

type poolCredentialPatchRequest struct {
	Status *string `json:"status,omitempty" enum:"active,disabled" doc:"凭证管理状态；仅允许 active/disabled"`
	Weight *int    `json:"weight,omitempty" minimum:"0" doc:"权重；0 表示保留为 0"`
}

type poolCredentialOutput struct {
	Body poolCredentialDTO
}

type deletePoolCredentialOutput struct {
	Body struct {
		Deleted bool `json:"deleted" doc:"是否已删除"`
	}
}

type poolAvailableModelsInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
}

type poolAvailableModelsDTO struct {
	PoolID            string   `json:"pool_id" doc:"凭证池 ID"`
	FixedProviderType string   `json:"fixed_provider_type" enum:"codex,claude_oauth,gemini_cli,antigravity" doc:"固定供应商类型"`
	Models            []string `json:"models" doc:"当前可用上游模型"`
	Source            string   `json:"source" enum:"live,cache,stale,fallback" doc:"模型来源"`
	ProfileRevision   string   `json:"profile_revision" doc:"发现模型时使用的客户端 Profile 或 fallback 版本"`
	ObservedAt        int64    `json:"observed_at" doc:"模型目录观测时间，Unix 毫秒"`
}

type poolAvailableModelsOutput struct {
	Body poolAvailableModelsDTO
}

type importPoolAvailableModelsInput struct {
	PoolID string `path:"poolID" doc:"凭证池 ID"`
	Body   struct {
		Models []string `json:"models" doc:"待导入的对外 model_code 列表"`
	}
}

type importPoolAvailableModelsOutput struct {
	Body struct {
		Created []string `json:"created" doc:"新创建显式上游模型绑定的 model_code"`
		Skipped []string `json:"skipped" doc:"已存在而跳过的 model_code"`
	}
}

type oauthPoolHealthDTO struct {
	PoolID            string `json:"pool_id" doc:"凭证池 ID"`
	PoolName          string `json:"pool_name" doc:"凭证池名称"`
	FixedProviderType string `json:"fixed_provider_type" enum:"codex,claude_oauth,gemini_cli,antigravity" doc:"固定供应商类型"`
	OAuthStrategy     string `json:"oauth_strategy" enum:"round_robin,weighted" doc:"OAuth 凭证选择策略"`
	Total             int    `json:"total" doc:"凭证总数"`
	Active            int    `json:"active" doc:"active 凭证数"`
	Invalid           int    `json:"invalid" doc:"invalid 凭证数"`
	Disabled          int    `json:"disabled" doc:"disabled 凭证数"`
	CoolingDown       int    `json:"cooling_down" doc:"当前处于临时冷却的 active 凭证数"`
	ExpiringSoon      int    `json:"expiring_soon" doc:"1 小时内过期凭证数"`
}

type oauthPoolHealthOutput struct {
	Body struct {
		Items []oauthPoolHealthDTO `json:"items"`
		Total int                  `json:"total"`
	}
}

func registerOAuthPools(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-credential-pools",
		Method:      http.MethodGet,
		Path:        "/api/v1/credential-pools",
		Summary:     "OAuth 凭证池列表",
		Description: "返回固定供应商 OAuth 凭证池列表。本端点为只读契约，不返回任何 token、密文或 provider API key。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, _ *struct{}) (*credentialPoolsOutput, error) {
		if d.PoolReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential pool reader is not configured")
		}
		pools, err := d.PoolReader.ListPools(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &credentialPoolsOutput{}
		out.Body.Items = make([]credentialPoolDTO, 0, len(pools))
		for _, pool := range pools {
			out.Body.Items = append(out.Body.Items, credentialPoolToDTO(pool))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-create-credential-pool",
		Method:      http.MethodPost,
		Path:        "/api/v1/credential-pools",
		Summary:     "创建 OAuth 凭证池",
		Description: "创建固定供应商 OAuth 凭证池；可服务模型和上游协议由显式上游模型绑定维护。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *createCredentialPoolInput) (*credentialPoolOutput, error) {
		if d.PoolReader == nil || d.PoolWriter == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential pool reader or writer is not configured")
		}
		write, err := credentialPoolCreateInput(in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		id, err := d.PoolWriter.CreatePool(ctx, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		pool, err := d.PoolReader.GetPool(ctx, id)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &credentialPoolOutput{}
		out.Body = credentialPoolToDTO(*pool)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-credential-pool",
		Method:      http.MethodPatch,
		Path:        "/api/v1/credential-pools/{poolID}",
		Summary:     "更新 OAuth 凭证池",
		Description: "更新凭证池可变字段；fixed_provider_type 创建后不可变。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *updateCredentialPoolInput) (*credentialPoolOutput, error) {
		if d.PoolReader == nil || d.PoolWriter == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential pool reader or writer is not configured")
		}
		current, err := d.PoolReader.GetPool(ctx, in.PoolID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		write, err := credentialPoolUpdateInput(*current, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := d.PoolWriter.UpdatePool(ctx, in.PoolID, write); err != nil {
			return nil, mapServiceError(err)
		}
		updated, err := d.PoolReader.GetPool(ctx, in.PoolID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &credentialPoolOutput{}
		out.Body = credentialPoolToDTO(*updated)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-credential-pool-status",
		Method:      http.MethodPatch,
		Path:        "/api/v1/credential-pools/{poolID}/status",
		Summary:     "更新 OAuth 凭证池状态",
		Description: "独立启用或停用凭证池；新增凭证池默认停用。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *updateCredentialPoolStatusInput) (*credentialPoolOutput, error) {
		if d.PoolReader == nil || d.PoolWriter == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential pool reader or writer is not configured")
		}
		status, err := validateCredentialPoolStatus(in.Body.Status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if err := d.PoolWriter.UpdatePoolStatus(ctx, in.PoolID, status); err != nil {
			return nil, mapServiceError(err)
		}
		updated, err := d.PoolReader.GetPool(ctx, in.PoolID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &credentialPoolOutput{Body: credentialPoolToDTO(*updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-credential-pool",
		Method:      http.MethodDelete,
		Path:        "/api/v1/credential-pools/{poolID}",
		Summary:     "删除 OAuth 凭证池",
		Description: "删除凭证池会级联删除池内 OAuth credential；若存在上游部署引用，数据库会拒绝删除。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *deleteCredentialPoolInput) (*deleteCredentialPoolOutput, error) {
		if d.PoolWriter == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential pool writer is not configured")
		}
		if err := d.PoolWriter.DeletePool(ctx, in.PoolID); err != nil {
			return nil, mapServiceError(err)
		}
		out := &deleteCredentialPoolOutput{}
		out.Body.Deleted = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-pool-credentials",
		Method:      http.MethodGet,
		Path:        "/api/v1/credential-pools/{poolID}/credentials",
		Summary:     "OAuth 凭证池账号列表",
		Description: "返回指定 OAuth 凭证池内账号的非敏感展示信息与健康指标；不返回 access token、refresh token、密文或 key hash。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *poolCredentialsInput) (*poolCredentialsOutput, error) {
		if d.CredentialReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential store is not configured")
		}
		if in.PoolID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("poolID is required")
		}
		rows, err := d.CredentialReader.ListForPool(ctx, in.PoolID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &poolCredentialsOutput{}
		out.Body.Items = make([]poolCredentialDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, poolCredentialSummaryToDTO(row))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-import-pool-credential",
		Method:      http.MethodPost,
		Path:        "/api/v1/credential-pools/{poolID}/credentials",
		Summary:     "导入 OAuth 凭证",
		Description: "向指定凭证池导入 OAuth credential。响应只返回非敏感展示字段，不返回 access token、refresh token 或密文。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *createPoolCredentialInput) (*poolCredentialOutput, error) {
		if d.CredentialCreator == nil || d.CredentialReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential creator or reader is not configured")
		}
		if strings.TrimSpace(in.Body.ProviderType) == "" && d.PoolReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential pool reader is not configured")
		}
		row, err := importPoolCredential(ctx, d, in.PoolID, in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &poolCredentialOutput{}
		out.Body = poolCredentialSummaryToDTO(*row)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-pool-credential",
		Method:      http.MethodPatch,
		Path:        "/api/v1/credential-pools/{poolID}/credentials/{credID}",
		Summary:     "更新 OAuth 凭证状态/权重",
		Description: "管理侧只允许在 active/disabled 间切换状态；invalid 由运行时失败路径维护。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *updatePoolCredentialInput) (*poolCredentialOutput, error) {
		if d.CredentialWriter == nil || d.CredentialReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential store is not configured")
		}
		if _, err := getPoolCredentialScoped(ctx, d.CredentialReader, in.PoolID, in.CredID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := validatePoolCredentialPatch(in.Body); err != nil {
			return nil, mapServiceError(err)
		}
		if in.Body.Status != nil {
			if err := d.CredentialWriter.UpdateStatus(ctx, in.CredID, strings.TrimSpace(*in.Body.Status)); err != nil {
				return nil, mapServiceError(err)
			}
		}
		if in.Body.Weight != nil {
			if err := d.CredentialWriter.UpdateWeight(ctx, in.CredID, *in.Body.Weight); err != nil {
				return nil, mapServiceError(err)
			}
		}
		row, err := getPoolCredentialScoped(ctx, d.CredentialReader, in.PoolID, in.CredID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &poolCredentialOutput{}
		out.Body = poolCredentialSummaryToDTO(*row)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-refresh-pool-credential",
		Method:      http.MethodPost,
		Path:        "/api/v1/credential-pools/{poolID}/credentials/{credID}/refresh",
		Summary:     "刷新 OAuth 凭证令牌",
		Description: "调用 token refresher 刷新指定凭证，返回刷新后的凭证（不含 token 明文）。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *refreshPoolCredentialInput) (*poolCredentialOutput, error) {
		if d.CredentialReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential store is not configured")
		}
		if d.TokenRefresher == nil {
			return nil, httpx.ErrUnavailable.WithDetail("token refresher is not configured")
		}
		if _, err := getPoolCredentialScoped(ctx, d.CredentialReader, in.PoolID, in.CredID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := d.TokenRefresher.RefreshByID(ctx, in.CredID); err != nil {
			return nil, httpx.ErrBadRequest.WithDetail("token refresh failed: " + err.Error())
		}
		row, err := getPoolCredentialScoped(ctx, d.CredentialReader, in.PoolID, in.CredID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &poolCredentialOutput{}
		out.Body = poolCredentialSummaryToDTO(*row)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-pool-credential",
		Method:      http.MethodDelete,
		Path:        "/api/v1/credential-pools/{poolID}/credentials/{credID}",
		Summary:     "删除 OAuth 凭证",
		Description: "永久删除指定凭证池内的 OAuth credential。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *deletePoolCredentialInput) (*deletePoolCredentialOutput, error) {
		if d.CredentialWriter == nil || d.CredentialReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential store is not configured")
		}
		if _, err := getPoolCredentialScoped(ctx, d.CredentialReader, in.PoolID, in.CredID); err != nil {
			return nil, mapServiceError(err)
		}
		if err := d.CredentialWriter.Delete(ctx, in.CredID); err != nil {
			return nil, mapServiceError(err)
		}
		out := &deletePoolCredentialOutput{}
		out.Body.Deleted = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-pool-available-models",
		Method:      http.MethodGet,
		Path:        "/api/v1/credential-pools/{poolID}/available-models",
		Summary:     "OAuth 凭证池可用模型",
		Description: "优先通过版本化客户端 Profile 在线发现模型，并以缓存、最后成功快照或内置目录降级。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, in *poolAvailableModelsInput) (*poolAvailableModelsOutput, error) {
		if d.PoolReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential pool reader is not configured")
		}
		if in.PoolID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("poolID is required")
		}
		pool, err := d.PoolReader.GetPool(ctx, in.PoolID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		catalog := resolvePoolModelCatalog(ctx, d, *pool)
		out := &poolAvailableModelsOutput{}
		out.Body = poolAvailableModelsDTO{
			PoolID:            pool.ID,
			FixedProviderType: string(pool.FixedProviderType),
			Models:            modelCardIDs(catalog.Models),
			Source:            catalog.Source,
			ProfileRevision:   catalog.ProfileRevision,
			ObservedAt:        catalog.ObservedAt.UnixMilli(),
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "ai-import-pool-available-models",
		Method:        http.MethodPost,
		Path:          "/api/v1/credential-pools/{poolID}/import-available-models",
		Summary:       "导入凭证池可用模型到显式绑定",
		Description:   "把当前在线目录、缓存快照或版本化 fallback 中选中的模型创建为显式上游模型绑定（去重幂等）。",
		Tags:          []string{"credential-pools"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *importPoolAvailableModelsInput) (*importPoolAvailableModelsOutput, error) {
		if d.PoolReader == nil || d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth credential pool reader or database is not configured")
		}
		if in.PoolID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("poolID is required")
		}
		pool, err := d.PoolReader.GetPool(ctx, in.PoolID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if len(in.Body.Models) == 0 {
			return nil, httpx.ErrBadRequest.WithDetail("models list is empty")
		}

		catalog := resolvePoolModelCatalog(ctx, d, *pool)
		allowedModels := make(map[string]struct{})
		for _, modelCode := range modelCardIDs(catalog.Models) {
			allowedModels[modelCode] = struct{}{}
		}

		tx, err := d.Postgres.Begin(ctx)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("begin tx failed: " + err.Error())
		}
		defer tx.Rollback(ctx)

		existingRows, err := tx.Query(ctx, `
			SELECT model_code
			FROM ai_upstream_models
			WHERE upstream_kind = 'oauth_pool' AND upstream_id = $1::uuid
		`, in.PoolID)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("load upstream model bindings failed: " + err.Error())
		}
		existing := make(map[string]struct{})
		for existingRows.Next() {
			var code string
			if err := existingRows.Scan(&code); err != nil {
				existingRows.Close()
				return nil, httpx.ErrInternal.WithDetail("scan upstream model bindings failed: " + err.Error())
			}
			existing[code] = struct{}{}
		}
		existingRows.Close()
		if err := existingRows.Err(); err != nil {
			return nil, httpx.ErrInternal.WithDetail("iterate upstream model bindings failed: " + err.Error())
		}

		endpointProtocol := fixedProviderEndpointProtocol(pool.FixedProviderType)
		out := &importPoolAvailableModelsOutput{}
		out.Body.Created = []string{}
		out.Body.Skipped = []string{}

		for _, modelCode := range in.Body.Models {
			modelCode = strings.TrimSpace(modelCode)
			if modelCode == "" {
				continue
			}
			if _, ok := allowedModels[modelCode]; !ok {
				return nil, httpx.ErrBadRequest.WithDetail("model is not present in the current pool catalog: " + modelCode)
			}
			if _, ok := existing[modelCode]; ok {
				out.Body.Skipped = append(out.Body.Skipped, modelCode)
				continue
			}
			capType, proto := inferCapabilityAndProtocol(modelCode, endpointProtocol)
			if _, err := tx.Exec(ctx, `
			INSERT INTO ai_upstream_models (
				upstream_kind,
				upstream_id,
				model_code,
				capability_type,
				api_format,
				upstream_model_name,
				status
			) VALUES ('oauth_pool', $1::uuid, $2, $3, $4, $5, 'active')
		`, in.PoolID, modelCode, capType, proto, modelCode); err != nil {
				return nil, httpx.ErrInternal.WithDetail("insert upstream model binding failed: " + err.Error())
			}
			existing[modelCode] = struct{}{}
			out.Body.Created = append(out.Body.Created, modelCode)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, httpx.ErrInternal.WithDetail("commit tx failed: " + err.Error())
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-oauth-pool-health",
		Method:      http.MethodGet,
		Path:        "/api/v1/oauth-pool-health",
		Summary:     "OAuth 凭证池健康汇总",
		Description: "返回每个 OAuth 凭证池的账号状态聚合指标。",
		Tags:        []string{"credential-pools"},
	}, func(ctx context.Context, _ *struct{}) (*oauthPoolHealthOutput, error) {
		if d.PoolHealthReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("oauth pool health reader is not configured")
		}
		rows, err := d.PoolHealthReader.GetPoolHealthSummary(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &oauthPoolHealthOutput{}
		out.Body.Items = make([]oauthPoolHealthDTO, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, oauthPoolHealthToDTO(row))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

func credentialPoolToDTO(pool domain.CredentialPool) credentialPoolDTO {
	return credentialPoolDTO{
		ID:                pool.ID,
		Name:              pool.Name,
		TenantDisplayName: pool.TenantDisplayName,
		TenantAccessMode:  pool.TenantAccessMode,
		FixedProviderType: string(pool.FixedProviderType),
		OAuthStrategy:     pool.OAuthStrategy,
		Notes:             pool.Notes,
		Status:            pool.Status,
		PriceBookID:       pool.PriceBookID,
		TenantMultiplier:  pool.TenantMultiplier,
		CreatedAt:         timeToMillisPtr(pool.CreatedAt),
		UpdatedAt:         timeToMillisPtr(pool.UpdatedAt),
	}
}

func credentialPoolCreateInput(req credentialPoolWriteRequest) (domain.CredentialPoolCreate, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return domain.CredentialPoolCreate{}, domain.NewValidationError("name", "name is required")
	}
	fixedType := strings.TrimSpace(req.FixedProviderType)
	if fixedType == "" {
		return domain.CredentialPoolCreate{}, domain.NewValidationError("fixed_provider_type", "fixed_provider_type is required")
	}
	if err := validateFixedProviderType(fixedType); err != nil {
		return domain.CredentialPoolCreate{}, err
	}
	strategy, err := credentialPoolStrategyOrDefault(req.OAuthStrategy)
	if err != nil {
		return domain.CredentialPoolCreate{}, err
	}
	if req.TenantMultiplier != nil && *req.TenantMultiplier < 0 {
		return domain.CredentialPoolCreate{}, domain.NewValidationError("tenant_multiplier", "tenant_multiplier must be >= 0")
	}
	accessMode, err := upstreamaccess.NormalizeMode(req.TenantAccessMode)
	if err != nil {
		return domain.CredentialPoolCreate{}, err
	}
	return domain.CredentialPoolCreate{
		Name:              name,
		TenantDisplayName: upstreamaccess.NormalizeDisplayName(name, req.TenantDisplayName),
		TenantAccessMode:  accessMode,
		FixedProviderType: domain.FixedProviderType(fixedType),
		OAuthStrategy:     strategy,
		Notes:             req.Notes,
		Status:            "disabled",
		PriceBookID:       strings.TrimSpace(req.PriceBookID),
		TenantMultiplier:  req.TenantMultiplier,
	}, nil
}

func credentialPoolUpdateInput(current domain.CredentialPool, req credentialPoolWriteRequest) (domain.CredentialPoolUpdate, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = current.Name
	}
	strategy := current.OAuthStrategy
	if strings.TrimSpace(req.OAuthStrategy) != "" {
		var err error
		strategy, err = credentialPoolStrategyOrDefault(req.OAuthStrategy)
		if err != nil {
			return domain.CredentialPoolUpdate{}, err
		}
	}
	priceBookID := current.PriceBookID
	if strings.TrimSpace(req.PriceBookID) != "" {
		priceBookID = strings.TrimSpace(req.PriceBookID)
	}
	multiplier := current.TenantMultiplier
	if req.TenantMultiplier != nil {
		if *req.TenantMultiplier < 0 {
			return domain.CredentialPoolUpdate{}, domain.NewValidationError("tenant_multiplier", "tenant_multiplier must be >= 0")
		}
		multiplier = *req.TenantMultiplier
	}
	accessMode := current.TenantAccessMode
	if strings.TrimSpace(req.TenantAccessMode) != "" {
		var err error
		accessMode, err = upstreamaccess.NormalizeMode(req.TenantAccessMode)
		if err != nil {
			return domain.CredentialPoolUpdate{}, err
		}
	}
	displayName := current.TenantDisplayName
	if strings.TrimSpace(req.TenantDisplayName) != "" {
		displayName = upstreamaccess.NormalizeDisplayName(name, req.TenantDisplayName)
	}
	return domain.CredentialPoolUpdate{
		Name:              name,
		TenantDisplayName: displayName,
		TenantAccessMode:  accessMode,
		OAuthStrategy:     strategy,
		Notes:             req.Notes,
		Status:            current.Status,
		PriceBookID:       priceBookID,
		TenantMultiplier:  &multiplier,
	}, nil
}

func fixedProviderEndpointProtocol(providerType domain.FixedProviderType) string {
	switch providerType {
	case domain.FixedProviderClaudeOAuth:
		return string(domain.EndpointProtocolAnthropic)
	case domain.FixedProviderGeminiCLI, domain.FixedProviderAntigravity:
		return string(domain.EndpointProtocolGemini)
	default:
		return string(domain.EndpointProtocolOpenAICompatible)
	}
}

func credentialPoolStrategyOrDefault(strategy string) (string, error) {
	if strings.TrimSpace(strategy) == "" {
		return "round_robin", nil
	}
	switch strategy {
	case "round_robin", "weighted":
		return strategy, nil
	default:
		return "", domain.NewValidationError("oauth_strategy", "oauth_strategy must be round_robin or weighted")
	}
}

func validateCredentialPoolStatus(status string) (string, error) {
	status = strings.TrimSpace(status)
	switch status {
	case "active", "disabled":
		return status, nil
	default:
		return "", domain.NewValidationError("status", "status must be active or disabled")
	}
}

func validateFixedProviderType(fixedType string) error {
	switch fixedType {
	case string(domain.FixedProviderCodex),
		string(domain.FixedProviderClaudeOAuth),
		string(domain.FixedProviderGeminiCLI),
		string(domain.FixedProviderAntigravity):
		return nil
	default:
		return domain.NewValidationError("fixed_provider_type", "fixed_provider_type must be codex, claude_oauth, gemini_cli, or antigravity")
	}
}

func poolCredentialCreateInput(providerType string, req poolCredentialWriteRequest) (domain.OAuthCredentialCreate, error) {
	providerType = strings.TrimSpace(providerType)
	if err := validateFixedProviderType(providerType); err != nil {
		return domain.OAuthCredentialCreate{}, err
	}
	accessToken := strings.TrimSpace(req.AccessToken)
	if accessToken == "" {
		return domain.OAuthCredentialCreate{}, domain.NewValidationError("access_token", "access_token is required")
	}
	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(req.Email)
	if name == "" {
		name = email
	}
	if name == "" {
		name = providerType
	}
	weight := 100
	if req.Weight != nil {
		if *req.Weight < 0 {
			return domain.OAuthCredentialCreate{}, domain.NewValidationError("weight", "weight must be >= 0")
		}
		if *req.Weight > 0 {
			weight = *req.Weight
		}
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0).UTC()
		expiresAt = &t
	}
	return domain.OAuthCredentialCreate{
		Name:         name,
		ProviderType: domain.FixedProviderType(providerType),
		Email:        email,
		AccessToken:  accessToken,
		RefreshToken: req.RefreshToken,
		TokenType:    strings.TrimSpace(req.TokenType),
		Scope:        strings.TrimSpace(req.Scope),
		ExpiresAt:    expiresAt,
		AuthMetadata: poolCredentialAuthMetadata(req),
		Weight:       weight,
	}, nil
}

func poolCredentialAuthMetadata(req poolCredentialWriteRequest) map[string]any {
	meta := make(map[string]any, len(req.AuthMetadata)+4)
	for key, value := range req.AuthMetadata {
		meta[key] = value
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
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func importPoolCredential(ctx context.Context, d AIDeps, poolID string, req poolCredentialWriteRequest) (*domain.OAuthCredentialSummary, error) {
	providerType := strings.TrimSpace(req.ProviderType)
	if providerType == "" {
		pool, err := d.PoolReader.GetPool(ctx, poolID)
		if err != nil {
			return nil, err
		}
		providerType = string(pool.FixedProviderType)
	}
	write, err := poolCredentialCreateInput(providerType, req)
	if err != nil {
		return nil, err
	}
	id, err := d.CredentialCreator.Create(ctx, poolID, write)
	if err != nil {
		return nil, err
	}
	return d.CredentialReader.GetSummaryByID(ctx, id)
}

func validatePoolCredentialPatch(req poolCredentialPatchRequest) error {
	if req.Status != nil {
		switch strings.TrimSpace(*req.Status) {
		case "active", "disabled":
		default:
			return domain.NewValidationError("status", "status must be active or disabled")
		}
	}
	if req.Weight != nil && *req.Weight < 0 {
		return domain.NewValidationError("weight", "weight must be >= 0")
	}
	return nil
}

func getPoolCredentialScoped(ctx context.Context, reader OAuthCredentialReader, poolID, credID string) (*domain.OAuthCredentialSummary, error) {
	row, err := reader.GetSummaryByID(ctx, credID)
	if err != nil {
		return nil, err
	}
	if row.PoolID != poolID {
		return nil, pgx.ErrNoRows
	}
	return row, nil
}

func poolCredentialSummaryToDTO(row domain.OAuthCredentialSummary) poolCredentialDTO {
	return poolCredentialDTO{
		ID:                   row.ID,
		PoolID:               row.PoolID,
		Name:                 row.Name,
		ProviderType:         row.ProviderType,
		Email:                row.Email,
		TokenType:            row.TokenType,
		Scope:                row.Scope,
		ExpiresAt:            timePtrToMillis(row.ExpiresAt),
		AuthMetadata:         domain.CredentialSummaryMetadata(row.AuthMetadata),
		Weight:               row.Weight,
		Status:               row.Status,
		InvalidReason:        stringPtrOrNil(row.InvalidReason),
		CooldownUntil:        timePtrToMillis(row.CooldownUntil),
		LastUsedAt:           timePtrToMillis(row.LastUsedAt),
		LastRefreshedAt:      timePtrToMillis(row.LastRefreshedAt),
		LastFailedAt:         timePtrToMillis(row.LastFailedAt),
		ConsecutiveFailCount: row.ConsecutiveFailCount,
		SuccessCount:         row.SuccessCount,
		FailCount:            row.FailCount,
		CreatedAt:            timeToMillisPtr(row.CreatedAt),
		UpdatedAt:            timeToMillisPtr(row.UpdatedAt),
	}
}

func oauthPoolHealthToDTO(row domain.OAuthPoolHealthSummary) oauthPoolHealthDTO {
	return oauthPoolHealthDTO{
		PoolID:            row.PoolID,
		PoolName:          row.PoolName,
		FixedProviderType: string(row.FixedProviderType),
		OAuthStrategy:     row.OAuthStrategy,
		Total:             row.Total,
		Active:            row.Active,
		Invalid:           row.Invalid,
		Disabled:          row.Disabled,
		CoolingDown:       row.CoolingDown,
		ExpiringSoon:      row.ExpiringSoon,
	}
}

func resolvePoolModelCatalog(ctx context.Context, d AIDeps, pool domain.CredentialPool) clientcatalog.Result {
	if d.ClientCatalog != nil {
		return d.ClientCatalog.Resolve(ctx, pool)
	}
	return clientcatalog.Result{
		Models:          clientcatalog.FallbackModels(pool.FixedProviderType),
		Source:          "fallback",
		ProfileRevision: clientcatalog.FallbackRevision,
		ObservedAt:      time.Now().UTC(),
	}
}

func modelCardIDs(models []clientruntime.ModelCard) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		if id := strings.TrimSpace(model.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

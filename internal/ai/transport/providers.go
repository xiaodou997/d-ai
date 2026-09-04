package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	"xiaodou/dai/libs/go/httpx"
)

// 上游账号（ai_upstream_accounts）：原 provider+endpoint 合并为顶级实体。

type accountDTO struct {
	ID                string               `json:"id" doc:"上游账号 ID"`
	Name              string               `json:"name" doc:"账号名称"`
	TenantDisplayName string               `json:"tenant_display_name" doc:"租户侧展示名称"`
	TenantAccessMode  string               `json:"tenant_access_mode" enum:"public,restricted" doc:"租户访问范围"`
	Endpoints         []accountEndpointDTO `json:"endpoints" doc:"账号支持的请求端点；同一 API 格式至多一个"`
	ConcurrencyLimit  *int32               `json:"concurrency_limit,omitempty" doc:"最大并发请求数；为空表示不限制"`
	PriceBookID       string               `json:"price_book_id,omitempty" doc:"租户结算价格表 ID"`
	TenantMultiplier  *float64             `json:"tenant_multiplier,omitempty" doc:"默认租户扣费倍率"`
	Status            string               `json:"status" enum:"active,invalid,disabled" doc:"状态：active=可用，invalid=凭据失效，disabled=管理员停用"`
	InvalidReason     string               `json:"invalid_reason,omitempty" doc:"系统判定失效的原因"`
	InvalidAt         *int64               `json:"invalid_at,omitempty" doc:"系统判定失效时间，Unix 毫秒"`

	CreatedAt *int64 `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt *int64 `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type accountEndpointDTO struct {
	ID            string          `json:"id" doc:"请求端点 ID"`
	AccountID     string          `json:"account_id" doc:"上游账号 ID"`
	APIFormat     string          `json:"api_format" enum:"openai_chat,openai_responses,openai_embeddings,openai_images,anthropic_messages,gemini_generate,gemini_embeddings" doc:"精确 API 格式"`
	BaseURL       string          `json:"base_url" doc:"端点基础 URL"`
	PathOverride  string          `json:"path_override,omitempty" doc:"自定义请求路径；为空使用格式默认路径"`
	AuthScheme    string          `json:"auth_scheme" enum:"format_default,bearer,anthropic_api_key,gemini_api_key,custom_header" doc:"API Key 注入方式"`
	AuthHeader    string          `json:"auth_header,omitempty" doc:"custom_header 使用的请求头名称"`
	ExtraHeaders  json.RawMessage `json:"extra_headers" doc:"端点附加请求头 JSON；敏感值返回 ***REDACTED***"`
	Status        string          `json:"status" enum:"active,disabled"`
	HealthStatus  string          `json:"health_status" enum:"unknown,healthy,unhealthy"`
	LastError     string          `json:"last_error,omitempty"`
	LastCheckedAt *int64          `json:"last_checked_at,omitempty" doc:"最后检查时间，Unix 毫秒"`
	CreatedAt     *int64          `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt     *int64          `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type accountEndpointWriteRequest struct {
	APIFormat    string          `json:"api_format" enum:"openai_chat,openai_responses,openai_embeddings,openai_images,anthropic_messages,gemini_generate,gemini_embeddings"`
	BaseURL      string          `json:"base_url"`
	PathOverride string          `json:"path_override,omitempty"`
	AuthScheme   string          `json:"auth_scheme,omitempty" enum:"format_default,bearer,anthropic_api_key,gemini_api_key,custom_header"`
	AuthHeader   string          `json:"auth_header,omitempty"`
	ExtraHeaders json.RawMessage `json:"extra_headers,omitempty" doc:"端点附加请求头；编辑时敏感字段值为 ***REDACTED*** 表示保留原值，删除字段表示删除请求头"`
	Status       string          `json:"status,omitempty" enum:"active,disabled"`
}

type accountsOutput struct {
	Body struct {
		Items []accountDTO `json:"items"`
		Total int          `json:"total"`
	}
}

type accountOutput struct {
	Body accountDTO
}

type createAccountRequest struct {
	Name              string                        `json:"name" doc:"账号名称"`
	TenantDisplayName string                        `json:"tenant_display_name,omitempty" doc:"租户侧展示名称；为空时使用账号名称"`
	TenantAccessMode  string                        `json:"tenant_access_mode,omitempty" enum:"public,restricted" doc:"租户访问范围；为空保留原值"`
	APIKey            string                        `json:"api_key" doc:"上游 API key 明文；仅写入时接收，响应不返回"`
	Endpoints         []accountEndpointWriteRequest `json:"endpoints" minItems:"1" doc:"账号支持的请求端点；同一 API 格式至多一个"`
	ConcurrencyLimit  *int32                        `json:"concurrency_limit,omitempty" minimum:"1" doc:"最大并发请求数；为空表示不限制"`
	PriceBookID       string                        `json:"price_book_id,omitempty" doc:"租户结算价格表 ID；为空不绑定"`
	TenantMultiplier  *float64                      `json:"tenant_multiplier,omitempty" doc:"默认租户扣费倍率；为空不设置"`
}

type updateAccountRequest struct {
	Name              string   `json:"name" doc:"账号名称"`
	TenantDisplayName string   `json:"tenant_display_name,omitempty" doc:"租户侧展示名称；为空时使用账号名称"`
	TenantAccessMode  string   `json:"tenant_access_mode,omitempty" enum:"public,restricted" doc:"租户访问范围；为空默认 public"`
	APIKey            string   `json:"api_key,omitempty" doc:"上游 API key 明文；为空保留原密文"`
	ConcurrencyLimit  *int32   `json:"concurrency_limit,omitempty" minimum:"1" doc:"最大并发请求数；为空表示不限制"`
	PriceBookID       string   `json:"price_book_id,omitempty" doc:"租户结算价格表 ID；为空不绑定"`
	TenantMultiplier  *float64 `json:"tenant_multiplier,omitempty" doc:"默认租户扣费倍率；为空不设置"`
}

type accountStatusRequest struct {
	Status string `json:"status" enum:"active,disabled" doc:"状态"`
}

type createAccountInput struct{ Body createAccountRequest }
type updateAccountInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	Body      updateAccountRequest
}
type updateAccountStatusInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	Body      accountStatusRequest
}
type deleteAccountInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
}
type deleteAccountOutput struct {
	Body struct {
		Deleted bool `json:"deleted" doc:"是否已删除"`
	}
}

type accountEndpointsInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
}
type accountEndpointInput struct {
	AccountID  string `path:"accountID" doc:"上游账号 ID"`
	EndpointID string `path:"endpointID" doc:"请求端点 ID"`
}
type createAccountEndpointInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	Body      accountEndpointWriteRequest
}
type updateAccountEndpointInput struct {
	AccountID  string `path:"accountID" doc:"上游账号 ID"`
	EndpointID string `path:"endpointID" doc:"请求端点 ID"`
	Body       accountEndpointWriteRequest
}
type accountEndpointsOutput struct {
	Body struct {
		Items []accountEndpointDTO `json:"items"`
		Total int                  `json:"total"`
	}
}
type accountEndpointOutput struct{ Body accountEndpointDTO }

func registerUpstreamAccounts(api huma.API, d UpstreamAccountManagementHTTPDeps) {
	registerUpstreamAccountTransfer(api, d)
	registerUpstreamAccountEndpoints(api, d)

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-upstream-accounts",
		Method:      http.MethodGet,
		Path:        "/api/v1/upstream-accounts",
		Summary:     "上游账号列表",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, _ *struct{}) (*accountsOutput, error) {
		if d.Accounts == nil {
			return nil, httpx.ErrUnavailable.WithDetail("account service is not configured")
		}
		accounts, err := d.Accounts.ListAccounts(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &accountsOutput{}
		out.Body.Items = make([]accountDTO, 0, len(accounts))
		for _, a := range accounts {
			out.Body.Items = append(out.Body.Items, accountToDTO(a))
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-create-upstream-account",
		Method:      http.MethodPost,
		Path:        "/api/v1/upstream-accounts",
		Summary:     "创建上游账号",
		Description: "创建一个供应商账号和一把 API Key；可支持多个不同 API 格式的请求端点。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *createAccountInput) (*accountOutput, error) {
		if d.AccountManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("account service is not configured")
		}
		a, err := d.AccountManager.CreateAccount(ctx, upstreamcontrol.CreateAccountInput{
			Name:              in.Body.Name,
			TenantDisplayName: in.Body.TenantDisplayName,
			TenantAccessMode:  in.Body.TenantAccessMode,
			APIKey:            in.Body.APIKey,
			Endpoints:         endpointWritesFromRequests(in.Body.Endpoints),
			ConcurrencyLimit:  int32PtrToIntPtr(in.Body.ConcurrencyLimit),
			PriceBookID:       in.Body.PriceBookID,
			TenantMultiplier:  in.Body.TenantMultiplier,
		})
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &accountOutput{Body: accountToDTO(a)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-upstream-account",
		Method:      http.MethodPatch,
		Path:        "/api/v1/upstream-accounts/{accountID}",
		Summary:     "更新上游账号",
		Description: "更新上游账号元数据或 API Key；请求端点通过独立 Endpoint API 管理。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *updateAccountInput) (*accountOutput, error) {
		if d.AccountManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("account service is not configured")
		}
		a, err := d.AccountManager.UpdateAccount(ctx, upstreamcontrol.UpdateAccountInput{
			ID:                in.AccountID,
			Name:              in.Body.Name,
			TenantDisplayName: in.Body.TenantDisplayName,
			TenantAccessMode:  in.Body.TenantAccessMode,
			APIKey:            in.Body.APIKey,
			ConcurrencyLimit:  int32PtrToIntPtr(in.Body.ConcurrencyLimit),
			PriceBookID:       in.Body.PriceBookID,
			TenantMultiplier:  in.Body.TenantMultiplier,
		})
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &accountOutput{Body: accountToDTO(a)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-upstream-account-status",
		Method:      http.MethodPatch,
		Path:        "/api/v1/upstream-accounts/{accountID}/status",
		Summary:     "更新上游账号状态",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *updateAccountStatusInput) (*accountOutput, error) {
		if d.AccountManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("account service is not configured")
		}
		a, err := d.AccountManager.UpdateAccountStatus(ctx, in.AccountID, in.Body.Status)
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &accountOutput{Body: accountToDTO(a)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-upstream-account",
		Method:      http.MethodDelete,
		Path:        "/api/v1/upstream-accounts/{accountID}",
		Summary:     "删除上游账号",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *deleteAccountInput) (*deleteAccountOutput, error) {
		if d.AccountManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("account service is not configured")
		}
		endpointIDs := make([]string, 0)
		if d.RuntimeHealth != nil && d.EndpointManager != nil {
			endpoints, err := d.EndpointManager.ListEndpoints(ctx, in.AccountID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			for _, endpoint := range endpoints {
				endpointIDs = append(endpointIDs, endpoint.ID)
			}
		}
		if err := d.AccountManager.DeleteAccount(ctx, in.AccountID); err != nil {
			return nil, mapServiceError(err)
		}
		if d.RuntimeHealth != nil {
			d.RuntimeHealth.Forget(in.AccountID)
			for _, endpointID := range endpointIDs {
				d.RuntimeHealth.Forget(endpointID)
			}
		}
		out := &deleteAccountOutput{}
		out.Body.Deleted = true
		return out, nil
	})
}

func accountToDTO(a domain.UpstreamAccount) accountDTO {
	return accountDTO{
		ID:                a.ID,
		Name:              a.Name,
		TenantDisplayName: a.TenantDisplayName,
		TenantAccessMode:  a.TenantAccessMode,
		Endpoints:         endpointDTOs(a.Endpoints),
		ConcurrencyLimit:  intPtrToInt32Ptr(a.ConcurrencyLimit),
		PriceBookID:       a.PriceBookID,
		TenantMultiplier:  a.TenantMultiplier,
		Status:            a.Status,
		InvalidReason:     a.InvalidReason,
		InvalidAt:         timeToMillisPtrFromOptional(a.InvalidAt),
		CreatedAt:         timeToMillisPtr(a.CreatedAt),
		UpdatedAt:         timeToMillisPtr(a.UpdatedAt),
	}
}

func endpointWriteFromRequest(in accountEndpointWriteRequest) domain.UpstreamAccountEndpointWrite {
	return domain.UpstreamAccountEndpointWrite{
		APIFormat: domain.UpstreamProtocol(in.APIFormat), BaseURL: in.BaseURL,
		PathOverride: in.PathOverride, AuthScheme: in.AuthScheme, AuthHeader: in.AuthHeader,
		ExtraHeaders: in.ExtraHeaders, Status: in.Status,
	}
}

func endpointWritesFromRequests(items []accountEndpointWriteRequest) []domain.UpstreamAccountEndpointWrite {
	out := make([]domain.UpstreamAccountEndpointWrite, 0, len(items))
	for _, item := range items {
		out = append(out, endpointWriteFromRequest(item))
	}
	return out
}

func endpointToDTO(item domain.UpstreamAccountEndpoint) accountEndpointDTO {
	return accountEndpointDTO{
		ID: item.ID, AccountID: item.AccountID, APIFormat: string(item.APIFormat), BaseURL: item.BaseURL,
		PathOverride: item.PathOverride, AuthScheme: item.AuthScheme, AuthHeader: item.AuthHeader,
		ExtraHeaders: redactProviderExtraHeaders(item.ExtraHeaders), Status: item.Status,
		HealthStatus: string(item.HealthStatus), LastError: item.LastError,
		LastCheckedAt: timeToMillisPtrFromOptional(item.LastCheckedAt),
		CreatedAt:     timeToMillisPtr(item.CreatedAt), UpdatedAt: timeToMillisPtr(item.UpdatedAt),
	}
}

func endpointDTOs(items []domain.UpstreamAccountEndpoint) []accountEndpointDTO {
	out := make([]accountEndpointDTO, 0, len(items))
	for _, item := range items {
		out = append(out, endpointToDTO(item))
	}
	return out
}

func registerUpstreamAccountEndpoints(api huma.API, d UpstreamAccountManagementHTTPDeps) {
	huma.Register(api, huma.Operation{OperationID: "ai-list-upstream-account-endpoints", Method: http.MethodGet, Path: "/api/v1/upstream-accounts/{accountID}/endpoints", Summary: "上游账号请求端点列表", Tags: []string{"upstream-accounts"}}, func(ctx context.Context, in *accountEndpointsInput) (*accountEndpointsOutput, error) {
		if d.EndpointManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("endpoint service is not configured")
		}
		items, err := d.EndpointManager.ListEndpoints(ctx, in.AccountID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &accountEndpointsOutput{}
		out.Body.Items, out.Body.Total = endpointDTOs(items), len(items)
		return out, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-create-upstream-account-endpoint", Method: http.MethodPost, Path: "/api/v1/upstream-accounts/{accountID}/endpoints", Summary: "创建上游账号请求端点", Tags: []string{"upstream-accounts"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *createAccountEndpointInput) (*accountEndpointOutput, error) {
		if d.EndpointManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("endpoint service is not configured")
		}
		item, err := d.EndpointManager.CreateEndpoint(ctx, in.AccountID, endpointWriteFromRequest(in.Body))
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &accountEndpointOutput{Body: endpointToDTO(item)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-update-upstream-account-endpoint", Method: http.MethodPatch, Path: "/api/v1/upstream-accounts/{accountID}/endpoints/{endpointID}", Summary: "更新上游账号请求端点", Tags: []string{"upstream-accounts"}}, func(ctx context.Context, in *updateAccountEndpointInput) (*accountEndpointOutput, error) {
		if d.EndpointManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("endpoint service is not configured")
		}
		item, err := d.EndpointManager.UpdateEndpoint(ctx, in.AccountID, in.EndpointID, endpointWriteFromRequest(in.Body))
		if err != nil {
			return nil, mapServiceError(err)
		}
		syncEndpointRuntimeHealth(d.RuntimeHealth, item)
		return &accountEndpointOutput{Body: endpointToDTO(item)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-delete-upstream-account-endpoint", Method: http.MethodDelete, Path: "/api/v1/upstream-accounts/{accountID}/endpoints/{endpointID}", Summary: "删除上游账号请求端点", Tags: []string{"upstream-accounts"}}, func(ctx context.Context, in *accountEndpointInput) (*deleteAccountOutput, error) {
		if d.EndpointManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("endpoint service is not configured")
		}
		if err := d.EndpointManager.DeleteEndpoint(ctx, in.AccountID, in.EndpointID); err != nil {
			return nil, mapServiceError(err)
		}
		if d.RuntimeHealth != nil {
			d.RuntimeHealth.Forget(in.EndpointID)
		}
		out := &deleteAccountOutput{}
		out.Body.Deleted = true
		return out, nil
	})
}

func syncEndpointRuntimeHealth(health routing.HealthTracker, endpoint domain.UpstreamAccountEndpoint) {
	if health == nil || endpoint.ID == "" {
		return
	}
	if endpoint.Status == domain.EndpointStatusActive {
		health.RecordSuccess(endpoint.ID, routing.TargetEndpoint)
		return
	}
	health.Forget(endpoint.ID)
}

func timeToMillisPtrFromOptional(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	return timeToMillisPtr(*value)
}

func redactProviderExtraHeaders(raw []byte) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage("null")
	}
	var headers map[string]any
	if err := json.Unmarshal(raw, &headers); err != nil {
		return json.RawMessage("null")
	}
	for key := range headers {
		if upstreamcontrol.IsSensitiveHeaderKey(key) {
			headers[key] = upstreamcontrol.RedactedHeaderValue
		}
	}
	out, err := json.Marshal(headers)
	if err != nil {
		return json.RawMessage("null")
	}
	return out
}

func jsonObjectOrNull(b []byte) json.RawMessage {
	if len(b) == 0 || !json.Valid(b) {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}

func jsonObjectOrEmpty(b []byte) json.RawMessage {
	if len(b) == 0 || !json.Valid(b) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(b)
}

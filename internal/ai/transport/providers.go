package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	"xiaodou/dai/libs/go/httpx"
)

// 上游账号（ai_upstream_accounts）：原 provider+endpoint 合并为顶级实体。

type accountDTO struct {
	ID                    string          `json:"id" doc:"上游账号 ID"`
	Name                  string          `json:"name" doc:"账号名称"`
	TenantDisplayName     string          `json:"tenant_display_name" doc:"租户侧展示名称"`
	TenantAccessMode      string          `json:"tenant_access_mode" enum:"public,restricted" doc:"租户访问范围"`
	BaseURL               string          `json:"base_url" doc:"上游基础 URL"`
	ExtraHeaders          json.RawMessage `json:"extra_headers" doc:"附加请求头 JSON"`
	DefaultProviderFamily string          `json:"default_provider_family" doc:"默认上游家族"`
	ConcurrencyLimit      *int32          `json:"concurrency_limit,omitempty" doc:"最大并发请求数；为空表示不限制"`
	PriceBookID           string          `json:"price_book_id,omitempty" doc:"租户结算价格表 ID"`
	TenantMultiplier      *float64        `json:"tenant_multiplier,omitempty" doc:"默认租户扣费倍率"`
	Status                string          `json:"status" enum:"active,invalid,disabled" doc:"状态：active=可用，invalid=凭据失效，disabled=管理员停用"`
	InvalidReason         string          `json:"invalid_reason,omitempty" doc:"系统判定失效的原因"`
	InvalidAt             *int64          `json:"invalid_at,omitempty" doc:"系统判定失效时间，Unix 毫秒"`

	CreatedAt *int64 `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt *int64 `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
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
	Name                  string          `json:"name" doc:"账号名称"`
	TenantDisplayName     string          `json:"tenant_display_name,omitempty" doc:"租户侧展示名称；为空时使用账号名称"`
	TenantAccessMode      string          `json:"tenant_access_mode,omitempty" enum:"public,restricted" doc:"租户访问范围；为空保留原值"`
	BaseURL               string          `json:"base_url" doc:"上游基础 URL"`
	APIKey                string          `json:"api_key" doc:"上游 API key 明文；仅写入时接收，响应不返回"`
	ExtraHeaders          json.RawMessage `json:"extra_headers,omitempty" doc:"附加请求头 JSON；为空默认 {}"`
	DefaultProviderFamily string          `json:"default_provider_family,omitempty" enum:"openai_compatible,anthropic,gemini" doc:"默认上游家族；为空默认 openai_compatible"`
	ConcurrencyLimit      *int32          `json:"concurrency_limit,omitempty" minimum:"1" doc:"最大并发请求数；为空表示不限制"`
	PriceBookID           string          `json:"price_book_id,omitempty" doc:"租户结算价格表 ID；为空不绑定"`
	TenantMultiplier      *float64        `json:"tenant_multiplier,omitempty" doc:"默认租户扣费倍率；为空不设置"`
}

type updateAccountRequest struct {
	Name                  string          `json:"name" doc:"账号名称"`
	TenantDisplayName     string          `json:"tenant_display_name,omitempty" doc:"租户侧展示名称；为空时使用账号名称"`
	TenantAccessMode      string          `json:"tenant_access_mode,omitempty" enum:"public,restricted" doc:"租户访问范围；为空默认 public"`
	BaseURL               string          `json:"base_url" doc:"上游基础 URL"`
	APIKey                string          `json:"api_key,omitempty" doc:"上游 API key 明文；为空保留原密文"`
	ExtraHeaders          json.RawMessage `json:"extra_headers,omitempty" doc:"附加请求头 JSON；为空默认 {}"`
	DefaultProviderFamily string          `json:"default_provider_family,omitempty" enum:"openai_compatible,anthropic,gemini" doc:"默认上游家族；为空保留原值"`
	ConcurrencyLimit      *int32          `json:"concurrency_limit,omitempty" minimum:"1" doc:"最大并发请求数；为空表示不限制"`
	PriceBookID           string          `json:"price_book_id,omitempty" doc:"租户结算价格表 ID；为空不绑定"`
	TenantMultiplier      *float64        `json:"tenant_multiplier,omitempty" doc:"默认租户扣费倍率；为空不设置"`
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

func registerUpstreamAccounts(api huma.API, d UpstreamAccountManagementHTTPDeps) {
	registerUpstreamAccountTransfer(api, d)

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
		Description: "创建上游账号；可服务模型和上游协议由显式上游模型绑定维护。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *createAccountInput) (*accountOutput, error) {
		if d.AccountManager == nil {
			return nil, httpx.ErrUnavailable.WithDetail("account service is not configured")
		}
		a, err := d.AccountManager.CreateAccount(ctx, upstreamcontrol.CreateAccountInput{
			Name:              in.Body.Name,
			TenantDisplayName: in.Body.TenantDisplayName,
			TenantAccessMode:  in.Body.TenantAccessMode,
			BaseURL:           in.Body.BaseURL,
			APIKey:            in.Body.APIKey,
			ExtraHeaders:      in.Body.ExtraHeaders,
			DefaultProtocol:   in.Body.DefaultProviderFamily,
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
		Description: "更新上游账号；可服务模型和上游协议由显式上游模型绑定维护。",
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
			BaseURL:           in.Body.BaseURL,
			APIKey:            in.Body.APIKey,
			ExtraHeaders:      in.Body.ExtraHeaders,
			DefaultProtocol:   in.Body.DefaultProviderFamily,
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
		if err := d.AccountManager.DeleteAccount(ctx, in.AccountID); err != nil {
			return nil, mapServiceError(err)
		}
		out := &deleteAccountOutput{}
		out.Body.Deleted = true
		return out, nil
	})
}

func accountToDTO(a domain.UpstreamAccount) accountDTO {
	return accountDTO{
		ID:                    a.ID,
		Name:                  a.Name,
		TenantDisplayName:     a.TenantDisplayName,
		TenantAccessMode:      a.TenantAccessMode,
		BaseURL:               a.BaseURL,
		ExtraHeaders:          redactProviderExtraHeaders(a.ExtraHeaders),
		DefaultProviderFamily: a.DefaultProtocol,
		ConcurrencyLimit:      intPtrToInt32Ptr(a.ConcurrencyLimit),
		PriceBookID:           a.PriceBookID,
		TenantMultiplier:      a.TenantMultiplier,
		Status:                a.Status,
		InvalidReason:         a.InvalidReason,
		InvalidAt:             timeToMillisPtrFromOptional(a.InvalidAt),
		CreatedAt:             timeToMillisPtr(a.CreatedAt),
		UpdatedAt:             timeToMillisPtr(a.UpdatedAt),
	}
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
		if isSensitiveHeaderKey(key) {
			headers[key] = "***REDACTED***"
		}
	}
	out, err := json.Marshal(headers)
	if err != nil {
		return json.RawMessage("null")
	}
	return out
}

func isSensitiveHeaderKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, part := range []string{"authorization", "proxy-authorization", "cookie", "token", "secret", "password", "api-key", "api_key", "apikey", "key"} {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
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

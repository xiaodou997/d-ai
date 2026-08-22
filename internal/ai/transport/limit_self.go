package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/commercial"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/libs/go/httpx"
)

type scopedLimitPolicyWriteRequest struct {
	ConcurrencyLimit *int32 `json:"concurrency_limit,omitempty" doc:"最大同时请求数"`
	Status           string `json:"status,omitempty" enum:"active,disabled" doc:"状态；为空默认 active"`
}

type tenantSelfUserLimitInput struct {
	UserID string `path:"userID"`
}

type tenantSelfUpsertUserLimitInput struct {
	UserID string `path:"userID"`
	Body   scopedLimitPolicyWriteRequest
}

type selfAPIKeyLimitInput struct {
	APIKeyID string `path:"apiKeyID"`
}

type selfUpsertAPIKeyLimitInput struct {
	APIKeyID string `path:"apiKeyID"`
	Body     scopedLimitPolicyWriteRequest
}

func registerTenantSelfLimits(api huma.API, d TenantSelfControlHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-user-limit-policies",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/users/{userID}/limit-policies",
		Summary:     "租户自助·指定用户限流策略",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, in *tenantSelfUserLimitInput) (*runtimeLimitPoliciesOutput, error) {
		if d.LimitPolicies == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		if err := ensureTenantOwnsEndUser(ctx, d.TenantEndUsers, tenantID, in.UserID); err != nil {
			return nil, err
		}
		return singleScopedPolicyResponse(ctx, d.LimitPolicies, commercial.LimitScopeUser, in.UserID)
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-upsert-tenant-self-user-limit-policy",
		Method:      http.MethodPut,
		Path:        "/api/v1/tenants/me/users/{userID}/limit-policies",
		Summary:     "租户自助·创建或更新指定用户限流策略",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, in *tenantSelfUpsertUserLimitInput) (*runtimeLimitPolicyOutput, error) {
		if d.LimitPolicies == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		if err := ensureTenantOwnsEndUser(ctx, d.TenantEndUsers, tenantID, in.UserID); err != nil {
			return nil, err
		}
		policy, err := upsertScopedLimitPolicy(ctx, d.LimitPolicies, commercial.LimitPolicyWrite{
			ScopeType:        commercial.LimitScopeUser,
			ScopeID:          in.UserID,
			ConcurrencyLimit: int32PtrToIntPtr(in.Body.ConcurrencyLimit),
			Status:           commercial.Status(in.Body.Status),
			CreatedBy:        userIDFromContext(ctx),
		})
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &runtimeLimitPolicyOutput{Body: runtimeLimitPolicyToDTO(policy)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-api-key-limit-policies",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/api-key-limit-policies",
		Summary:     "租户自助·租户 API 密钥限流策略列表",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, _ *struct{}) (*runtimeLimitPoliciesOutput, error) {
		return listOwnedAPIKeyPolicies(ctx, tenantIDFromContext(ctx), "", d.APIKeys, d.LimitPolicies)
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-upsert-tenant-self-api-key-limit-policy",
		Method:      http.MethodPut,
		Path:        "/api/v1/tenants/me/api-keys/{apiKeyID}/limit-policies",
		Summary:     "租户自助·创建或更新租户 API 密钥限流策略",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, in *selfUpsertAPIKeyLimitInput) (*runtimeLimitPolicyOutput, error) {
		if d.LimitPolicies == nil || d.APIKeys == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial or api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		if err := ensureTenantAPIKeyScope(ctx, d.APIKeys, tenantID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		policy, err := upsertScopedLimitPolicy(ctx, d.LimitPolicies, commercial.LimitPolicyWrite{
			ScopeType:        commercial.LimitScopeAPIKey,
			ScopeID:          in.APIKeyID,
			ConcurrencyLimit: int32PtrToIntPtr(in.Body.ConcurrencyLimit),
			Status:           commercial.Status(in.Body.Status),
			CreatedBy:        userIDFromContext(ctx),
		})
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &runtimeLimitPolicyOutput{Body: runtimeLimitPolicyToDTO(policy)}, nil
	})
}

func registerUserSelfLimits(api huma.API, d UserSelfControlHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-api-key-limit-policies",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/api-key-limit-policies",
		Summary:     "终端用户自助·个人 API 密钥限流策略列表",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, _ *struct{}) (*runtimeLimitPoliciesOutput, error) {
		return listOwnedAPIKeyPolicies(ctx, tenantIDFromContext(ctx), userIDFromContext(ctx), d.APIKeys, d.LimitPolicies)
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-upsert-user-self-api-key-limit-policy",
		Method:      http.MethodPut,
		Path:        "/api/v1/users/me/api-keys/{apiKeyID}/limit-policies",
		Summary:     "终端用户自助·创建或更新个人 API 密钥限流策略",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, in *selfUpsertAPIKeyLimitInput) (*runtimeLimitPolicyOutput, error) {
		if d.LimitPolicies == nil || d.APIKeys == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial or api key service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		if err := ensureUserAPIKeyScope(ctx, d.APIKeys, tenantID, userID, in.APIKeyID); err != nil {
			return nil, mapServiceError(err)
		}
		policy, err := upsertScopedLimitPolicy(ctx, d.LimitPolicies, commercial.LimitPolicyWrite{
			ScopeType:        commercial.LimitScopeAPIKey,
			ScopeID:          in.APIKeyID,
			ConcurrencyLimit: int32PtrToIntPtr(in.Body.ConcurrencyLimit),
			Status:           commercial.Status(in.Body.Status),
			CreatedBy:        userID,
		})
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &runtimeLimitPolicyOutput{Body: runtimeLimitPolicyToDTO(policy)}, nil
	})
}

func singleScopedPolicyResponse(ctx context.Context, policies CommercialLimitPolicyManager, scopeType commercial.LimitScope, scopeID string) (*runtimeLimitPoliciesOutput, error) {
	items, err := policies.ListLimitPolicies(ctx, commercial.LimitPolicyFilter{
		ScopeType: scopeType,
		ScopeID:   scopeID,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &runtimeLimitPoliciesOutput{}
	out.Body.Items = make([]runtimeLimitPolicyDTO, 0, len(items))
	for _, policy := range items {
		out.Body.Items = append(out.Body.Items, runtimeLimitPolicyToDTO(policy))
	}
	out.Body.Total = len(out.Body.Items)
	return out, nil
}

func listOwnedAPIKeyPolicies(ctx context.Context, tenantID, userID string, apiKeys APIKeyReader, policies CommercialLimitPolicyManager) (*runtimeLimitPoliciesOutput, error) {
	if apiKeys == nil || policies == nil {
		return nil, httpx.ErrUnavailable.WithDetail("commercial or api key service is not configured")
	}
	if tenantID == "" {
		return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
	}
	var (
		keys []coreidentity.APIKey
		err  error
	)
	if userID == "" {
		keys, err = apiKeys.ListForTenant(ctx, tenantID)
	} else {
		keys, err = apiKeys.ListForUser(ctx, tenantID, userID)
	}
	if err != nil {
		return nil, mapServiceError(err)
	}
	scopeIDs := make([]string, 0, len(keys))
	for _, key := range keys {
		scopeIDs = append(scopeIDs, key.ID)
	}
	policyItems, err := policies.ListLimitPolicies(ctx, commercial.LimitPolicyFilter{
		ScopeType: commercial.LimitScopeAPIKey,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	allowed := make(map[string]struct{}, len(scopeIDs))
	for _, scopeID := range scopeIDs {
		allowed[scopeID] = struct{}{}
	}
	out := &runtimeLimitPoliciesOutput{}
	out.Body.Items = make([]runtimeLimitPolicyDTO, 0, len(policyItems))
	for _, policy := range policyItems {
		if _, ok := allowed[policy.ScopeID]; !ok {
			continue
		}
		out.Body.Items = append(out.Body.Items, runtimeLimitPolicyToDTO(policy))
	}
	out.Body.Total = len(out.Body.Items)
	return out, nil
}

func upsertScopedLimitPolicy(ctx context.Context, policies CommercialLimitPolicyManager, in commercial.LimitPolicyWrite) (commercial.LimitPolicy, error) {
	existing, err := policies.ListLimitPolicies(ctx, commercial.LimitPolicyFilter{
		ScopeType: in.ScopeType,
		ScopeID:   in.ScopeID,
	})
	if err != nil {
		return commercial.LimitPolicy{}, err
	}
	if len(existing) > 0 {
		return policies.UpdateLimitPolicy(ctx, existing[0].ID, in)
	}
	return policies.CreateLimitPolicy(ctx, in)
}

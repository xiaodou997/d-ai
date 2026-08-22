package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/libs/go/httpx"
)

// 模型授权(model grants)已删除，由分组体系(groups.go)替代。本文件仅保留运行时限流策略。

type runtimeLimitPolicyDTO struct {
	ID               string  `json:"id" doc:"限流策略 ID"`
	ScopeType        string  `json:"scope_type" enum:"tenant,user,api_key" doc:"限流作用域类型"`
	ScopeID          string  `json:"scope_id" doc:"限流作用域 ID"`
	ConcurrencyLimit *int32  `json:"concurrency_limit,omitempty" doc:"最大同时请求数"`
	Status           string  `json:"status" enum:"active,disabled" doc:"状态"`
	CreatedBy        *string `json:"created_by,omitempty" doc:"创建人"`
	CreatedAt        *int64  `json:"created_at,omitempty" doc:"创建时间，Unix 毫秒"`
	UpdatedAt        *int64  `json:"updated_at,omitempty" doc:"更新时间，Unix 毫秒"`
}

type runtimeLimitPoliciesOutput struct {
	Body struct {
		Items    []runtimeLimitPolicyDTO `json:"items"`
		Total    int                     `json:"total"`
		Included IdentityIncludedDTO     `json:"included"`
	}
}

type runtimeLimitPolicyOutput struct {
	Body runtimeLimitPolicyDTO
}

type limitPolicyWriteRequest struct {
	ScopeType        string `json:"scope_type" enum:"tenant,user,api_key" doc:"限流作用域类型"`
	ScopeID          string `json:"scope_id" doc:"限流作用域 ID"`
	ConcurrencyLimit *int32 `json:"concurrency_limit,omitempty" doc:"最大同时请求数"`
	Status           string `json:"status,omitempty" enum:"active,disabled" doc:"状态；为空默认 active"`
	CreatedBy        string `json:"created_by,omitempty" doc:"创建人"`
}

type createLimitPolicyInput struct {
	Body limitPolicyWriteRequest
}

type updateLimitPolicyInput struct {
	PolicyID string `path:"policyID" doc:"限流策略 ID"`
	Body     limitPolicyWriteRequest
}

type updateLimitPolicyStatusInput struct {
	PolicyID string `path:"policyID" doc:"限流策略 ID"`
	Body     struct {
		Status string `json:"status" enum:"active,disabled" doc:"状态"`
	}
}

func limitPolicyWriteFromRequest(req limitPolicyWriteRequest) commercial.LimitPolicyWrite {
	return commercial.LimitPolicyWrite{
		ScopeType:        commercial.LimitScope(req.ScopeType),
		ScopeID:          req.ScopeID,
		ConcurrencyLimit: int32PtrToIntPtr(req.ConcurrencyLimit),
		Status:           commercial.Status(req.Status),
		CreatedBy:        req.CreatedBy,
	}
}

func registerLimits(api huma.API, d CoreHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-runtime-limit-policies",
		Method:      http.MethodGet,
		Path:        "/api/v1/limit-policies",
		Summary:     "运行时限流策略列表",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, _ *struct{}) (*runtimeLimitPoliciesOutput, error) {
		if d.LimitPolicies == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		policies, err := d.LimitPolicies.ListLimitPolicies(ctx, commercial.LimitPolicyFilter{
			ScopeType: commercial.LimitScopeTenant,
		})
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runtimeLimitPoliciesOutput{}
		out.Body.Items = make([]runtimeLimitPolicyDTO, 0, len(policies))
		for _, policy := range policies {
			out.Body.Items = append(out.Body.Items, runtimeLimitPolicyToDTO(policy))
		}
		out.Body.Total = len(out.Body.Items)
		out.Body.Included = buildIdentityIncludedForLimitPolicies(ctx, d, policies)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "ai-create-runtime-limit-policy",
		Method:        http.MethodPost,
		Path:          "/api/v1/limit-policies",
		Summary:       "创建运行时限流策略",
		Tags:          []string{"limit-policies"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *createLimitPolicyInput) (*runtimeLimitPolicyOutput, error) {
		if d.LimitPolicies == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		if in.Body.ScopeType != "tenant" {
			return nil, httpx.ErrBadRequest.WithDetail("admin limit policies only support tenant scope")
		}
		policy, err := d.LimitPolicies.CreateLimitPolicy(ctx, limitPolicyWriteFromRequest(in.Body))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runtimeLimitPolicyOutput{}
		out.Body = runtimeLimitPolicyToDTO(policy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-runtime-limit-policy",
		Method:      http.MethodPatch,
		Path:        "/api/v1/limit-policies/{policyID}",
		Summary:     "更新运行时限流策略",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, in *updateLimitPolicyInput) (*runtimeLimitPolicyOutput, error) {
		if d.LimitPolicies == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		if in.Body.ScopeType != "tenant" {
			return nil, httpx.ErrBadRequest.WithDetail("admin limit policies only support tenant scope")
		}
		policy, err := d.LimitPolicies.UpdateLimitPolicy(ctx, in.PolicyID, limitPolicyWriteFromRequest(in.Body))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runtimeLimitPolicyOutput{}
		out.Body = runtimeLimitPolicyToDTO(policy)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-runtime-limit-policy-status",
		Method:      http.MethodPatch,
		Path:        "/api/v1/limit-policies/{policyID}/status",
		Summary:     "更新运行时限流策略状态",
		Tags:        []string{"limit-policies"},
	}, func(ctx context.Context, in *updateLimitPolicyStatusInput) (*runtimeLimitPolicyOutput, error) {
		if d.LimitPolicies == nil {
			return nil, httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		policy, err := d.LimitPolicies.UpdateLimitPolicyStatus(ctx, in.PolicyID, commercial.Status(in.Body.Status))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runtimeLimitPolicyOutput{}
		out.Body = runtimeLimitPolicyToDTO(policy)
		return out, nil
	})
}

func runtimeLimitPolicyToDTO(policy commercial.LimitPolicy) runtimeLimitPolicyDTO {
	return runtimeLimitPolicyDTO{
		ID:               policy.ID,
		ScopeType:        string(policy.ScopeType),
		ScopeID:          policy.ScopeID,
		ConcurrencyLimit: intPtrToInt32Ptr(policy.ConcurrencyLimit),
		Status:           string(policy.Status),
		CreatedBy:        stringPtrOrNil(policy.CreatedBy),
		CreatedAt:        timeToMillisPtr(policy.CreatedAt),
		UpdatedAt:        timeToMillisPtr(policy.UpdatedAt),
	}
}

func int32PtrToIntPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func intPtrToInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

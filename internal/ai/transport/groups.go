package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

// ============================================================================
// 分组 DTO
// ============================================================================

type groupDTO struct {
	ID                      string  `json:"id"`
	TenantID                string  `json:"tenant_id"`
	Name                    string  `json:"name"`
	Description             string  `json:"description"`
	RetailPriceBookID       string  `json:"retail_price_book_id" doc:"用户零售价格表"`
	DefaultUserMultiplier   float64 `json:"default_user_multiplier" doc:"用户默认零售倍率"`
	UserDefaultVisible      bool    `json:"user_default_visible"`
	AllowProtocolConversion bool    `json:"allow_protocol_conversion" doc:"允许协议转换：true=本组候选可作跨格式转换目标"`
	SortOrder               int32   `json:"sort_order"`
	Status                  string  `json:"status"`
	RetailPriceBookName     string  `json:"retail_price_book_name,omitempty"`
	CreatedAt               *int64  `json:"created_at,omitempty"`
	UpdatedAt               *int64  `json:"updated_at,omitempty"`
}

type groupWriteRequest struct {
	Name                    string   `json:"name"`
	Description             string   `json:"description,omitempty"`
	RetailPriceBookID       string   `json:"retail_price_book_id"`
	DefaultUserMultiplier   *float64 `json:"default_user_multiplier,omitempty" doc:"为空默认 1"`
	UserDefaultVisible      bool     `json:"user_default_visible,omitempty"`
	AllowProtocolConversion bool     `json:"allow_protocol_conversion,omitempty" doc:"允许协议转换；默认 false=仅同家族 passthrough"`
	SortOrder               int32    `json:"sort_order,omitempty"`
	Status                  string   `json:"status,omitempty" enum:"active,disabled"`
}

type groupsOutput struct {
	Body struct {
		Items []groupDTO `json:"items"`
		Total int        `json:"total"`
	}
}
type groupOutput struct{ Body groupDTO }

type createGroupInput struct{ Body groupWriteRequest }
type groupIDInput struct {
	GroupID string `path:"groupID"`
}
type updateGroupInput struct {
	GroupID string `path:"groupID"`
	Body    groupWriteRequest
}
type updateGroupStatusInput struct {
	GroupID string `path:"groupID"`
	Body    accountStatusRequest
}

type groupClientSurfacePolicyDTO struct {
	GroupID         string   `json:"group_id"`
	Mode            string   `json:"mode" enum:"all,restricted"`
	AllowedSurfaces []string `json:"allowed_surfaces" enum:"openai_chat,openai_responses,openai_embeddings,anthropic_messages,gemini_text,gemini_embeddings,openai_images,gemini_images"`
}

type groupClientSurfacePolicyWriteRequest struct {
	Mode            string   `json:"mode" enum:"all,restricted"`
	AllowedSurfaces []string `json:"allowed_surfaces" enum:"openai_chat,openai_responses,openai_embeddings,anthropic_messages,gemini_text,gemini_embeddings,openai_images,gemini_images" doc:"restricted 模式允许的客户端 API surface；all 模式忽略"`
}

type groupClientSurfacePolicyOutput struct{ Body groupClientSurfacePolicyDTO }

type replaceGroupClientSurfacePolicyInput struct {
	GroupID string `path:"groupID"`
	Body    groupClientSurfacePolicyWriteRequest
}

type deletedOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

type groupDispatchRuleDTO struct {
	ID                 string `json:"id"`
	GroupID            string `json:"group_id"`
	ClientSurface      string `json:"client_surface"`
	MatchType          string `json:"match_type"`
	MatchValue         string `json:"match_value"`
	TargetModelCode    string `json:"target_model_code"`
	Priority           int32  `json:"priority"`
	Status             string `json:"status"`
	Notes              string `json:"notes,omitempty"`
	RequiredCapability string `json:"required_capability,omitempty"`
	PriceState         string `json:"price_state,omitempty"`
	CanEnable          bool   `json:"can_enable"`
	CreatedAt          *int64 `json:"created_at,omitempty"`
	UpdatedAt          *int64 `json:"updated_at,omitempty"`
}

type groupDispatchRuleWriteRequest struct {
	ClientSurface   string `json:"client_surface" enum:"openai_chat,openai_responses,openai_embeddings,anthropic_messages,gemini_text,gemini_embeddings,openai_images,gemini_images"`
	MatchType       string `json:"match_type" enum:"exact,prefix,wildcard,regex"`
	MatchValue      string `json:"match_value"`
	TargetModelCode string `json:"target_model_code"`
	Priority        *int32 `json:"priority,omitempty"`
	Notes           string `json:"notes,omitempty"`
}

type groupDispatchRulesOutput struct {
	Body struct {
		Items []groupDispatchRuleDTO `json:"items"`
		Total int                    `json:"total"`
	}
}

type groupDispatchRuleOutput struct{ Body groupDispatchRuleDTO }

type createGroupDispatchRuleInput struct {
	GroupID string `path:"groupID"`
	Body    groupDispatchRuleWriteRequest
}

type groupDispatchPreviewRequest struct {
	ClientSurface  string `json:"client_surface" enum:"openai_chat,openai_responses,openai_embeddings,anthropic_messages,gemini_text,gemini_embeddings,openai_images,gemini_images"`
	RequestedModel string `json:"requested_model"`
}

type groupDispatchPreviewCandidateDTO struct {
	TargetType         string `json:"target_type"`
	AccountID          string `json:"account_id,omitempty"`
	CredentialPoolID   string `json:"credential_pool_id,omitempty"`
	DisplayName        string `json:"display_name,omitempty"`
	ProviderFamily     string `json:"provider_family,omitempty"`
	ProviderAPIFormat  string `json:"provider_api_format,omitempty"`
	ProtocolConversion bool   `json:"protocol_conversion"`
	Priority           int32  `json:"priority"`
}

type groupDispatchPreviewRejectionDTO struct {
	TargetType    string `json:"target_type,omitempty"`
	TargetID      string `json:"target_id,omitempty"`
	ResolvedModel string `json:"resolved_model"`
	ReasonCode    string `json:"reason_code" enum:"no_active_group_target,access_denied,target_not_found,target_inactive,model_binding_missing,protocol_incompatible,credential_unavailable,binding_invalid,binding_unavailable"`
	ReasonDetail  string `json:"reason_detail,omitempty"`
	Priority      int32  `json:"priority"`
}

type groupDispatchPreviewDTO struct {
	RequestedModel       string                             `json:"requested_model"`
	ClientSurface        string                             `json:"client_surface"`
	MatchedRule          *groupDispatchRuleDTO              `json:"matched_rule,omitempty"`
	ResolvedLogicalModel string                             `json:"resolved_logical_model"`
	CandidateUpstreams   []groupDispatchPreviewCandidateDTO `json:"candidate_upstreams"`
	RejectedCandidates   []groupDispatchPreviewRejectionDTO `json:"rejected_candidates"`
}

type groupDispatchPreviewOutput struct{ Body groupDispatchPreviewDTO }

type previewGroupDispatchInput struct {
	GroupID string `path:"groupID"`
	Body    groupDispatchPreviewRequest
}

type updateGroupDispatchRuleInput struct {
	GroupID string `path:"groupID"`
	RuleID  string `path:"ruleID"`
	Body    groupDispatchRuleWriteRequest
}

type updateGroupDispatchRuleStatusInput struct {
	GroupID string `path:"groupID"`
	RuleID  string `path:"ruleID"`
	Body    accountStatusRequest
}

type dispatchModelDTO struct {
	ModelCode        string `json:"model_code"`
	Capability       string `json:"capability"`
	AvailableTargets int    `json:"available_targets"`
}

type dispatchModelsOutput struct {
	Body struct {
		Items []dispatchModelDTO `json:"items"`
		Total int                `json:"total"`
	}
}

type dispatchModelsInput struct {
	GroupID       string `path:"groupID"`
	ClientSurface string `query:"client_surface"`
}

type deleteGroupDispatchRuleInput struct {
	GroupID string `path:"groupID"`
	RuleID  string `path:"ruleID"`
}

// 分组关联上游目标 DTO（账号或凭证池）
type groupTargetDTO struct {
	ID                    string `json:"id"`
	GroupID               string `json:"group_id"`
	AccountID             string `json:"account_id,omitempty"`
	CredentialPoolID      string `json:"credential_pool_id,omitempty"`
	Priority              int32  `json:"priority"`
	Status                string `json:"status"`
	TargetType            string `json:"target_type,omitempty" doc:"account|pool"`
	AccountName           string `json:"account_name,omitempty"`
	DefaultProviderFamily string `json:"default_provider_family,omitempty"`
	PoolName              string `json:"pool_name,omitempty"`
	FixedProviderType     string `json:"fixed_provider_type,omitempty"`
	// Available 反映该绑定的上游资源当前是否仍可被本租户路由；false 时 UnavailableReason
	// 给出原因（inactive/access_revoked/missing）。用于呈现「已绑定但请求会被拒」的哑故障。
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty" doc:"inactive|access_revoked|missing"`
	CreatedAt         *int64 `json:"created_at,omitempty"`
	UpdatedAt         *int64 `json:"updated_at,omitempty"`
}

type groupTargetWriteRequest struct {
	AccountID        string `json:"account_id,omitempty" doc:"上游账号 id；与 credential_pool_id 二选一"`
	CredentialPoolID string `json:"credential_pool_id,omitempty" doc:"凭证池 id；与 account_id 二选一"`
	Priority         *int32 `json:"priority,omitempty"`
	Status           string `json:"status,omitempty" enum:"active,disabled"`
}

// 更新仅允许改 priority/status；换目标请删除后重新关联。
type groupTargetUpdateRequest struct {
	Priority *int32 `json:"priority,omitempty"`
	Status   string `json:"status,omitempty" enum:"active,disabled"`
}

type groupTargetsOutput struct {
	Body struct {
		Items []groupTargetDTO `json:"items"`
		Total int              `json:"total"`
	}
}
type groupTargetOutput struct{ Body groupTargetDTO }

type createGroupTargetInput struct {
	GroupID string `path:"groupID"`
	Body    groupTargetWriteRequest
}
type updateGroupTargetInput struct {
	GroupID   string `path:"groupID"`
	BindingID string `path:"bindingID"`
	Body      groupTargetUpdateRequest
}
type deleteGroupTargetInput struct {
	GroupID   string `path:"groupID"`
	BindingID string `path:"bindingID"`
}

// 反查：按上游目标查关联了哪些分组
type linkedGroupsByTargetInput struct {
	TargetKind string `query:"target_kind" doc:"direct_upstream|oauth_pool"`
	TargetID   string `query:"target_id"`
}

type linkedGroupItemDTO struct {
	GroupID                    string  `json:"group_id"`
	GroupName                  string  `json:"group_name,omitempty"`
	GroupDefaultUserMultiplier float64 `json:"group_default_user_multiplier,omitempty"`
	Priority                   int32   `json:"priority"`
	Status                     string  `json:"status"`
}

type linkedGroupsOutput struct {
	Body struct {
		Items []linkedGroupItemDTO `json:"items"`
		Total int                  `json:"total"`
	}
}

type userGroupDTO struct {
	GroupID                string   `json:"group_id"`
	GroupName              string   `json:"group_name,omitempty"`
	UserMultiplierOverride *float64 `json:"multiplier_override,omitempty"`
}
type userGroupsOutput struct {
	Body struct {
		Items []userGroupDTO `json:"items"`
		Total int            `json:"total"`
	}
}
type userGroupsListInput struct {
	UserID string `path:"userID"`
}
type upsertUserGroupInput struct {
	UserID  string `path:"userID"`
	GroupID string `path:"groupID"`
	Body    userGroupWriteRequest
}
type userGroupWriteRequest struct {
	UserMultiplierOverride *float64 `json:"multiplier_override,omitempty" doc:"为空则继承分组默认用户倍率"`
}
type deleteUserGroupInput struct {
	UserID  string `path:"userID"`
	GroupID string `path:"groupID"`
}

func registerGroups(api huma.API, d AIDeps) {
	commercialSvcReady := func() error {
		if d.CommercialSvc == nil {
			return httpx.ErrUnavailable.WithDetail("commercial service is not configured")
		}
		return nil
	}

	huma.Register(api, huma.Operation{OperationID: "ai-list-groups", Method: http.MethodGet, Path: "/api/v1/tenants/me/groups", Summary: "分组列表", Tags: []string{"groups"}},
		func(ctx context.Context, _ *struct{}) (*groupsOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			tenantID := tenantIDFromContext(ctx)
			items, err := d.CommercialSvc.ListGroups(ctx, tenantID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			priceBookNames, err := loadPriceBookNameMap(ctx, d, tenantID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			out := &groupsOutput{}
			out.Body.Items = make([]groupDTO, 0, len(items))
			for _, group := range items {
				dto := groupToDTO(group)
				dto.RetailPriceBookName = priceBookNames[group.RetailPriceBookID]
				out.Body.Items = append(out.Body.Items, dto)
			}
			out.Body.Total = len(out.Body.Items)
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-create-group", Method: http.MethodPost, Path: "/api/v1/tenants/me/groups", Summary: "创建分组", Tags: []string{"groups"}},
		func(ctx context.Context, in *createGroupInput) (*groupOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			tenantID := tenantIDFromContext(ctx)
			group, err := d.CommercialSvc.CreateGroup(ctx, tenantID, groupWriteFromReq(in.Body))
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupOutput{Body: groupToDTO(group)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-get-group", Method: http.MethodGet, Path: "/api/v1/tenants/me/groups/{groupID}", Summary: "分组详情", Tags: []string{"groups"}},
		func(ctx context.Context, in *groupIDInput) (*groupOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			group, err := d.CommercialSvc.GetGroup(ctx, tenantGroupScope(ctx, in.GroupID))
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupOutput{Body: groupToDTO(group)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-update-group", Method: http.MethodPatch, Path: "/api/v1/tenants/me/groups/{groupID}", Summary: "更新分组", Tags: []string{"groups"}},
		func(ctx context.Context, in *updateGroupInput) (*groupOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			group, err := d.CommercialSvc.UpdateGroup(ctx, tenantGroupScope(ctx, in.GroupID), groupWriteFromReq(in.Body))
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupOutput{Body: groupToDTO(group)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-update-group-status", Method: http.MethodPatch, Path: "/api/v1/tenants/me/groups/{groupID}/status", Summary: "更新分组状态", Tags: []string{"groups"}},
		func(ctx context.Context, in *updateGroupStatusInput) (*groupOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			group, err := d.CommercialSvc.UpdateGroupStatus(ctx, tenantGroupScope(ctx, in.GroupID), commercial.Status(in.Body.Status))
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupOutput{Body: groupToDTO(group)}, nil
		})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-group-client-surface-policy",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/groups/{groupID}/client-surface-policy",
		Summary:     "获取分组 API 入口策略",
		Tags:        []string{"groups"},
	}, func(ctx context.Context, in *groupIDInput) (*groupClientSurfacePolicyOutput, error) {
		if err := commercialSvcReady(); err != nil {
			return nil, err
		}
		policy, err := d.CommercialSvc.GetGroupClientSurfacePolicy(ctx, tenantGroupScope(ctx, in.GroupID))
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &groupClientSurfacePolicyOutput{Body: groupClientSurfacePolicyToDTO(policy)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-replace-group-client-surface-policy",
		Method:      http.MethodPut,
		Path:        "/api/v1/tenants/me/groups/{groupID}/client-surface-policy",
		Summary:     "整体替换分组 API 入口策略",
		Tags:        []string{"groups"},
	}, func(ctx context.Context, in *replaceGroupClientSurfacePolicyInput) (*groupClientSurfacePolicyOutput, error) {
		if err := commercialSvcReady(); err != nil {
			return nil, err
		}
		allowed := make([]surface.ID, 0, len(in.Body.AllowedSurfaces))
		for _, item := range in.Body.AllowedSurfaces {
			allowed = append(allowed, surface.ID(item))
		}
		policy, err := d.CommercialSvc.ReplaceGroupClientSurfacePolicy(ctx, tenantGroupScope(ctx, in.GroupID), commercial.GroupClientSurfacePolicyWrite{
			Mode:            commercial.GroupClientSurfacePolicyMode(in.Body.Mode),
			AllowedSurfaces: allowed,
		})
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &groupClientSurfacePolicyOutput{Body: groupClientSurfacePolicyToDTO(policy)}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "ai-delete-group", Method: http.MethodDelete, Path: "/api/v1/tenants/me/groups/{groupID}", Summary: "删除分组", Tags: []string{"groups"}},
		func(ctx context.Context, in *groupIDInput) (*deletedOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			if err := d.CommercialSvc.DeleteGroup(ctx, tenantGroupScope(ctx, in.GroupID)); err != nil {
				return nil, mapServiceError(err)
			}
			out := &deletedOutput{}
			out.Body.Deleted = true
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-list-group-dispatch-rules", Method: http.MethodGet, Path: "/api/v1/tenants/me/groups/{groupID}/dispatch-rules", Summary: "分组请求模型调度规则列表", Tags: []string{"groups"}},
		func(ctx context.Context, in *groupIDInput) (*groupDispatchRulesOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			items, err := d.CommercialSvc.ListDispatchRules(ctx, tenantGroupScope(ctx, in.GroupID))
			if err != nil {
				return nil, mapServiceError(err)
			}
			out := &groupDispatchRulesOutput{}
			out.Body.Items = make([]groupDispatchRuleDTO, 0, len(items))
			for _, item := range items {
				out.Body.Items = append(out.Body.Items, groupDispatchRuleToDTO(item))
			}
			out.Body.Total = len(out.Body.Items)
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-preview-group-dispatch", Method: http.MethodPost, Path: "/api/v1/tenants/me/groups/{groupID}/dispatch-rules/preview", Summary: "预览分组请求模型调度", Tags: []string{"groups"}},
		func(ctx context.Context, in *previewGroupDispatchInput) (*groupDispatchPreviewOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			preview, err := d.CommercialSvc.PreviewDispatch(ctx, tenantGroupScope(ctx, in.GroupID), in.Body.RequestedModel, surface.ID(in.Body.ClientSurface))
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupDispatchPreviewOutput{Body: groupDispatchPreviewToDTO(preview)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-add-group-dispatch-rule", Method: http.MethodPost, Path: "/api/v1/tenants/me/groups/{groupID}/dispatch-rules", Summary: "新增分组请求模型调度规则", Tags: []string{"groups"}},
		func(ctx context.Context, in *createGroupDispatchRuleInput) (*groupDispatchRuleOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			write, err := dispatchRuleWriteFromRequest(in.Body)
			if err != nil {
				return nil, mapServiceError(err)
			}
			rule, err := d.CommercialSvc.AddDispatchRule(ctx, tenantGroupScope(ctx, in.GroupID), write)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupDispatchRuleOutput{Body: groupDispatchRuleToDTO(rule)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-update-group-dispatch-rule", Method: http.MethodPatch, Path: "/api/v1/tenants/me/groups/{groupID}/dispatch-rules/{ruleID}", Summary: "更新分组请求模型调度规则", Tags: []string{"groups"}},
		func(ctx context.Context, in *updateGroupDispatchRuleInput) (*groupDispatchRuleOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			write, err := dispatchRuleWriteFromRequest(in.Body)
			if err != nil {
				return nil, mapServiceError(err)
			}
			rule, err := d.CommercialSvc.UpdateDispatchRule(ctx, tenantGroupScope(ctx, in.GroupID), in.RuleID, write)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupDispatchRuleOutput{Body: groupDispatchRuleToDTO(rule)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-update-group-dispatch-rule-status", Method: http.MethodPatch, Path: "/api/v1/tenants/me/groups/{groupID}/dispatch-rules/{ruleID}/status", Summary: "更新分组请求模型调度规则状态", Tags: []string{"groups"}},
		func(ctx context.Context, in *updateGroupDispatchRuleStatusInput) (*groupDispatchRuleOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			rule, err := d.CommercialSvc.UpdateDispatchRuleStatus(ctx, tenantGroupScope(ctx, in.GroupID), in.RuleID, commercial.Status(in.Body.Status))
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupDispatchRuleOutput{Body: groupDispatchRuleToDTO(rule)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-list-group-dispatch-models", Method: http.MethodGet, Path: "/api/v1/tenants/me/groups/{groupID}/dispatch-models", Summary: "可用于调度的逻辑模型", Tags: []string{"groups"}},
		func(ctx context.Context, in *dispatchModelsInput) (*dispatchModelsOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			items, err := d.CommercialSvc.ListDispatchModels(ctx, tenantGroupScope(ctx, in.GroupID), surface.ID(in.ClientSurface))
			if err != nil {
				return nil, mapServiceError(err)
			}
			out := &dispatchModelsOutput{}
			out.Body.Items = make([]dispatchModelDTO, 0, len(items))
			for _, item := range items {
				out.Body.Items = append(out.Body.Items, dispatchModelDTO{ModelCode: item.ModelCode, Capability: item.Capability, AvailableTargets: item.AvailableTargets})
			}
			out.Body.Total = len(out.Body.Items)
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-delete-group-dispatch-rule", Method: http.MethodDelete, Path: "/api/v1/tenants/me/groups/{groupID}/dispatch-rules/{ruleID}", Summary: "删除分组请求模型调度规则", Tags: []string{"groups"}},
		func(ctx context.Context, in *deleteGroupDispatchRuleInput) (*deletedOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			if err := d.CommercialSvc.DeleteDispatchRule(ctx, tenantGroupScope(ctx, in.GroupID), in.RuleID); err != nil {
				return nil, mapServiceError(err)
			}
			out := &deletedOutput{}
			out.Body.Deleted = true
			return out, nil
		})

	// ---- group targets (分组 → 上游目标) ----
	huma.Register(api, huma.Operation{OperationID: "ai-list-group-targets", Method: http.MethodGet, Path: "/api/v1/tenants/me/groups/{groupID}/targets", Summary: "分组关联上游目标列表", Tags: []string{"groups"}},
		func(ctx context.Context, in *groupIDInput) (*groupTargetsOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			items, err := d.CommercialSvc.ListGroupTargetDetails(ctx, tenantGroupScope(ctx, in.GroupID))
			if err != nil {
				return nil, mapServiceError(err)
			}
			out := &groupTargetsOutput{}
			out.Body.Items = make([]groupTargetDTO, 0, len(items))
			for _, item := range items {
				out.Body.Items = append(out.Body.Items, groupTargetToDTO(item))
			}
			out.Body.Total = len(out.Body.Items)
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-add-group-target", Method: http.MethodPost, Path: "/api/v1/tenants/me/groups/{groupID}/targets", Summary: "关联上游目标", Tags: []string{"groups"}},
		func(ctx context.Context, in *createGroupTargetInput) (*groupTargetOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			scope := tenantGroupScope(ctx, in.GroupID)
			target, err := d.CommercialSvc.AddGroupTarget(ctx, scope, groupTargetWriteFromRequest(in.Body))
			if err != nil {
				return nil, mapServiceError(err)
			}
			detail, err := d.CommercialSvc.GetGroupTargetDetail(ctx, scope, target.ID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupTargetOutput{Body: groupTargetToDTO(detail)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-update-group-target", Method: http.MethodPatch, Path: "/api/v1/tenants/me/groups/{groupID}/targets/{bindingID}", Summary: "更新关联(优先级/状态)", Tags: []string{"groups"}},
		func(ctx context.Context, in *updateGroupTargetInput) (*groupTargetOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			scope := tenantGroupScope(ctx, in.GroupID)
			existing, err := d.CommercialSvc.GetGroupTargetDetail(ctx, scope, in.BindingID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			target, err := d.CommercialSvc.UpdateGroupTarget(ctx, scope, in.BindingID, groupTargetUpdateWriteFromRequest(existing.GroupTarget, in.Body))
			if err != nil {
				return nil, mapServiceError(err)
			}
			detail, err := d.CommercialSvc.GetGroupTargetDetail(ctx, scope, target.ID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			return &groupTargetOutput{Body: groupTargetToDTO(detail)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-delete-group-target", Method: http.MethodDelete, Path: "/api/v1/tenants/me/groups/{groupID}/targets/{bindingID}", Summary: "解除关联", Tags: []string{"groups"}},
		func(ctx context.Context, in *deleteGroupTargetInput) (*deletedOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			if err := d.CommercialSvc.DeleteGroupTarget(ctx, tenantGroupScope(ctx, in.GroupID), in.BindingID); err != nil {
				return nil, mapServiceError(err)
			}
			out := &deletedOutput{}
			out.Body.Deleted = true
			return out, nil
		})

	// ---- user bindings ----
	huma.Register(api, huma.Operation{OperationID: "ai-list-user-groups", Method: http.MethodGet, Path: "/api/v1/tenants/me/users/{userID}/groups", Summary: "用户分组绑定列表", Tags: []string{"groups"}},
		func(ctx context.Context, in *userGroupsListInput) (*userGroupsOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			tenantID := tenantIDFromContext(ctx)
			if err := ensureTenantOwnsEndUser(ctx, d.TenantEndUsers, tenantID, in.UserID); err != nil {
				return nil, err
			}
			items, err := d.CommercialSvc.ListUserBindings(ctx, tenantID, in.UserID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			groupMap, err := loadCommercialGroupMap(ctx, d.CommercialSvc, tenantID)
			if err != nil {
				return nil, mapServiceError(err)
			}
			out := &userGroupsOutput{}
			out.Body.Items = make([]userGroupDTO, 0, len(items))
			for _, binding := range items {
				out.Body.Items = append(out.Body.Items, userGroupToDTO(binding, groupMap[binding.GroupID]))
			}
			out.Body.Total = len(out.Body.Items)
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-upsert-user-group", Method: http.MethodPut, Path: "/api/v1/tenants/me/users/{userID}/groups/{groupID}", Summary: "绑定/更新用户分组", Tags: []string{"groups"}},
		func(ctx context.Context, in *upsertUserGroupInput) (*deletedOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			tenantID := tenantIDFromContext(ctx)
			if err := ensureTenantOwnsEndUser(ctx, d.TenantEndUsers, tenantID, in.UserID); err != nil {
				return nil, err
			}
			if _, err := d.CommercialSvc.UpsertUserBinding(ctx, commercial.UserGroupBindingWrite{
				TenantID:               tenantID,
				UserID:                 in.UserID,
				GroupID:                in.GroupID,
				UserMultiplierOverride: in.Body.UserMultiplierOverride,
			}); err != nil {
				return nil, mapServiceError(err)
			}
			out := &deletedOutput{}
			out.Body.Deleted = true
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "ai-delete-user-group", Method: http.MethodDelete, Path: "/api/v1/tenants/me/users/{userID}/groups/{groupID}", Summary: "解绑用户分组", Tags: []string{"groups"}},
		func(ctx context.Context, in *deleteUserGroupInput) (*deletedOutput, error) {
			if err := commercialSvcReady(); err != nil {
				return nil, err
			}
			tenantID := tenantIDFromContext(ctx)
			if err := ensureTenantOwnsEndUser(ctx, d.TenantEndUsers, tenantID, in.UserID); err != nil {
				return nil, err
			}
			if err := d.CommercialSvc.DeleteUserBinding(ctx, tenantID, in.UserID, in.GroupID); err != nil {
				return nil, mapServiceError(err)
			}
			out := &deletedOutput{}
			out.Body.Deleted = true
			return out, nil
		})
}

func groupWriteFromReq(req groupWriteRequest) commercial.GroupWrite {
	defaultMultiplier := 1.0
	if req.DefaultUserMultiplier != nil {
		defaultMultiplier = *req.DefaultUserMultiplier
	}
	return commercial.GroupWrite{
		Name:                    req.Name,
		Description:             req.Description,
		RetailPriceBookID:       req.RetailPriceBookID,
		DefaultUserMultiplier:   defaultMultiplier,
		UserDefaultVisible:      req.UserDefaultVisible,
		AllowProtocolConversion: req.AllowProtocolConversion,
		SortOrder:               int(req.SortOrder),
		Status:                  commercial.Status(req.Status),
	}
}

func groupToDTO(group commercial.Group) groupDTO {
	return groupDTO{
		ID:                      group.ID,
		TenantID:                group.TenantID,
		Name:                    group.Name,
		Description:             group.Description,
		RetailPriceBookID:       group.RetailPriceBookID,
		DefaultUserMultiplier:   group.DefaultUserMultiplier,
		UserDefaultVisible:      group.UserDefaultVisible,
		AllowProtocolConversion: group.AllowProtocolConversion,
		SortOrder:               int32(group.SortOrder),
		Status:                  string(group.Status),
		CreatedAt:               timeToMillisPtr(group.CreatedAt),
		UpdatedAt:               timeToMillisPtr(group.UpdatedAt),
	}
}

func groupDispatchRuleToDTO(rule commercial.DispatchRule) groupDispatchRuleDTO {
	return groupDispatchRuleDTO{
		ID:                 rule.ID,
		GroupID:            rule.GroupID,
		ClientSurface:      string(rule.ClientSurface),
		MatchType:          string(rule.MatchType),
		MatchValue:         rule.MatchValue,
		TargetModelCode:    rule.TargetModelID,
		Priority:           int32(rule.Priority),
		Status:             string(rule.Status),
		Notes:              rule.Notes,
		RequiredCapability: rule.RequiredCapability,
		PriceState:         rule.PriceState,
		CanEnable:          rule.CanEnable,
		CreatedAt:          timeToMillisPtr(rule.CreatedAt),
		UpdatedAt:          timeToMillisPtr(rule.UpdatedAt),
	}
}

func groupDispatchPreviewToDTO(preview commercial.DispatchPreview) groupDispatchPreviewDTO {
	out := groupDispatchPreviewDTO{
		RequestedModel:       preview.RequestedModel,
		ClientSurface:        preview.ClientSurface,
		ResolvedLogicalModel: preview.ResolvedModelID,
		CandidateUpstreams:   make([]groupDispatchPreviewCandidateDTO, 0, len(preview.CandidateUpstreams)),
		RejectedCandidates:   make([]groupDispatchPreviewRejectionDTO, 0, len(preview.RejectedCandidates)),
	}
	if preview.MatchedRule != nil {
		dto := groupDispatchRuleToDTO(*preview.MatchedRule)
		out.MatchedRule = &dto
	}
	for _, item := range preview.CandidateUpstreams {
		out.CandidateUpstreams = append(out.CandidateUpstreams, groupDispatchPreviewCandidateDTO{
			TargetType:         item.TargetType,
			AccountID:          item.AccountID,
			CredentialPoolID:   item.CredentialPoolID,
			DisplayName:        item.DisplayName,
			ProviderFamily:     item.ProviderFamily,
			ProviderAPIFormat:  item.SelectedProtocol,
			ProtocolConversion: item.ProtocolConversion,
			Priority:           int32(item.Priority),
		})
	}
	for _, rejected := range preview.RejectedCandidates {
		out.RejectedCandidates = append(out.RejectedCandidates, groupDispatchPreviewRejectionDTO{
			TargetType:    rejected.TargetType,
			TargetID:      rejected.TargetID,
			ResolvedModel: rejected.ResolvedModelID,
			ReasonCode:    rejected.ReasonCode,
			ReasonDetail:  rejected.ReasonDetail,
			Priority:      int32(rejected.Priority),
		})
	}
	return out
}

func userGroupToDTO(binding commercial.UserGroupBinding, group commercial.Group) userGroupDTO {
	return userGroupDTO{
		GroupID:                binding.GroupID,
		GroupName:              group.Name,
		UserMultiplierOverride: binding.UserMultiplierOverride,
	}
}

func loadCommercialGroupMap(ctx context.Context, svc *commercial.Service, tenantID string) (map[string]commercial.Group, error) {
	items, err := svc.ListGroups(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]commercial.Group, len(items))
	for _, item := range items {
		out[item.ID] = item
	}
	return out, nil
}

func loadPriceBookNameMap(ctx context.Context, d AIDeps, tenantID string) (map[string]string, error) {
	if d.TenantPriceBooks == nil {
		return map[string]string{}, nil
	}
	items, err := d.TenantPriceBooks.ListVisiblePriceBooks(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		out[item.ID] = item.Name
	}
	return out, nil
}

func dispatchRuleWriteFromRequest(req groupDispatchRuleWriteRequest) (commercial.DispatchRuleWrite, error) {
	if !surface.IsKnown(surface.ID(req.ClientSurface)) {
		return commercial.DispatchRuleWrite{}, domain.NewValidationError("client_surface", "unsupported client_surface")
	}
	priority := int32OrDefault(req.Priority, 100)
	return commercial.DispatchRuleWrite{
		ClientSurface: surface.ID(req.ClientSurface),
		MatchType:     commercial.DispatchMatchType(req.MatchType),
		MatchValue:    req.MatchValue,
		TargetModelID: req.TargetModelCode,
		Priority:      int(priority),
		Notes:         req.Notes,
	}, nil
}

func groupTargetWriteFromRequest(req groupTargetWriteRequest) commercial.GroupTargetWrite {
	targetKind := commercial.TargetKindDirectUpstream
	targetID := req.AccountID
	if req.CredentialPoolID != "" {
		targetKind = commercial.TargetKindOAuthPool
		targetID = req.CredentialPoolID
	}
	return commercial.GroupTargetWrite{
		TargetKind: targetKind,
		TargetID:   targetID,
		Priority:   int(int32OrDefault(req.Priority, 100)),
		Status:     commercial.Status(req.Status),
	}
}

func groupClientSurfacePolicyToDTO(policy commercial.GroupClientSurfacePolicy) groupClientSurfacePolicyDTO {
	allowed := make([]string, 0, len(policy.AllowedSurfaces))
	for _, item := range policy.AllowedSurfaces {
		allowed = append(allowed, string(item))
	}
	return groupClientSurfacePolicyDTO{
		GroupID:         policy.GroupID,
		Mode:            string(policy.Mode),
		AllowedSurfaces: allowed,
	}
}

func groupTargetUpdateWriteFromRequest(existing commercial.GroupTarget, req groupTargetUpdateRequest) commercial.GroupTargetWrite {
	status := existing.Status
	if req.Status != "" {
		status = commercial.Status(req.Status)
	}
	return commercial.GroupTargetWrite{
		Priority: int(int32OrDefault(req.Priority, int32(existing.Priority))),
		Status:   status,
	}
}

func tenantGroupScope(ctx context.Context, groupID string) commercial.TenantGroupScope {
	return commercial.TenantGroupScope{TenantID: tenantIDFromContext(ctx), GroupID: groupID}
}

func providerFamilyToAPI(family string) string {
	switch family {
	case "google":
		return "gemini"
	default:
		return family
	}
}

func int32OrDefault(v *int32, fallback int32) int32 {
	if v == nil {
		return fallback
	}
	return *v
}

func providerFamilyFromAPI(family string) string {
	switch family {
	case "gemini":
		return "google"
	default:
		return family
	}
}

func groupTargetToDTO(item commercial.GroupTargetDetail) groupTargetDTO {
	accountID := ""
	credentialPoolID := ""
	targetType := "account"
	if item.TargetKind == commercial.TargetKindOAuthPool {
		targetType = "pool"
		credentialPoolID = item.TargetID
	} else {
		accountID = item.TargetID
	}
	return groupTargetDTO{
		ID:                    item.ID,
		GroupID:               item.GroupID,
		AccountID:             accountID,
		CredentialPoolID:      credentialPoolID,
		Priority:              int32(item.Priority),
		Status:                string(item.Status),
		TargetType:            targetType,
		AccountName:           item.AccountName,
		DefaultProviderFamily: item.DefaultProtocol,
		PoolName:              item.PoolName,
		FixedProviderType:     item.FixedProviderType,
		Available:             item.Available,
		UnavailableReason:     item.UnavailableReason,
		CreatedAt:             timeToMillisPtr(item.CreatedAt),
		UpdatedAt:             timeToMillisPtr(item.UpdatedAt),
	}
}

package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

// appDTO 是管理侧(owner 视角)的应用全量视图。
type appDTO struct {
	OwnerType         string                `json:"owner_type"`
	OwnerTenantID     string                `json:"owner_tenant_id,omitempty"`
	OwnerUserID       string                `json:"owner_user_id,omitempty"`
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	Description       string                `json:"description"`
	Status            string                `json:"status"`
	Capability        string                `json:"capability"`
	PromptStrategy    string                `json:"prompt_strategy"`
	PromptBindings    []appPromptBindingDTO `json:"prompt_bindings"`
	GroupID           string                `json:"group_id"`
	ModelCode         string                `json:"model_code"`
	RuntimeConfig     map[string]any        `json:"runtime_config"`
	PublishedByTenant bool                  `json:"published_by_tenant"`
	CreatedBy         *string               `json:"created_by,omitempty"`
	UpdatedBy         *string               `json:"updated_by,omitempty"`
	CreatedAt         *int64                `json:"created_at,omitempty"`
	UpdatedAt         *int64                `json:"updated_at,omitempty"`
}

type appPromptBindingDTO struct {
	PromptID     string   `json:"prompt_id"`
	PromptName   string   `json:"prompt_name"`
	Variables    []string `json:"variables"`
	BindingRole  string   `json:"binding_role"`
	DisplayOrder int      `json:"display_order"`
}

// consumerAppDTO 是使用侧(选择器)脱敏视图:不暴露模型、分组、提示词等底层实现。
type consumerAppDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Capability     string   `json:"capability"`
	PromptStrategy string   `json:"prompt_strategy"`
	PublisherLabel string   `json:"publisher_label"`
	Variables      []string `json:"variables"`
	PromptNames    []string `json:"prompt_names"`
}

type appsOutput struct {
	Body struct {
		Items []appDTO `json:"items"`
		Total int      `json:"total"`
	}
}

type appOutput struct {
	Body appDTO
}

type appDeleteOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

type appPublicationOutput struct {
	Body struct {
		Published bool `json:"published"`
	}
}

type appWriteRequest struct {
	TemplateID     string         `json:"template_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Status         string         `json:"status"`
	Capability     string         `json:"capability"`
	PromptStrategy string         `json:"prompt_strategy"`
	PromptIDs      []string       `json:"prompt_ids"`
	GroupID        string         `json:"group_id"`
	ModelCode      string         `json:"model_code"`
	RuntimeConfig  map[string]any `json:"runtime_config"`
}

type appTemplateDTO struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	DefaultCapability   string   `json:"default_capability"`
	AllowedCapabilities []string `json:"allowed_capabilities"`
	PromptStrategy      string   `json:"prompt_strategy"`
	MinPromptBindings   int      `json:"min_prompt_bindings"`
	MaxPromptBindings   int      `json:"max_prompt_bindings"`
}

type appTemplatesOutput struct {
	Body struct {
		Items []appTemplateDTO `json:"items"`
	}
}

type appCreateInput struct {
	Body appWriteRequest
}

type appUpdateInput struct {
	AgentID string `path:"agentID"`
	Body    struct {
		Name           *string        `json:"name,omitempty"`
		Description    *string        `json:"description,omitempty"`
		Status         *string        `json:"status,omitempty"`
		Capability     *string        `json:"capability,omitempty"`
		PromptStrategy *string        `json:"prompt_strategy,omitempty"`
		PromptIDs      *[]string      `json:"prompt_ids,omitempty"`
		GroupID        *string        `json:"group_id,omitempty"`
		ModelCode      *string        `json:"model_code,omitempty"`
		RuntimeConfig  map[string]any `json:"runtime_config,omitempty"`
	}
}

type appAgentIDInput struct {
	AgentID string `path:"agentID"`
}

type appWrite struct {
	Name           string
	Description    string
	Status         string
	Capability     string
	PromptStrategy string
	PromptIDs      []string
	GroupID        string
	ModelCode      string
	RuntimeConfig  map[string]any
}

type appPatch struct {
	Name           *string
	Description    *string
	Status         *string
	Capability     *string
	PromptStrategy *string
	PromptIDs      *[]string
	GroupID        *string
	ModelCode      *string
	RuntimeConfig  *map[string]any
}

func registerTenantSelfAppAgents(api huma.API, d AIDeps) {
	registerScopedAppAgents(api, d, tenantAppScope, "/api/v1/tenants/me/app-layer/agents", "租户应用", []string{"tenant-app-agents"})
	registerTenantAppPublications(api, d)
}

func registerUserSelfAppAgents(api huma.API, d AIDeps) {
	registerScopedAppAgents(api, d, userAppScope, "/api/v1/users/me/app-layer/agents", "用户应用", []string{"user-app-agents"})
}

func registerScopedAppAgents(
	api huma.API,
	d AIDeps,
	scopeResolver func(context.Context) appScope,
	pathBase string,
	summaryPrefix string,
	tags []string,
) {
	templatePath := strings.TrimSuffix(pathBase, "/agents") + "/templates"
	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(templatePath, "/", "-") + "-list",
		Method:      http.MethodGet,
		Path:        templatePath,
		Summary:     "应用创建模板",
		Description: "返回代码内置的固定应用创建逻辑，不读取或管理数据库模板。",
		Tags:        tags,
	}, func(_ context.Context, _ *struct{}) (*appTemplatesOutput, error) {
		out := &appTemplatesOutput{}
		for _, template := range application.ListAppTemplates() {
			capabilities := make([]string, 0, len(template.AllowedCapabilities))
			for _, capability := range template.AllowedCapabilities {
				capabilities = append(capabilities, string(capability))
			}
			out.Body.Items = append(out.Body.Items, appTemplateDTO{
				ID: string(template.ID), Name: template.Name, Description: template.Description,
				DefaultCapability: string(template.DefaultCapability), AllowedCapabilities: capabilities,
				PromptStrategy: string(template.PromptStrategy), MinPromptBindings: template.MinPromptBindings,
				MaxPromptBindings: template.MaxPromptBindings,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(pathBase, "/", "-") + "-list",
		Method:      http.MethodGet,
		Path:        pathBase,
		Summary:     summaryPrefix + "列表",
		Tags:        tags,
	}, func(ctx context.Context, _ *struct{}) (*appsOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		scope := scopeResolver(ctx)
		items, err := listOwnedApps(ctx, d, scope)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &appsOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(pathBase, "/", "-") + "-get",
		Method:      http.MethodGet,
		Path:        pathBase + "/{agentID}",
		Summary:     summaryPrefix + "详情",
		Tags:        tags,
	}, func(ctx context.Context, in *appAgentIDInput) (*appOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		scope := scopeResolver(ctx)
		item, err := getApp(ctx, d, scope, strings.TrimSpace(in.AgentID))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &appOutput{}
		out.Body = item
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   strings.ReplaceAll(pathBase, "/", "-") + "-create",
		Method:        http.MethodPost,
		Path:          pathBase,
		Summary:       "创建" + summaryPrefix,
		Tags:          tags,
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *appCreateInput) (*appOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		write, err := normalizeAppWrite(in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		actor := strings.TrimSpace(claimsUserID(ctx))
		scope := scopeResolver(ctx)
		item, err := createApp(ctx, d, scope, write, actor)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &appOutput{}
		out.Body = item
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(pathBase, "/", "-") + "-update",
		Method:      http.MethodPatch,
		Path:        pathBase + "/{agentID}",
		Summary:     "更新" + summaryPrefix,
		Tags:        tags,
	}, func(ctx context.Context, in *appUpdateInput) (*appOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		write, err := normalizeAppPatch(in)
		if err != nil {
			return nil, mapServiceError(err)
		}
		actor := strings.TrimSpace(claimsUserID(ctx))
		scope := scopeResolver(ctx)
		item, err := updateApp(ctx, d, scope, strings.TrimSpace(in.AgentID), write, actor)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &appOutput{}
		out.Body = item
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(pathBase, "/", "-") + "-delete",
		Method:      http.MethodDelete,
		Path:        pathBase + "/{agentID}",
		Summary:     "删除" + summaryPrefix,
		Tags:        tags,
	}, func(ctx context.Context, in *appAgentIDInput) (*appDeleteOutput, error) {
		scope := scopeResolver(ctx)
		if err := validateAppScope(scope); err != nil {
			return nil, mapServiceError(err)
		}
		if err := deleteApp(ctx, d, scope, strings.TrimSpace(in.AgentID)); err != nil {
			return nil, mapServiceError(err)
		}
		out := &appDeleteOutput{}
		out.Body.Deleted = true
		return out, nil
	})
}

// registerTenantAppPublications 注册租户应用向本租户用户发布的能力。
func registerTenantAppPublications(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "tenant-app-agents-publish",
		Method:      http.MethodPut,
		Path:        "/api/v1/tenants/me/app-layer/agents/{agentID}/publication",
		Summary:     "发布应用给本租户终端用户",
		Tags:        []string{"tenant-app-agents"},
	}, func(ctx context.Context, in *appAgentIDInput) (*appPublicationOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		tenantID := strings.TrimSpace(tenantIDFromContext(ctx))
		if tenantID == "" {
			return nil, httpx.ErrForbidden.WithDetail("tenant id is required")
		}
		actor := strings.TrimSpace(claimsUserID(ctx))
		if err := pgadapter.NewApplicationAppRepo(d.Postgres).SetTenantPublication(ctx, tenantID, strings.TrimSpace(in.AgentID), actor); err != nil {
			return nil, mapServiceError(err)
		}
		out := &appPublicationOutput{}
		out.Body.Published = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "tenant-app-agents-unpublish",
		Method:      http.MethodDelete,
		Path:        "/api/v1/tenants/me/app-layer/agents/{agentID}/publication",
		Summary:     "撤回本租户的应用发布",
		Tags:        []string{"tenant-app-agents"},
	}, func(ctx context.Context, in *appAgentIDInput) (*appPublicationOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		tenantID := strings.TrimSpace(tenantIDFromContext(ctx))
		if tenantID == "" {
			return nil, httpx.ErrForbidden.WithDetail("tenant id is required")
		}
		if err := pgadapter.NewApplicationAppRepo(d.Postgres).RemoveTenantPublication(ctx, tenantID, strings.TrimSpace(in.AgentID)); err != nil {
			return nil, mapServiceError(err)
		}
		out := &appPublicationOutput{}
		out.Body.Published = false
		return out, nil
	})
}

// listVisibleApps 返回使用侧脱敏后的可见应用列表。
func listVisibleApps(ctx context.Context, d AIDeps, viewer pgadapter.AppViewer) ([]consumerAppDTO, error) {
	items, err := pgadapter.NewApplicationAppRepo(d.Postgres).ListVisibleAgents(ctx, viewer, []string{
		transportAppCapabilityChat,
		transportAppCapabilityImageGen,
		transportAppCapabilityImageEdit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]consumerAppDTO, 0, len(items))
	for _, item := range items {
		out = append(out, appRecordToConsumerDTO(item))
	}
	return out, nil
}

func listOwnedApps(ctx context.Context, d AIDeps, scope appScope) ([]appDTO, error) {
	if err := validateAppScope(scope); err != nil {
		return nil, err
	}
	rows, err := pgadapter.NewApplicationAppRepo(d.Postgres).ListOwnedAgents(ctx, scope.owner())
	if err != nil {
		return nil, err
	}
	out := make([]appDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, appRecordToDTO(row))
	}
	return out, nil
}

func getApp(ctx context.Context, d AIDeps, scope appScope, agentID string) (appDTO, error) {
	if err := validateAppScope(scope); err != nil {
		return appDTO{}, err
	}
	row, err := pgadapter.NewApplicationAppRepo(d.Postgres).GetOwnedAgent(ctx, scope.owner(), agentID)
	if err != nil {
		return appDTO{}, err
	}
	return appRecordToDTO(row), nil
}

func normalizeAppCapability(value string) (string, error) {
	capability := strings.TrimSpace(value)
	if capability == "" {
		return "", domain.NewValidationError("capability", "is required")
	}
	switch capability {
	case transportAppCapabilityChat, transportAppCapabilityImageGen, transportAppCapabilityImageEdit:
		return capability, nil
	default:
		return "", domain.NewValidationError("capability", "must be chat, image_generation or image_edit")
	}
}

func normalizeAppWrite(in appWriteRequest) (appWrite, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return appWrite{}, domain.NewValidationError("name", "is required")
	}
	promptIDs := normalizePromptIDs(in.PromptIDs)
	modelCode := strings.TrimSpace(in.ModelCode)
	if modelCode == "" {
		return appWrite{}, domain.NewValidationError("model_code", "is required")
	}
	groupID := strings.TrimSpace(in.GroupID)
	if groupID == "" {
		return appWrite{}, domain.NewValidationError("group_id", "is required")
	}
	capabilityInput := strings.TrimSpace(in.Capability)
	promptStrategyInput := strings.TrimSpace(in.PromptStrategy)
	if templateID := strings.TrimSpace(in.TemplateID); templateID != "" {
		template, ok := application.FindAppTemplate(templateID)
		if !ok {
			return appWrite{}, domain.NewValidationError("template_id", "is not a supported application template")
		}
		if capabilityInput == "" {
			capabilityInput = string(template.DefaultCapability)
		}
		if promptStrategyInput == "" {
			promptStrategyInput = string(template.PromptStrategy)
		}
	}
	capability, err := normalizeAppCapability(capabilityInput)
	if err != nil {
		return appWrite{}, err
	}
	promptStrategy, err := normalizePromptStrategy(promptStrategyInput)
	if err != nil {
		return appWrite{}, err
	}
	if templateID := strings.TrimSpace(in.TemplateID); templateID != "" {
		template, ok := application.FindAppTemplate(templateID)
		if !ok {
			return appWrite{}, domain.NewValidationError("template_id", "is not a supported application template")
		}
		if !template.AllowsCapability(application.AppCapability(capability)) {
			return appWrite{}, domain.NewValidationError("capability", "is not supported by the selected template")
		}
		if promptStrategy != string(template.PromptStrategy) {
			return appWrite{}, domain.NewValidationError("prompt_strategy", "does not match the selected template")
		}
		promptStrategy = string(template.PromptStrategy)
		if len(promptIDs) < template.MinPromptBindings || template.MaxPromptBindings >= 0 && len(promptIDs) > template.MaxPromptBindings {
			return appWrite{}, domain.NewValidationError("prompt_ids", "count does not match the selected template")
		}
	}
	status, err := normalizePromptStatus(in.Status)
	if err != nil {
		return appWrite{}, err
	}
	return appWrite{
		Name:           name,
		Description:    strings.TrimSpace(in.Description),
		Status:         status,
		Capability:     capability,
		PromptStrategy: promptStrategy,
		PromptIDs:      promptIDs,
		GroupID:        groupID,
		ModelCode:      modelCode,
		RuntimeConfig:  normalizeAppRuntimeConfig(capability, in.RuntimeConfig),
	}, nil
}

func normalizeAppPatch(in *appUpdateInput) (appPatch, error) {
	var out appPatch
	if in.Body.Name != nil {
		value := strings.TrimSpace(*in.Body.Name)
		if value == "" {
			return out, domain.NewValidationError("name", "cannot be empty")
		}
		out.Name = &value
	}
	if in.Body.Description != nil {
		value := strings.TrimSpace(*in.Body.Description)
		out.Description = &value
	}
	if in.Body.Status != nil {
		value, err := normalizePromptStatus(*in.Body.Status)
		if err != nil {
			return out, err
		}
		out.Status = &value
	}
	if in.Body.Capability != nil {
		value, err := normalizeAppCapability(*in.Body.Capability)
		if err != nil {
			return out, err
		}
		out.Capability = &value
	}
	if in.Body.PromptStrategy != nil {
		value, err := normalizePromptStrategy(*in.Body.PromptStrategy)
		if err != nil {
			return out, err
		}
		out.PromptStrategy = &value
	}
	if in.Body.PromptIDs != nil {
		values := normalizePromptIDs(*in.Body.PromptIDs)
		out.PromptIDs = &values
	}
	if in.Body.GroupID != nil {
		value := strings.TrimSpace(*in.Body.GroupID)
		if value == "" {
			return out, domain.NewValidationError("group_id", "cannot be empty")
		}
		out.GroupID = &value
	}
	if in.Body.ModelCode != nil {
		value := strings.TrimSpace(*in.Body.ModelCode)
		if value == "" {
			return out, domain.NewValidationError("model_code", "cannot be empty")
		}
		out.ModelCode = &value
	}
	if in.Body.RuntimeConfig != nil {
		cfg := in.Body.RuntimeConfig
		out.RuntimeConfig = &cfg
	}
	if out.Name == nil && out.Description == nil && out.Status == nil && out.Capability == nil &&
		out.PromptStrategy == nil && out.PromptIDs == nil && out.GroupID == nil && out.ModelCode == nil && out.RuntimeConfig == nil {
		return out, domain.NewValidationError("", "no fields to update")
	}
	return out, nil
}

func normalizePromptStrategy(value string) (string, error) {
	strategy := strings.TrimSpace(value)
	if strategy == "" {
		return "", domain.NewValidationError("prompt_strategy", "is required")
	}
	switch strategy {
	case pgadapter.PromptStrategyNone, pgadapter.PromptStrategyCallerVariables, pgadapter.PromptStrategyBoundExact:
		return strategy, nil
	default:
		return "", domain.NewValidationError("prompt_strategy", "must be none, caller_variables or bound_prompt_exact")
	}
}

func normalizePromptIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// normalizeAppRuntimeConfig runs the incoming payload through the single shared
// parser so only well-formed, closed config (no stray keys) is ever persisted.
func normalizeAppRuntimeConfig(capability string, value map[string]any) map[string]any {
	return application.ParseRuntimeConfig(appTypeForCapability(capability), value).ToStored()
}

func appTypeForCapability(capability string) application.AppType {
	switch strings.TrimSpace(capability) {
	case transportAppCapabilityImageGen:
		return application.AppTypeImageGenerationAgent
	case transportAppCapabilityImageEdit:
		return application.AppTypeImageEditAgent
	default:
		return application.AppTypeChatAgent
	}
}

func modelCapabilityForApp(capability string) (string, error) {
	switch strings.TrimSpace(capability) {
	case transportAppCapabilityChat:
		return string(domain.CapabilityChat), nil
	case transportAppCapabilityImageGen, transportAppCapabilityImageEdit:
		return string(domain.CapabilityImage), nil
	default:
		return "", domain.NewValidationError("capability", "must be chat, image_generation or image_edit")
	}
}

// validateAppModelBinding 校验租户/用户应用只能绑定本租户拥有的分组；
// 用户应用还要求分组默认开放，或该用户有显式例外绑定。
//
// 运行时对应用绑定的分组做 forced-group bypass,不再重复校验调用者可见性,
// 因此这里是应用→分组授权的唯一闸口。
func validateAppModelBinding(ctx context.Context, d AIDeps, scope appScope, capability, groupID, modelCode string) error {
	capabilityType, err := modelCapabilityForApp(capability)
	if err != nil {
		return err
	}
	var ok int
	err = d.Postgres.QueryRow(ctx, `
		SELECT 1
		FROM ai_groups g
		JOIN ai_group_targets gt
		  ON gt.group_id = g.id
		 AND gt.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.status = 'active'
		 AND um.model_code = $2
		 AND um.capability_type = $3
		JOIN ai_price_book_entries e
		  ON e.price_book_id = g.retail_price_book_id
		 AND e.model_code = um.model_code
		 AND e.capability_type = um.capability_type
		LEFT JOIN ai_user_groups ug
		  ON ug.group_id = g.id
		 AND ug.tenant_id = $5
		 AND ug.user_id = $6
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream'
		 AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool'
		 AND cp.id = gt.target_id
		JOIN ai_price_book_entries account_e
		  ON account_e.price_book_id = COALESCE(a.price_book_id, cp.price_book_id)
		 AND account_e.model_code = um.model_code
		 AND account_e.capability_type = um.capability_type
		WHERE g.id = $1::uuid
		  AND g.tenant_id = $5
		  AND g.status = 'active'
		  AND (
		    $4 = 'tenant'
		    OR (
		      $4 = 'user'
		      AND (g.user_default_visible OR ug.id IS NOT NULL)
		    )
		  )
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		  AND (
		    COALESCE(a.tenant_access_mode, cp.tenant_access_mode) = 'public'
		    OR EXISTS (
		      SELECT 1 FROM ai_upstream_resource_tenant_policies rg
		      WHERE rg.resource_kind = gt.target_kind AND rg.resource_id = gt.target_id AND rg.tenant_id = $5
		        AND rg.access_granted
		    )
		  )
		LIMIT 1
	`, groupID, modelCode, capabilityType, scope.OwnerType, scope.OwnerTenantID, scope.OwnerUserID).Scan(&ok)
	if err == pgx.ErrNoRows {
		return domain.NewValidationError("model_code", "must reference an active model with matching capability in the selected group")
	}
	return err
}

func createApp(ctx context.Context, d AIDeps, scope appScope, write appWrite, actor string) (appDTO, error) {
	if err := validateAppScope(scope); err != nil {
		return appDTO{}, err
	}
	if err := validateAppModelBinding(ctx, d, scope, write.Capability, write.GroupID, write.ModelCode); err != nil {
		return appDTO{}, err
	}
	row, err := pgadapter.NewApplicationAppRepo(d.Postgres).CreateOwnedAgent(
		ctx,
		scope.owner(),
		write.Name,
		write.Description,
		write.Status,
		write.Capability,
		write.PromptStrategy,
		write.PromptIDs,
		write.GroupID,
		write.ModelCode,
		write.RuntimeConfig,
		actor,
	)
	if err != nil {
		return appDTO{}, err
	}
	return appRecordToDTO(row), nil
}

func updateApp(ctx context.Context, d AIDeps, scope appScope, agentID string, patch appPatch, actor string) (appDTO, error) {
	if err := validateAppScope(scope); err != nil {
		return appDTO{}, err
	}
	current, err := getApp(ctx, d, scope, agentID)
	if err != nil {
		return appDTO{}, err
	}

	name := current.Name
	if patch.Name != nil {
		name = *patch.Name
	}
	description := current.Description
	if patch.Description != nil {
		description = *patch.Description
	}
	status := current.Status
	if patch.Status != nil {
		status = *patch.Status
	}
	capability := current.Capability
	if patch.Capability != nil {
		capability = *patch.Capability
	}
	promptStrategy := current.PromptStrategy
	if patch.PromptStrategy != nil {
		promptStrategy = *patch.PromptStrategy
	}
	promptIDs := make([]string, 0, len(current.PromptBindings))
	for _, binding := range current.PromptBindings {
		promptIDs = append(promptIDs, binding.PromptID)
	}
	if patch.PromptIDs != nil {
		promptIDs = *patch.PromptIDs
	}
	modelCode := current.ModelCode
	if patch.ModelCode != nil {
		modelCode = *patch.ModelCode
	}
	groupID := current.GroupID
	if patch.GroupID != nil {
		groupID = *patch.GroupID
	}
	runtimeConfig := current.RuntimeConfig
	if patch.RuntimeConfig != nil {
		runtimeConfig = *patch.RuntimeConfig
	}
	runtimeConfig = normalizeAppRuntimeConfig(capability, runtimeConfig)
	if patchRequiresAppModelValidation(patch) {
		if err := validateAppModelBinding(ctx, d, scope, capability, groupID, modelCode); err != nil {
			return appDTO{}, err
		}
	}

	row, err := pgadapter.NewApplicationAppRepo(d.Postgres).UpdateOwnedAgent(
		ctx,
		scope.owner(),
		agentID,
		name,
		description,
		status,
		capability,
		promptStrategy,
		promptIDs,
		groupID,
		modelCode,
		runtimeConfig,
		actor,
	)
	if err != nil {
		return appDTO{}, err
	}
	return appRecordToDTO(row), nil
}

func patchRequiresAppModelValidation(patch appPatch) bool {
	if patch.Capability != nil || patch.GroupID != nil || patch.ModelCode != nil {
		return true
	}
	return patch.Status != nil && *patch.Status == "active"
}

func appRecordToDTO(row pgadapter.AppAgentRecord) appDTO {
	bindings := make([]appPromptBindingDTO, 0, len(row.PromptBindings))
	for _, binding := range row.PromptBindings {
		bindings = append(bindings, appPromptBindingDTO{
			PromptID: binding.PromptID, PromptName: binding.PromptName,
			Variables: decodeStringArray(binding.Variables), BindingRole: binding.BindingRole,
			DisplayOrder: int(binding.DisplayOrder),
		})
	}
	return appDTO{
		OwnerType:         row.OwnerType,
		OwnerTenantID:     row.OwnerTenantID,
		OwnerUserID:       row.OwnerUserID,
		ID:                row.ID,
		Name:              row.Name,
		Description:       row.Description,
		Status:            row.Status,
		Capability:        row.Capability,
		PromptStrategy:    row.PromptStrategy,
		PromptBindings:    bindings,
		GroupID:           row.GroupID,
		ModelCode:         row.ModelCode,
		RuntimeConfig:     normalizeAppRuntimeConfig(row.Capability, decodeJSONObject(row.DefaultOptions)),
		PublishedByTenant: row.PublishedByTenant,
		CreatedBy:         row.CreatedBy,
		UpdatedBy:         row.UpdatedBy,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func appRecordToConsumerDTO(row pgadapter.AppAgentRecord) consumerAppDTO {
	variables := []string{}
	promptNames := make([]string, 0, len(row.PromptBindings))
	for _, binding := range row.PromptBindings {
		promptNames = append(promptNames, binding.PromptName)
		if binding.BindingRole == "primary" {
			variables = decodeStringArray(binding.Variables)
		}
	}
	return consumerAppDTO{
		ID:             row.ID,
		Name:           row.Name,
		Description:    row.Description,
		Capability:     row.Capability,
		PromptStrategy: row.PromptStrategy,
		PublisherLabel: appPublisherLabel(row),
		Variables:      variables,
		PromptNames:    promptNames,
	}
}

func decodeStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func appPublisherLabel(row pgadapter.AppAgentRecord) string {
	switch row.OwnerType {
	case transportAppOwnerTenant:
		if strings.TrimSpace(row.OwnerTenantID) != "" {
			return row.OwnerTenantID
		}
		return "租户"
	case transportAppOwnerUser:
		return "我"
	default:
		return "应用"
	}
}

func deleteApp(ctx context.Context, d AIDeps, scope appScope, agentID string) error {
	return pgadapter.NewApplicationAppRepo(d.Postgres).DeleteOwnedAgent(ctx, scope.owner(), agentID)
}

func decodeJSONObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

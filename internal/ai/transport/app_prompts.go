package transport

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

type appPromptDTO struct {
	OwnerType     string   `json:"owner_type"`
	OwnerTenantID string   `json:"owner_tenant_id,omitempty"`
	OwnerUserID   string   `json:"owner_user_id,omitempty"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Status        string   `json:"status"`
	TemplateText  string   `json:"template_text"`
	Variables     []string `json:"variables"`
	CreatedBy     *string  `json:"created_by,omitempty"`
	UpdatedBy     *string  `json:"updated_by,omitempty"`
	CreatedAt     *int64   `json:"created_at,omitempty"`
	UpdatedAt     *int64   `json:"updated_at,omitempty"`
}

type appPromptDetailDTO struct {
	Prompt appPromptDTO `json:"prompt"`
}

type appPromptsOutput struct {
	Body struct {
		Items []appPromptDTO `json:"items"`
		Total int            `json:"total"`
	}
}

type appPromptOutput struct {
	Body appPromptDetailDTO
}

type appPromptDeleteOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

type appPromptCreateInput struct {
	Body struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Status       string `json:"status"`
		TemplateText string `json:"template_text"`
	}
}

type appPromptUpdateInput struct {
	PromptID string `path:"promptID"`
	Body     struct {
		Name         *string `json:"name,omitempty"`
		Description  *string `json:"description,omitempty"`
		Status       *string `json:"status,omitempty"`
		TemplateText *string `json:"template_text,omitempty"`
	}
}

type appPromptIDInput struct {
	PromptID string `path:"promptID"`
}

func registerTenantSelfAppPrompts(api huma.API, d AIDeps) {
	registerScopedAppPrompts(api, d, tenantAppScope, "/api/v1/tenants/me/app-layer/prompts", "租户提示词", []string{"tenant-app-prompts"})
}

func registerUserSelfAppPrompts(api huma.API, d AIDeps) {
	registerScopedAppPrompts(api, d, userAppScope, "/api/v1/users/me/app-layer/prompts", "用户提示词", []string{"user-app-prompts"})
}

func registerScopedAppPrompts(
	api huma.API,
	d AIDeps,
	scopeResolver func(context.Context) appScope,
	pathBase string,
	summaryPrefix string,
	tags []string,
) {
	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(pathBase, "/", "-") + "-list",
		Method:      http.MethodGet,
		Path:        pathBase,
		Summary:     summaryPrefix + "列表",
		Description: "应用层提示词资产列表，返回当前内容与变量摘要。",
		Tags:        tags,
	}, func(ctx context.Context, _ *struct{}) (*appPromptsOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		scope := scopeResolver(ctx)
		items, err := listAppPrompts(ctx, d, scope)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &appPromptsOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(pathBase, "/", "-") + "-get",
		Method:      http.MethodGet,
		Path:        pathBase + "/{promptID}",
		Summary:     summaryPrefix + "详情",
		Tags:        tags,
	}, func(ctx context.Context, in *appPromptIDInput) (*appPromptOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		scope := scopeResolver(ctx)
		detail, err := getAppPromptDetail(ctx, d, scope, strings.TrimSpace(in.PromptID))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &appPromptOutput{}
		out.Body = detail
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   strings.ReplaceAll(pathBase, "/", "-") + "-create",
		Method:        http.MethodPost,
		Path:          pathBase,
		Summary:       "创建" + summaryPrefix,
		Tags:          tags,
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *appPromptCreateInput) (*appPromptOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		write, err := normalizePromptCreate(in)
		if err != nil {
			return nil, mapServiceError(err)
		}
		actor := strings.TrimSpace(claimsUserID(ctx))
		scope := scopeResolver(ctx)
		detail, err := createAppPrompt(ctx, d, scope, write, actor)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &appPromptOutput{}
		out.Body = detail
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(pathBase, "/", "-") + "-update",
		Method:      http.MethodPatch,
		Path:        pathBase + "/{promptID}",
		Summary:     "更新" + summaryPrefix,
		Description: "更新提示词后，所有绑定该提示词的应用会在后续调用中使用新内容。",
		Tags:        tags,
	}, func(ctx context.Context, in *appPromptUpdateInput) (*appPromptOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		write, err := normalizePromptUpdate(in)
		if err != nil {
			return nil, mapServiceError(err)
		}
		actor := strings.TrimSpace(claimsUserID(ctx))
		scope := scopeResolver(ctx)
		detail, err := updateAppPrompt(ctx, d, scope, strings.TrimSpace(in.PromptID), write, actor)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &appPromptOutput{}
		out.Body = detail
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: strings.ReplaceAll(pathBase, "/", "-") + "-delete",
		Method:      http.MethodDelete,
		Path:        pathBase + "/{promptID}",
		Summary:     "删除" + summaryPrefix,
		Tags:        tags,
	}, func(ctx context.Context, in *appPromptIDInput) (*appPromptDeleteOutput, error) {
		if d.Postgres == nil {
			return nil, httpx.ErrUnavailable.WithDetail("postgres is not configured")
		}
		scope := scopeResolver(ctx)
		if err := validateAppScope(scope); err != nil {
			return nil, mapServiceError(err)
		}
		if err := deleteAppPrompt(ctx, d, scope, strings.TrimSpace(in.PromptID)); err != nil {
			return nil, mapServiceError(err)
		}
		out := &appPromptDeleteOutput{}
		out.Body.Deleted = true
		return out, nil
	})
}

type promptCreateWrite struct {
	Name         string
	Description  string
	Status       string
	TemplateText string
}

type promptUpdateWrite struct {
	Name         *string
	Description  *string
	Status       *string
	TemplateText *string
}

func normalizePromptCreate(in *appPromptCreateInput) (promptCreateWrite, error) {
	name, err := application.NormalizePromptName(in.Body.Name)
	if err != nil {
		return promptCreateWrite{}, domain.NewValidationError("name", err.Error())
	}
	templateText := strings.TrimSpace(in.Body.TemplateText)
	if templateText == "" {
		return promptCreateWrite{}, domain.NewValidationError("template_text", "is required")
	}
	if _, err := application.ExtractPromptVariables(templateText); err != nil {
		return promptCreateWrite{}, domain.NewValidationError("template_text", err.Error())
	}
	status, err := normalizePromptStatus(in.Body.Status)
	if err != nil {
		return promptCreateWrite{}, err
	}
	return promptCreateWrite{
		Name:         name,
		Description:  strings.TrimSpace(in.Body.Description),
		Status:       status,
		TemplateText: templateText,
	}, nil
}

func normalizePromptUpdate(in *appPromptUpdateInput) (promptUpdateWrite, error) {
	var out promptUpdateWrite
	if in.Body.Name != nil {
		name, err := application.NormalizePromptName(*in.Body.Name)
		if err != nil {
			return out, domain.NewValidationError("name", err.Error())
		}
		out.Name = &name
	}
	if in.Body.Description != nil {
		desc := strings.TrimSpace(*in.Body.Description)
		out.Description = &desc
	}
	if in.Body.Status != nil {
		status, err := normalizePromptStatus(*in.Body.Status)
		if err != nil {
			return out, err
		}
		out.Status = &status
	}
	if in.Body.TemplateText != nil {
		templateText := strings.TrimSpace(*in.Body.TemplateText)
		if templateText == "" {
			return out, domain.NewValidationError("template_text", "cannot be empty")
		}
		if _, err := application.ExtractPromptVariables(templateText); err != nil {
			return out, domain.NewValidationError("template_text", err.Error())
		}
		out.TemplateText = &templateText
	}
	if out.Name == nil && out.Description == nil && out.Status == nil && out.TemplateText == nil {
		return out, domain.NewValidationError("", "no fields to update")
	}
	return out, nil
}

func normalizePromptStatus(value string) (string, error) {
	status := strings.TrimSpace(value)
	if status == "" {
		return "active", nil
	}
	switch status {
	case "active", "disabled":
		return status, nil
	default:
		return "", domain.NewValidationError("status", "must be active or disabled")
	}
}

func createAppPrompt(ctx context.Context, d AIDeps, scope appScope, write promptCreateWrite, actor string) (appPromptDetailDTO, error) {
	if err := validateAppScope(scope); err != nil {
		return appPromptDetailDTO{}, err
	}
	repo := pgadapter.NewApplicationPromptRepo(d.Postgres)
	prompt, err := repo.CreatePrompt(ctx, pgadapter.AppPromptWrite{
		OwnerType:     scope.OwnerType,
		OwnerTenantID: scope.OwnerTenantID,
		OwnerUserID:   scope.OwnerUserID,
		Name:          write.Name,
		Description:   write.Description,
		Status:        write.Status,
		TemplateText:  write.TemplateText,
		Notes:         "",
		Actor:         actor,
	})
	if err != nil {
		return appPromptDetailDTO{}, err
	}
	return getAppPromptDetailByRepo(ctx, repo, scope, prompt.ID)
}

func updateAppPrompt(ctx context.Context, d AIDeps, scope appScope, promptID string, write promptUpdateWrite, actor string) (appPromptDetailDTO, error) {
	if err := validateAppScope(scope); err != nil {
		return appPromptDetailDTO{}, err
	}
	repo := pgadapter.NewApplicationPromptRepo(d.Postgres)
	if _, err := repo.UpdatePrompt(ctx, pgadapter.AppPromptPatch{
		OwnerType:     scope.OwnerType,
		OwnerTenantID: scope.OwnerTenantID,
		OwnerUserID:   scope.OwnerUserID,
		PromptID:      promptID,
		Name:          write.Name,
		Description:   write.Description,
		Status:        write.Status,
		TemplateText:  write.TemplateText,
		Notes:         "",
		Actor:         actor,
	}); err != nil {
		return appPromptDetailDTO{}, err
	}
	return getAppPromptDetailByRepo(ctx, repo, scope, promptID)
}

func listAppPrompts(ctx context.Context, d AIDeps, scope appScope) ([]appPromptDTO, error) {
	if err := validateAppScope(scope); err != nil {
		return nil, err
	}
	repo := pgadapter.NewApplicationPromptRepo(d.Postgres)
	items, err := repo.ListPrompts(ctx, scope.OwnerType, scope.OwnerTenantID, scope.OwnerUserID)
	if err != nil {
		return nil, err
	}
	out := make([]appPromptDTO, 0, len(items))
	for _, item := range items {
		out = append(out, applicationPromptToDTO(item))
	}
	return out, nil
}

func getAppPromptDetail(ctx context.Context, d AIDeps, scope appScope, promptID string) (appPromptDetailDTO, error) {
	if err := validateAppScope(scope); err != nil {
		return appPromptDetailDTO{}, err
	}
	repo := pgadapter.NewApplicationPromptRepo(d.Postgres)
	return getAppPromptDetailByRepo(ctx, repo, scope, promptID)
}

func deleteAppPrompt(ctx context.Context, d AIDeps, scope appScope, promptID string) error {
	repo := pgadapter.NewApplicationPromptRepo(d.Postgres)
	return repo.DeletePrompt(ctx, scope.OwnerType, scope.OwnerTenantID, scope.OwnerUserID, promptID)
}

func getAppPromptDetailByRepo(ctx context.Context, repo *pgadapter.ApplicationPromptRepo, scope appScope, promptID string) (appPromptDetailDTO, error) {
	prompt, err := repo.GetPrompt(ctx, scope.OwnerType, scope.OwnerTenantID, scope.OwnerUserID, promptID)
	if err != nil {
		if err == domain.ErrNotFound {
			return appPromptDetailDTO{}, domain.ErrNotFound
		}
		return appPromptDetailDTO{}, err
	}
	return appPromptDetailDTO{Prompt: applicationPromptToDTO(prompt)}, nil
}

func applicationPromptToDTO(prompt application.Prompt) appPromptDTO {
	return appPromptDTO{
		OwnerType:     string(prompt.OwnerScope),
		OwnerTenantID: prompt.OwnerTenantID,
		OwnerUserID:   prompt.OwnerUserID,
		ID:            prompt.ID,
		Name:          prompt.Name,
		Description:   prompt.Description,
		Status:        string(prompt.Status),
		TemplateText:  prompt.CurrentTemplateText,
		Variables:     append([]string(nil), prompt.CurrentVariables...),
		CreatedBy:     stringPtrOrNil(prompt.CreatedBy),
		UpdatedBy:     stringPtrOrNil(prompt.UpdatedBy),
		CreatedAt:     timeToMillisPtr(prompt.CreatedAt),
		UpdatedAt:     timeToMillisPtr(prompt.UpdatedAt),
	}
}

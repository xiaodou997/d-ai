package transport

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/moneyfmt"
	"xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/libs/go/httpx"
)

type workspaceOverviewInput struct {
	RequestSource string `query:"request_source" doc:"请求来源过滤"`
	LogLimit      int32  `query:"log_limit" default:"100" doc:"最近日志条数；默认 100，最大 100"`
	ItemLimit     int32  `query:"item_limit" default:"6" doc:"最近会话/任务条数；默认 6，最大 50"`
}

type workspaceItemsInput struct {
	Limit int32 `query:"limit" default:"50" doc:"返回条数；默认 50，最大 50"`
}

type workspaceSessionDetailInput struct {
	SessionID string `path:"sessionID" doc:"会话 ID"`
}

type workspaceChatSessionCreateBody struct {
	ModelCode string `json:"model_code"`
	GroupID   string `json:"group_id"`
	Title     string `json:"title,omitempty"`
}

type workspaceChatSessionCreateInput struct {
	Body workspaceChatSessionCreateBody
}

type workspaceUsageSummaryDTO struct {
	RequestCount          int64   `json:"request_count"`
	SuccessRequests       int64   `json:"success_requests"`
	FailedRequests        int64   `json:"failed_requests"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalUserChargedUSD   float64 `json:"total_user_charged_usd"`
	AvgLatencyMs          float64 `json:"avg_latency_ms"`
}

type workspaceChatSessionDTO struct {
	ID                      string  `json:"id"`
	Title                   string  `json:"title"`
	ModelCode               string  `json:"model_code"`
	GroupID                 string  `json:"group_id,omitempty"`
	GroupName               string  `json:"group_name,omitempty"`
	EffectiveUserMultiplier float64 `json:"effective_user_multiplier,omitempty"`
	BillingGroupLabel       string  `json:"billing_group_label,omitempty"`
	ProviderAPIFormat       string  `json:"provider_api_format"`
	SelectedRouteID         string  `json:"selected_route_id"`
	Status                  string  `json:"status"`
	CreatedAt               int64   `json:"created_at"`
	UpdatedAt               int64   `json:"updated_at"`
}

type workspaceChatModelDTO struct {
	GroupID                 string   `json:"group_id"`
	GroupName               string   `json:"group_name"`
	EffectiveUserMultiplier float64  `json:"effective_user_multiplier"`
	BillingGroupLabel       string   `json:"billing_group_label"`
	ModelCode               string   `json:"model_code"`
	CapabilityType          string   `json:"capability_type"`
	DefaultAPIFormat        string   `json:"default_api_format"`
	AvailableAPIFormats     []string `json:"available_api_formats"`
	SupportsStream          bool     `json:"supports_stream"`
	Status                  string   `json:"status"`
}

type workspaceChatMessageDTO struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Protocol  string         `json:"protocol,omitempty"`
	RouteID   string         `json:"route_id,omitempty"`
	Usage     map[string]any `json:"usage,omitempty"`
	Error     map[string]any `json:"error,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

type workspaceImageJobDTO struct {
	ID                   string                   `json:"id"`
	Operation            string                   `json:"operation"`
	ModelCode            string                   `json:"model_code"`
	Prompt               string                   `json:"prompt"`
	Status               string                   `json:"status"`
	StoragePolicy        string                   `json:"storage_policy"`
	RawImageRetained     bool                     `json:"raw_image_retained"`
	Size                 string                   `json:"size,omitempty"`
	Quality              string                   `json:"quality,omitempty"`
	Style                string                   `json:"style,omitempty"`
	ResponseFormat       string                   `json:"response_format,omitempty"`
	RequestedOutputCount int                      `json:"requested_output_count"`
	CallerChargeUSD      float64                  `json:"caller_charge_usd"`
	ImageCount           int                      `json:"image_count"`
	InlineCount          int                      `json:"inline_count"`
	URLCount             int                      `json:"url_count"`
	RevisedPrompts       []string                 `json:"revised_prompts,omitempty"`
	Assets               []workspaceImageAssetDTO `json:"assets,omitempty"`
	ErrorMessage         string                   `json:"error_message,omitempty"`
	CreatedAt            int64                    `json:"created_at"`
	CompletedAt          *int64                   `json:"completed_at,omitempty"`
}

type workspaceImageAssetDTO struct {
	ID                  string `json:"id,omitempty"`
	Index               int    `json:"index,omitempty"`
	PreviewURL          string `json:"preview_url,omitempty"`
	DisplayURL          string `json:"display_url"`
	OriginalURL         string `json:"original_url,omitempty"`
	OriginalContentType string `json:"content_type,omitempty"`
	OriginalSizeBytes   int64  `json:"size_bytes,omitempty"`
	PreviewContentType  string `json:"preview_content_type,omitempty"`
	PreviewSizeBytes    int64  `json:"preview_size_bytes,omitempty"`
	Width               int    `json:"width,omitempty"`
	Height              int    `json:"height,omitempty"`
	ExpiresAt           int64  `json:"expires_at,omitempty"`
}

type userWorkspaceOverviewOutput struct {
	Body struct {
		Summary      workspaceUsageSummaryDTO  `json:"summary"`
		UsageLogs    []userUsageLogDTO         `json:"usage_logs"`
		ChatSessions []workspaceChatSessionDTO `json:"chat_sessions"`
		ImageJobs    []workspaceImageJobDTO    `json:"image_jobs"`
	}
}

type tenantWorkspaceOverviewOutput struct {
	Body struct {
		Summary      workspaceUsageSummaryDTO  `json:"summary"`
		UsageLogs    []tenantUsageLogDTO       `json:"usage_logs"`
		ChatSessions []workspaceChatSessionDTO `json:"chat_sessions"`
		ImageJobs    []workspaceImageJobDTO    `json:"image_jobs"`
	}
}

type workspaceChatSessionsOutput struct {
	Body struct {
		Items []workspaceChatSessionDTO `json:"items"`
		Total int                       `json:"total"`
	}
}

type workspaceChatModelsOutput struct {
	Body struct {
		Items []workspaceChatModelDTO `json:"items"`
		Total int                     `json:"total"`
	}
}

type workspaceChatSessionDetailOutput struct {
	Body struct {
		Session  workspaceChatSessionDTO   `json:"session"`
		Messages []workspaceChatMessageDTO `json:"messages"`
	}
}

type workspaceChatSessionOutput struct {
	Body workspaceChatSessionDTO
}

type workspaceImageJobsOutput struct {
	Body struct {
		Items []workspaceImageJobDTO `json:"items"`
		Total int                    `json:"total"`
	}
}

type workspaceDeleteOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

func registerTenantSelfWorkspace(api huma.API, d WorkspaceHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-get-tenant-self-workspace-overview",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/workspace/overview",
		Summary:     "租户工作台概览",
		Description: "返回租户工作台使用概览、最近对话、最近生图和最近用量日志。",
		Tags:        []string{"workspace"},
	}, func(ctx context.Context, in *workspaceOverviewInput) (*tenantWorkspaceOverviewOutput, error) {
		return buildTenantWorkspaceOverview(ctx, d, in)
	})
	registerScopedWorkspaceResourceReads(api, d, identity.ScopeTenant, "/api/v1/tenants/me/workspace", "租户工作台")
}

func registerUserSelfWorkspace(api huma.API, d WorkspaceHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-get-user-self-workspace-overview",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/workspace/overview",
		Summary:     "用户工作台概览",
		Description: "返回用户工作台使用概览、最近对话、最近生图和最近用量日志。",
		Tags:        []string{"workspace"},
	}, func(ctx context.Context, in *workspaceOverviewInput) (*userWorkspaceOverviewOutput, error) {
		return buildUserWorkspaceOverview(ctx, d, in)
	})
	registerScopedWorkspaceResourceReads(api, d, identity.ScopeUser, "/api/v1/users/me/workspace", "用户工作台")
}

func registerScopedWorkspaceResourceReads(api huma.API, d WorkspaceHTTPDeps, scope identity.Scope, pathBase string, summaryPrefix string) {
	huma.Register(api, huma.Operation{
		OperationID: pathBaseToOperationID(pathBase + "/chat/models"),
		Method:      http.MethodGet,
		Path:        pathBase + "/chat/models",
		Summary:     summaryPrefix + "对话模型列表",
		Description: "返回当前租户或用户可见的对话模型，以及每个模型当前可用的 API 格式。",
		Tags:        []string{"workspace"},
	}, func(ctx context.Context, _ *struct{}) (*workspaceChatModelsOutput, error) {
		return buildWorkspaceChatModels(ctx, d, scope)
	})

	huma.Register(api, huma.Operation{
		OperationID: pathBaseToOperationID(pathBase + "/chat/sessions"),
		Method:      http.MethodGet,
		Path:        pathBase + "/chat/sessions",
		Summary:     summaryPrefix + "会话列表",
		Description: "返回当前租户或用户可见的工作台会话列表。",
		Tags:        []string{"workspace"},
	}, func(ctx context.Context, in *workspaceItemsInput) (*workspaceChatSessionsOutput, error) {
		return buildWorkspaceChatSessions(ctx, d, scope, in)
	})

	huma.Register(api, huma.Operation{
		OperationID:   pathBaseToOperationID(pathBase + "/chat/sessions:create"),
		Method:        http.MethodPost,
		Path:          pathBase + "/chat/sessions",
		Summary:       summaryPrefix + "创建会话",
		Description:   "创建当前租户或用户的工作台对话会话。",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"workspace"},
	}, func(ctx context.Context, in *workspaceChatSessionCreateInput) (*workspaceChatSessionOutput, error) {
		return buildWorkspaceChatSessionCreate(ctx, d, scope, in)
	})

	huma.Register(api, huma.Operation{
		OperationID: pathBaseToOperationID(pathBase + "/chat/sessions/{sessionID}"),
		Method:      http.MethodGet,
		Path:        pathBase + "/chat/sessions/{sessionID}",
		Summary:     summaryPrefix + "会话详情",
		Description: "返回当前租户或用户可见的工作台会话详情与消息列表。",
		Tags:        []string{"workspace"},
	}, func(ctx context.Context, in *workspaceSessionDetailInput) (*workspaceChatSessionDetailOutput, error) {
		return buildWorkspaceChatSessionDetail(ctx, d, scope, in)
	})

	huma.Register(api, huma.Operation{
		OperationID: pathBaseToOperationID(pathBase + "/chat/sessions/{sessionID}:delete"),
		Method:      http.MethodDelete,
		Path:        pathBase + "/chat/sessions/{sessionID}",
		Summary:     summaryPrefix + "删除会话",
		Description: "删除当前租户或用户的工作台对话会话。",
		Tags:        []string{"workspace"},
	}, func(ctx context.Context, in *workspaceSessionDetailInput) (*workspaceDeleteOutput, error) {
		return buildWorkspaceChatSessionDelete(ctx, d, scope, in)
	})

	huma.Register(api, huma.Operation{
		OperationID: pathBaseToOperationID(pathBase + "/image/jobs"),
		Method:      http.MethodGet,
		Path:        pathBase + "/image/jobs",
		Summary:     summaryPrefix + "生图任务列表",
		Description: "返回当前租户或用户可见的最近生图任务摘要。",
		Tags:        []string{"workspace"},
	}, func(ctx context.Context, in *workspaceItemsInput) (*workspaceImageJobsOutput, error) {
		return buildWorkspaceImageJobs(ctx, d, scope, in)
	})
}

func buildTenantWorkspaceOverview(ctx context.Context, d WorkspaceHTTPDeps, in *workspaceOverviewInput) (*tenantWorkspaceOverviewOutput, error) {
	if d.DashboardQueries == nil {
		return nil, httpx.ErrUnavailable.WithDetail("dashboard service is not configured")
	}
	owner, err := workspaceOwnerFromContext(ctx, identity.ScopeTenant)
	if err != nil {
		return nil, err
	}
	return buildWorkspaceOverview(ctx, d, owner, in, func(logs []domain.UsageLog, summary workspaceUsageSummaryDTO, sessions []workspace.ChatSession, jobs []workspace.ImageJob) *tenantWorkspaceOverviewOutput {
		out := &tenantWorkspaceOverviewOutput{}
		out.Body.Summary = summary
		out.Body.UsageLogs = make([]tenantUsageLogDTO, 0, len(logs))
		for _, log := range logs {
			out.Body.UsageLogs = append(out.Body.UsageLogs, tenantUsageLogToDTO(log))
		}
		out.Body.ChatSessions = make([]workspaceChatSessionDTO, 0, len(sessions))
		for _, session := range sessions {
			out.Body.ChatSessions = append(out.Body.ChatSessions, workspaceChatSessionToDTO(session))
		}
		out.Body.ImageJobs = make([]workspaceImageJobDTO, 0, len(jobs))
		for _, job := range jobs {
			out.Body.ImageJobs = append(out.Body.ImageJobs, workspaceImageJobToDTO(job))
		}
		return out
	})
}

func buildUserWorkspaceOverview(ctx context.Context, d WorkspaceHTTPDeps, in *workspaceOverviewInput) (*userWorkspaceOverviewOutput, error) {
	owner, err := workspaceOwnerFromContext(ctx, identity.ScopeUser)
	if err != nil {
		return nil, err
	}
	return buildWorkspaceOverview(ctx, d, owner, in, func(logs []domain.UsageLog, summary workspaceUsageSummaryDTO, sessions []workspace.ChatSession, jobs []workspace.ImageJob) *userWorkspaceOverviewOutput {
		out := &userWorkspaceOverviewOutput{}
		out.Body.Summary = summary
		out.Body.UsageLogs = make([]userUsageLogDTO, 0, len(logs))
		for _, log := range logs {
			out.Body.UsageLogs = append(out.Body.UsageLogs, userUsageLogFromDomain(log))
		}
		out.Body.ChatSessions = make([]workspaceChatSessionDTO, 0, len(sessions))
		for _, session := range sessions {
			out.Body.ChatSessions = append(out.Body.ChatSessions, workspaceChatSessionToDTO(session))
		}
		out.Body.ImageJobs = make([]workspaceImageJobDTO, 0, len(jobs))
		for _, job := range jobs {
			out.Body.ImageJobs = append(out.Body.ImageJobs, workspaceImageJobToDTO(job))
		}
		return out
	})
}

func buildWorkspaceOverview[T any](ctx context.Context, d WorkspaceHTTPDeps, owner workspace.Owner, in *workspaceOverviewInput, render func([]domain.UsageLog, workspaceUsageSummaryDTO, []workspace.ChatSession, []workspace.ImageJob) *T) (*T, error) {
	if d.WorkspaceOverview == nil || d.UsageQueries == nil {
		return nil, httpx.ErrUnavailable.WithDetail("workspace dependencies are not configured")
	}
	logLimit, err := workspaceLogLimitFromInput(in.LogLimit)
	if err != nil {
		return nil, err
	}
	itemLimit, err := workspaceItemLimitFromInput(in.ItemLimit)
	if err != nil {
		return nil, err
	}
	overview, err := d.WorkspaceOverview.Overview(ctx, owner, itemLimit)
	if err != nil {
		return nil, mapServiceError(err)
	}
	logs, summary, err := workspaceLogsAndSummary(ctx, d, owner, in.RequestSource, logLimit)
	if err != nil {
		return nil, err
	}
	out := render(logs, summary, overview.RecentChatSessions, overview.RecentImageJobs)
	return out, nil
}

func buildWorkspaceChatModels(ctx context.Context, d WorkspaceHTTPDeps, scope identity.Scope) (*workspaceChatModelsOutput, error) {
	if d.WorkspaceModels == nil {
		return nil, httpx.ErrUnavailable.WithDetail("workspace service is not configured")
	}
	owner, err := workspaceOwnerFromContext(ctx, scope)
	if err != nil {
		return nil, err
	}
	items, err := d.WorkspaceModels.ListChatModels(ctx, owner)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &workspaceChatModelsOutput{}
	out.Body.Items = make([]workspaceChatModelDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, workspaceChatModelToDTO(item))
	}
	out.Body.Total = len(out.Body.Items)
	return out, nil
}

func buildWorkspaceChatSessions(ctx context.Context, d WorkspaceHTTPDeps, scope identity.Scope, in *workspaceItemsInput) (*workspaceChatSessionsOutput, error) {
	if d.WorkspaceSessions == nil {
		return nil, httpx.ErrUnavailable.WithDetail("workspace service is not configured")
	}
	owner, err := workspaceOwnerFromContext(ctx, scope)
	if err != nil {
		return nil, err
	}
	limit, err := workspaceItemLimitFromInput(in.Limit)
	if err != nil {
		return nil, err
	}
	items, err := d.WorkspaceSessions.ListChatSessions(ctx, owner, limit)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &workspaceChatSessionsOutput{}
	out.Body.Items = make([]workspaceChatSessionDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, workspaceChatSessionToDTO(item))
	}
	out.Body.Total = len(out.Body.Items)
	return out, nil
}

func buildWorkspaceChatSessionDetail(ctx context.Context, d WorkspaceHTTPDeps, scope identity.Scope, in *workspaceSessionDetailInput) (*workspaceChatSessionDetailOutput, error) {
	if d.WorkspaceSessions == nil {
		return nil, httpx.ErrUnavailable.WithDetail("workspace service is not configured")
	}
	owner, err := workspaceOwnerFromContext(ctx, scope)
	if err != nil {
		return nil, err
	}
	sessionID := in.SessionID
	session, err := d.WorkspaceSessions.GetChatSession(ctx, owner, sessionID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	messages, err := d.WorkspaceSessions.ListChatMessages(ctx, owner, sessionID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &workspaceChatSessionDetailOutput{}
	out.Body.Session = workspaceChatSessionToDTO(session)
	out.Body.Messages = make([]workspaceChatMessageDTO, 0, len(messages))
	for _, message := range messages {
		out.Body.Messages = append(out.Body.Messages, workspaceChatMessageToDTO(message))
	}
	return out, nil
}

func buildWorkspaceChatSessionCreate(ctx context.Context, d WorkspaceHTTPDeps, scope identity.Scope, in *workspaceChatSessionCreateInput) (*workspaceChatSessionOutput, error) {
	if d.WorkspaceManager == nil {
		return nil, httpx.ErrUnavailable.WithDetail("workspace service is not configured")
	}
	owner, err := workspaceOwnerFromContext(ctx, scope)
	if err != nil {
		return nil, err
	}
	session, err := d.WorkspaceManager.CreateChatSession(ctx, owner, workspace.CreateChatSessionInput{
		ModelCode: in.Body.ModelCode,
		GroupID:   in.Body.GroupID,
		Title:     in.Body.Title,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &workspaceChatSessionOutput{}
	out.Body = workspaceChatSessionToDTO(session)
	return out, nil
}

func buildWorkspaceChatSessionDelete(ctx context.Context, d WorkspaceHTTPDeps, scope identity.Scope, in *workspaceSessionDetailInput) (*workspaceDeleteOutput, error) {
	if d.WorkspaceManager == nil {
		return nil, httpx.ErrUnavailable.WithDetail("workspace service is not configured")
	}
	owner, err := workspaceOwnerFromContext(ctx, scope)
	if err != nil {
		return nil, err
	}
	if err := d.WorkspaceManager.DeleteChatSession(ctx, owner, in.SessionID); err != nil {
		return nil, mapServiceError(err)
	}
	out := &workspaceDeleteOutput{}
	out.Body.Deleted = true
	return out, nil
}

func buildWorkspaceImageJobs(ctx context.Context, d WorkspaceHTTPDeps, scope identity.Scope, in *workspaceItemsInput) (*workspaceImageJobsOutput, error) {
	if d.WorkspaceImages == nil {
		return nil, httpx.ErrUnavailable.WithDetail("workspace service is not configured")
	}
	owner, err := workspaceOwnerFromContext(ctx, scope)
	if err != nil {
		return nil, err
	}
	limit, err := workspaceItemLimitFromInput(in.Limit)
	if err != nil {
		return nil, err
	}
	items, err := d.WorkspaceImages.ListImageJobs(ctx, owner, limit)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &workspaceImageJobsOutput{}
	out.Body.Items = make([]workspaceImageJobDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, workspaceImageJobToDTO(item))
	}
	out.Body.Total = len(out.Body.Items)
	return out, nil
}

func workspaceOwnerFromContext(ctx context.Context, scope identity.Scope) (workspace.Owner, error) {
	tenantID := tenantIDFromContext(ctx)
	if tenantID == "" {
		return workspace.Owner{}, httpx.ErrBadRequest.WithDetail("tenant id is required")
	}
	owner := workspace.Owner{Scope: scope, TenantID: tenantID}
	if scope == identity.ScopeUser {
		owner.UserID = userIDFromContext(ctx)
		if owner.UserID == "" {
			return workspace.Owner{}, httpx.ErrBadRequest.WithDetail("user id is required")
		}
	}
	return owner, nil
}

func workspaceLogsAndSummary(ctx context.Context, d WorkspaceHTTPDeps, owner workspace.Owner, requestSource string, limit int32) ([]domain.UsageLog, workspaceUsageSummaryDTO, error) {
	filter := domain.UsageFilter{
		TenantID:      owner.TenantID,
		UserID:        owner.UserID,
		RequestSource: requestSource,
	}
	page, err := d.UsageQueries.ListLogs(ctx, filter, limit, 0)
	if err != nil {
		return nil, workspaceUsageSummaryDTO{}, mapServiceError(err)
	}
	if owner.Scope == identity.ScopeUser {
		summary, err := d.UsageQueries.UserSummary(ctx, owner.TenantID, owner.UserID, requestSource)
		if err != nil {
			return nil, workspaceUsageSummaryDTO{}, mapServiceError(err)
		}
		return page.Records, workspaceSummaryFromUser(summary), nil
	}
	summaryRows, err := d.DashboardQueries.Summary(ctx, domain.DashboardFilter{
		TenantID: owner.TenantID,
	})
	if err != nil {
		return nil, workspaceUsageSummaryDTO{}, mapServiceError(err)
	}
	return page.Records, workspaceSummaryFromDashboard(summaryRows), nil
}

func workspaceSummaryFromUser(summary domain.UserUsageSummary) workspaceUsageSummaryDTO {
	return workspaceUsageSummaryDTO{
		RequestCount:          summary.RequestCount,
		SuccessRequests:       summary.SuccessRequests,
		FailedRequests:        summary.FailedRequests,
		TotalTokens:           summary.TotalTokens,
		TotalPromptTokens:     summary.TotalPromptTokens,
		TotalCompletionTokens: summary.TotalCompletionTokens,
		TotalUserChargedUSD:   moneyfmt.MicroToUSD(summary.TotalUserChargedMicro),
		AvgLatencyMs:          summary.AvgLatencyMs,
	}
}

func workspaceSummaryFromDashboard(summary domain.DashboardSummary) workspaceUsageSummaryDTO {
	return workspaceUsageSummaryDTO{
		RequestCount:          summary.TotalRequests,
		SuccessRequests:       summary.SuccessfulRequests,
		FailedRequests:        summary.FailedRequests,
		TotalTokens:           summary.TotalTokens,
		TotalPromptTokens:     summary.TotalPromptTokens,
		TotalCompletionTokens: summary.TotalCompletionTokens,
		TotalUserChargedUSD:   moneyfmt.MicroToUSD(summary.TotalUserChargedMicro),
		AvgLatencyMs:          summary.AvgLatencyMs,
	}
}

func workspaceLogLimitFromInput(limit int32) (int32, error) {
	if limit <= 0 {
		return 0, httpx.ErrBadRequest.WithDetail("invalid log_limit")
	}
	if limit > 100 {
		limit = 100
	}
	return limit, nil
}

func workspaceItemLimitFromInput(limit int32) (int, error) {
	if limit <= 0 {
		return 0, httpx.ErrBadRequest.WithDetail("invalid item_limit")
	}
	if limit > 50 {
		limit = 50
	}
	return int(limit), nil
}

func workspaceChatSessionToDTO(session workspace.ChatSession) workspaceChatSessionDTO {
	return workspaceChatSessionDTO{
		ID:                      session.ID,
		Title:                   session.Title,
		ModelCode:               session.ModelCode,
		GroupID:                 session.GroupID,
		GroupName:               session.GroupName,
		EffectiveUserMultiplier: session.EffectiveUserMultiplier,
		BillingGroupLabel:       session.BillingGroupLabel,
		ProviderAPIFormat:       session.SelectedProtocol,
		SelectedRouteID:         session.SelectedRouteID,
		Status:                  session.Status,
		CreatedAt:               session.CreatedAt.UnixMilli(),
		UpdatedAt:               session.UpdatedAt.UnixMilli(),
	}
}

func workspaceChatModelToDTO(model workspace.ChatModel) workspaceChatModelDTO {
	return workspaceChatModelDTO{
		GroupID:                 model.GroupID,
		GroupName:               model.GroupName,
		EffectiveUserMultiplier: model.EffectiveUserMultiplier,
		BillingGroupLabel:       model.BillingGroupLabel,
		ModelCode:               model.ModelCode,
		CapabilityType:          model.CapabilityType,
		DefaultAPIFormat:        model.DefaultProtocol,
		AvailableAPIFormats:     append([]string{}, model.AvailableProtocols...),
		SupportsStream:          model.SupportsStream,
		Status:                  model.Status,
	}
}

func workspaceChatMessageToDTO(message workspace.ChatMessage) workspaceChatMessageDTO {
	return workspaceChatMessageDTO{
		ID:        message.ID,
		Role:      message.Role,
		Content:   message.Content,
		Protocol:  message.Protocol,
		RouteID:   message.RouteID,
		Usage:     message.Usage,
		Error:     message.Error,
		CreatedAt: message.CreatedAt.UnixMilli(),
	}
}

func workspaceImageJobToDTO(job workspace.ImageJob) workspaceImageJobDTO {
	dto := workspaceImageJobDTO{
		ID:                   job.ID,
		Operation:            job.Operation,
		ModelCode:            job.ModelCode,
		Prompt:               job.Prompt,
		Status:               job.Status,
		StoragePolicy:        job.StoragePolicy,
		RawImageRetained:     job.RawImageRetained,
		Size:                 job.Size,
		Quality:              job.Quality,
		Style:                job.Style,
		ResponseFormat:       job.ResponseFormat,
		RequestedOutputCount: job.RequestedOutputCount,
		CallerChargeUSD:      moneyfmt.MicroToUSD(job.CallerChargeMicro),
		ImageCount:           job.ImageCount,
		InlineCount:          job.InlineCount,
		URLCount:             job.URLCount,
		RevisedPrompts:       job.RevisedPrompts,
		Assets:               workspaceImageAssetsToDTO(job.Assets),
		ErrorMessage:         job.ErrorMessage,
		CreatedAt:            job.CreatedAt.UnixMilli(),
	}
	if job.CompletedAt != nil {
		completedAt := job.CompletedAt.UnixMilli()
		dto.CompletedAt = &completedAt
	}
	return dto
}

func workspaceImageAssetsToDTO(assets []workspace.ImageAsset) []workspaceImageAssetDTO {
	if len(assets) == 0 {
		return nil
	}
	out := make([]workspaceImageAssetDTO, 0, len(assets))
	for _, asset := range assets {
		if asset.PreviewURL == "" && asset.DisplayURL == "" && asset.OriginalURL == "" {
			continue
		}
		out = append(out, workspaceImageAssetDTO{
			ID:                  asset.ID,
			Index:               asset.Index,
			PreviewURL:          asset.PreviewURL,
			DisplayURL:          asset.DisplayURL,
			OriginalURL:         asset.OriginalURL,
			OriginalContentType: asset.OriginalContentType,
			OriginalSizeBytes:   asset.OriginalSizeBytes,
			PreviewContentType:  asset.PreviewContentType,
			PreviewSizeBytes:    asset.PreviewSizeBytes,
			Width:               asset.Width,
			Height:              asset.Height,
			ExpiresAt:           asset.ExpiresAt,
		})
	}
	return out
}

func pathBaseToOperationID(path string) string {
	path = strings.ReplaceAll(path, "/", "-")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	path = strings.ReplaceAll(path, "--", "-")
	path = strings.Trim(path, "-")
	return "ai-" + strings.ToLower(path)
}

func userUsageLogFromDomain(log domain.UsageLog) userUsageLogDTO {
	return userUsageLogDTO{
		ID:                              log.ID,
		RequestID:                       log.RequestID,
		TraceID:                         stringPtrOrNil(log.TraceID),
		TenantID:                        log.TenantID,
		UserID:                          stringPtrOrNil(log.UserID),
		RequestSource:                   log.RequestSource,
		GroupID:                         log.GroupID,
		GroupNameSnapshot:               log.GroupNameSnapshot,
		EffectiveUserMultiplierSnapshot: log.EffectiveUserMultiplierSnapshot,
		BillingGroupLabelSnapshot:       log.BillingGroupLabelSnapshot,
		ModelCode:                       log.ModelCode,
		PromptTokens:                    log.PromptTokens,
		CompletionTokens:                log.CompletionTokens,
		TotalTokens:                     log.TotalTokens,
		BillableUnitType:                log.BillableUnitType,
		BillableUnits:                   log.BillableUnits,
		UserChargedUSD:                  moneyfmt.MicroToUSD(log.UserChargedMicro),
		ServiceTier:                     log.ServiceTier,
		RequestStatus:                   log.RequestStatus,
		HTTPStatus:                      log.HTTPStatus,
		LatencyMs:                       log.LatencyMs,
		ErrorCode:                       stringPtrOrNil(log.ErrorCode),
		ErrorMessage:                    stringPtrOrNil(log.ErrorMessage),
		CreatedAt:                       timeToMillisPtr(log.CreatedAt),
	}
}

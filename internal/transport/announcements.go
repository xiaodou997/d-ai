package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/announcement"
	"xiaodou/dai/libs/go/httpx"
)

type announcementHandlers struct {
	service *announcement.Service
}

type announcementPathInput struct {
	ID string `path:"id"`
}

type listInboxInput struct {
	Page        int    `query:"page" default:"1"`
	Size        int    `query:"size" default:"20"`
	UnreadOnly  bool   `query:"unreadOnly" required:"false"`
	DisplayMode string `query:"displayMode" required:"false" enum:"inbox,popup"`
}

type audienceSelectionInput struct {
	Kind      string   `json:"kind" enum:"admin,tenant_user,end_user"`
	Scope     string   `json:"scope" enum:"all,tenant"`
	TenantIDs []string `json:"tenantIds,omitempty" required:"false"`
}

type announcementDraftBody struct {
	Title           string                   `json:"title" maxLength:"200"`
	ContentMarkdown string                   `json:"contentMarkdown" maxLength:"50000"`
	Category        string                   `json:"category" enum:"general,maintenance,upgrade,pricing,security"`
	Severity        string                   `json:"severity" enum:"info,important,critical"`
	DisplayMode     string                   `json:"displayMode" enum:"inbox,popup"`
	StartsAt        *int64                   `json:"startsAt,omitempty" required:"false"`
	EndsAt          *int64                   `json:"endsAt,omitempty" required:"false"`
	Audiences       []audienceSelectionInput `json:"audiences,omitempty" required:"false"`
}

type createAnnouncementInput struct {
	Body announcementDraftBody
}

type updateAnnouncementInput struct {
	ID   string `path:"id"`
	Body announcementDraftBody
}

type listManagedAnnouncementsInput struct {
	Status string `query:"status" required:"false" enum:"draft,published,archived"`
	Search string `query:"search" required:"false" maxLength:"200"`
	Page   int    `query:"page" default:"1"`
	Size   int    `query:"size" default:"20"`
}

type listAnnouncementRecipientsInput struct {
	ID     string `path:"id"`
	Search string `query:"search" required:"false" maxLength:"200"`
	Page   int    `query:"page" default:"1"`
	Size   int    `query:"size" default:"20"`
}

type audienceRuleOutput struct {
	Kind     string `json:"kind"`
	Scope    string `json:"scope"`
	TenantID string `json:"tenantId,omitempty"`
}

type announcementOutputItem struct {
	AnnouncementID        string               `json:"announcementId"`
	PublisherType         string               `json:"publisherType"`
	PublisherTenantID     string               `json:"publisherTenantId,omitempty"`
	Title                 string               `json:"title"`
	ContentMarkdown       string               `json:"contentMarkdown"`
	Category              string               `json:"category"`
	Severity              string               `json:"severity"`
	DisplayMode           string               `json:"displayMode"`
	Status                string               `json:"status"`
	StartsAt              *int64               `json:"startsAt,omitempty"`
	EndsAt                *int64               `json:"endsAt,omitempty"`
	PublishedAt           *int64               `json:"publishedAt,omitempty"`
	ArchivedAt            *int64               `json:"archivedAt,omitempty"`
	AudienceSizeAtPublish *int64               `json:"audienceSizeAtPublish,omitempty"`
	CreatedBy             string               `json:"createdBy"`
	UpdatedBy             string               `json:"updatedBy"`
	CreatedAt             int64                `json:"createdAt"`
	UpdatedAt             int64                `json:"updatedAt"`
	Audiences             []audienceRuleOutput `json:"audiences,omitempty"`
	ReadAt                *int64               `json:"readAt,omitempty"`
}

type announcementOutput struct{ Body announcementOutputItem }

type inboxOutput struct {
	Body struct {
		Items       []announcementOutputItem `json:"items"`
		Total       int64                    `json:"total"`
		UnreadCount int64                    `json:"unreadCount"`
		Page        int                      `json:"page"`
		Size        int                      `json:"size"`
	}
}

type managedAnnouncementsOutput struct {
	Body httpx.Page[announcementOutputItem]
}

type announcementStatsOutput struct {
	Body struct {
		AudienceSizeAtPublish int64 `json:"audienceSizeAtPublish"`
		CurrentAudienceSize   int64 `json:"currentAudienceSize"`
		ReadCount             int64 `json:"readCount"`
	}
}

type announcementRecipientOutput struct {
	UserType int    `json:"userType"`
	UserID   string `json:"userId"`
	TenantID string `json:"tenantId,omitempty"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	ReadAt   *int64 `json:"readAt,omitempty"`
}

type announcementRecipientsOutput struct {
	Body httpx.Page[announcementRecipientOutput]
}

func registerAnnouncements(api huma.API, d Deps) {
	h := &announcementHandlers{service: d.Announcements}
	ua := userAuth(api, d.JWT, d.Blacklist)
	allUsers := huma.Middlewares{ua, requireUserType(api, 1, 2, 3, 4)}
	admins := huma.Middlewares{ua, requireUserType(api, 1, 2)}
	tenants := huma.Middlewares{ua, requireUserType(api, 3)}

	huma.Register(api, huma.Operation{OperationID: "list-my-announcements", Method: http.MethodGet, Path: "/api/v1/announcements", Summary: "当前用户公告", Tags: []string{"announcements"}, Middlewares: allUsers}, h.listInbox)
	huma.Register(api, huma.Operation{OperationID: "get-my-announcement", Method: http.MethodGet, Path: "/api/v1/announcements/{id}", Summary: "当前用户公告详情", Tags: []string{"announcements"}, Middlewares: allUsers}, h.getVisible)
	huma.Register(api, huma.Operation{OperationID: "mark-announcement-read", Method: http.MethodPost, Path: "/api/v1/announcements/{id}/read", Summary: "标记公告已读", Tags: []string{"announcements"}, Middlewares: allUsers}, h.markRead)

	registerAnnouncementManagement(api, h, "/api/v1/admin/announcements", "admin", admins)
	registerAnnouncementManagement(api, h, "/api/v1/tenants/me/announcements", "tenant", tenants)
}

func registerAnnouncementManagement(api huma.API, h *announcementHandlers, base, prefix string, middleware huma.Middlewares) {
	tag := prefix + "-announcements"
	huma.Register(api, huma.Operation{OperationID: prefix + "-list-announcements", Method: http.MethodGet, Path: base, Summary: "公告管理列表", Tags: []string{tag}, Middlewares: middleware}, h.listManaged)
	huma.Register(api, huma.Operation{OperationID: prefix + "-create-announcement", Method: http.MethodPost, Path: base, Summary: "创建公告草稿", Tags: []string{tag}, Middlewares: middleware, DefaultStatus: http.StatusCreated}, h.createDraft)
	huma.Register(api, huma.Operation{OperationID: prefix + "-get-announcement", Method: http.MethodGet, Path: base + "/{id}", Summary: "公告管理详情", Tags: []string{tag}, Middlewares: middleware}, h.getManaged)
	huma.Register(api, huma.Operation{OperationID: prefix + "-update-announcement", Method: http.MethodPut, Path: base + "/{id}", Summary: "更新公告草稿", Tags: []string{tag}, Middlewares: middleware}, h.updateDraft)
	huma.Register(api, huma.Operation{OperationID: prefix + "-delete-announcement", Method: http.MethodDelete, Path: base + "/{id}", Summary: "删除公告草稿", Tags: []string{tag}, Middlewares: middleware}, h.deleteDraft)
	huma.Register(api, huma.Operation{OperationID: prefix + "-publish-announcement", Method: http.MethodPost, Path: base + "/{id}/publish", Summary: "发布公告", Tags: []string{tag}, Middlewares: middleware}, h.publish)
	huma.Register(api, huma.Operation{OperationID: prefix + "-archive-announcement", Method: http.MethodPost, Path: base + "/{id}/archive", Summary: "下线公告", Tags: []string{tag}, Middlewares: middleware}, h.archive)
	huma.Register(api, huma.Operation{OperationID: prefix + "-get-announcement-stats", Method: http.MethodGet, Path: base + "/{id}/stats", Summary: "公告已读统计", Tags: []string{tag}, Middlewares: middleware}, h.stats)
	huma.Register(api, huma.Operation{OperationID: prefix + "-list-announcement-recipients", Method: http.MethodGet, Path: base + "/{id}/recipients", Summary: "公告收件人明细", Tags: []string{tag}, Middlewares: middleware}, h.recipients)
}

func (h *announcementHandlers) listInbox(ctx context.Context, in *listInboxInput) (*inboxOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	page, err := h.service.ListInbox(ctx, announcement.Principal{UserType: claims.UserType, UserID: claims.UserID, TenantID: claims.TenantID}, announcement.InboxQuery{
		Page: in.Page, Size: in.Size, UnreadOnly: in.UnreadOnly, DisplayMode: announcement.DisplayMode(in.DisplayMode),
	})
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	out := &inboxOutput{}
	out.Body.Items = make([]announcementOutputItem, 0, len(page.Items))
	for _, item := range page.Items {
		out.Body.Items = append(out.Body.Items, announcementResponse(item.Announcement, item.ReadAt))
	}
	out.Body.Total, out.Body.UnreadCount, out.Body.Page, out.Body.Size = page.Total, page.UnreadCount, page.Page, page.Size
	return out, nil
}

func (h *announcementHandlers) getVisible(ctx context.Context, in *announcementPathInput) (*announcementOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	item, err := h.service.GetVisible(ctx, announcement.Principal{UserType: claims.UserType, UserID: claims.UserID, TenantID: claims.TenantID}, in.ID)
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	return &announcementOutput{Body: announcementResponse(item.Announcement, item.ReadAt)}, nil
}

func (h *announcementHandlers) markRead(ctx context.Context, in *announcementPathInput) (*messageOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	if err := h.service.MarkRead(ctx, announcement.Principal{UserType: claims.UserType, UserID: claims.UserID, TenantID: claims.TenantID}, in.ID); err != nil {
		return nil, announcementHTTPError(err)
	}
	out := &messageOutput{}
	out.Body.Message = "已标记为已读"
	return out, nil
}

func (h *announcementHandlers) listManaged(ctx context.Context, in *listManagedAnnouncementsInput) (*managedAnnouncementsOutput, error) {
	actor, err := announcementActor(ctx)
	if err != nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	page, err := h.service.ListManaged(ctx, actor, announcement.ManageQuery{Status: announcement.Status(in.Status), Search: in.Search, Page: in.Page, Size: in.Size})
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	items := make([]announcementOutputItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, announcementResponse(item, nil))
	}
	return &managedAnnouncementsOutput{Body: httpx.NewPage(items, page.Total, page.Page, page.Size)}, nil
}

func (h *announcementHandlers) createDraft(ctx context.Context, in *createAnnouncementInput) (*announcementOutput, error) {
	actor, err := announcementActor(ctx)
	if err != nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	item, err := h.service.CreateDraft(ctx, actor, announcementDraftInput(in.Body))
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	return &announcementOutput{Body: announcementResponse(item, nil)}, nil
}

func (h *announcementHandlers) getManaged(ctx context.Context, in *announcementPathInput) (*announcementOutput, error) {
	actor, err := announcementActor(ctx)
	if err != nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	item, err := h.service.GetManaged(ctx, actor, in.ID)
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	return &announcementOutput{Body: announcementResponse(item, nil)}, nil
}

func (h *announcementHandlers) updateDraft(ctx context.Context, in *updateAnnouncementInput) (*announcementOutput, error) {
	actor, err := announcementActor(ctx)
	if err != nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	item, err := h.service.UpdateDraft(ctx, actor, in.ID, announcementDraftInput(in.Body))
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	return &announcementOutput{Body: announcementResponse(item, nil)}, nil
}

func (h *announcementHandlers) deleteDraft(ctx context.Context, in *announcementPathInput) (*messageOutput, error) {
	actor, err := announcementActor(ctx)
	if err != nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	if err := h.service.DeleteDraft(ctx, actor, in.ID); err != nil {
		return nil, announcementHTTPError(err)
	}
	out := &messageOutput{}
	out.Body.Message = "草稿已删除"
	return out, nil
}

func (h *announcementHandlers) publish(ctx context.Context, in *announcementPathInput) (*announcementOutput, error) {
	return h.transition(ctx, in.ID, false)
}

func (h *announcementHandlers) archive(ctx context.Context, in *announcementPathInput) (*announcementOutput, error) {
	return h.transition(ctx, in.ID, true)
}

func (h *announcementHandlers) transition(ctx context.Context, id string, archive bool) (*announcementOutput, error) {
	actor, err := announcementActor(ctx)
	if err != nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	var item announcement.Announcement
	if archive {
		item, err = h.service.Archive(ctx, actor, id)
	} else {
		item, err = h.service.Publish(ctx, actor, id)
	}
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	return &announcementOutput{Body: announcementResponse(item, nil)}, nil
}

func (h *announcementHandlers) stats(ctx context.Context, in *announcementPathInput) (*announcementStatsOutput, error) {
	actor, err := announcementActor(ctx)
	if err != nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	stats, err := h.service.GetStats(ctx, actor, in.ID)
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	out := &announcementStatsOutput{}
	out.Body.AudienceSizeAtPublish, out.Body.CurrentAudienceSize, out.Body.ReadCount = stats.AudienceSizeAtPublish, stats.CurrentAudienceSize, stats.ReadCount
	return out, nil
}

func (h *announcementHandlers) recipients(ctx context.Context, in *listAnnouncementRecipientsInput) (*announcementRecipientsOutput, error) {
	actor, err := announcementActor(ctx)
	if err != nil || h.service == nil {
		return nil, httpx.ErrUnauthorized
	}
	page, err := h.service.ListRecipients(ctx, actor, in.ID, announcement.RecipientQuery{Search: in.Search, Page: in.Page, Size: in.Size})
	if err != nil {
		return nil, announcementHTTPError(err)
	}
	items := make([]announcementRecipientOutput, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, announcementRecipientOutput{UserType: item.UserType, UserID: item.UserID, TenantID: item.TenantID, Username: item.Username, Email: item.Email, ReadAt: millisPtr(item.ReadAt)})
	}
	return &announcementRecipientsOutput{Body: httpx.NewPage(items, page.Total, page.Page, page.Size)}, nil
}

func announcementActor(ctx context.Context) (announcement.Actor, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return announcement.Actor{}, httpx.ErrUnauthorized
	}
	return announcement.Actor{UserType: claims.UserType, UserID: claims.UserID, TenantID: claims.TenantID}, nil
}

func announcementDraftInput(body announcementDraftBody) announcement.DraftInput {
	rules := make([]announcement.AudienceRule, 0)
	for _, selection := range body.Audiences {
		if selection.Scope == string(announcement.AudienceScopeAll) {
			rules = append(rules, announcement.AudienceRule{Kind: announcement.AudienceKind(selection.Kind), ScopeType: announcement.AudienceScopeAll})
			continue
		}
		for _, tenantID := range selection.TenantIDs {
			rules = append(rules, announcement.AudienceRule{Kind: announcement.AudienceKind(selection.Kind), ScopeType: announcement.AudienceScopeTenant, TenantID: tenantID})
		}
	}
	return announcement.DraftInput{
		Title: body.Title, ContentMarkdown: body.ContentMarkdown, Category: announcement.Category(body.Category), Severity: announcement.Severity(body.Severity),
		DisplayMode: announcement.DisplayMode(body.DisplayMode), StartsAt: timeFromMillis(body.StartsAt), EndsAt: timeFromMillis(body.EndsAt), Audiences: rules,
	}
}

func announcementResponse(item announcement.Announcement, readAt *time.Time) announcementOutputItem {
	out := announcementOutputItem{
		AnnouncementID: item.ID, PublisherType: string(item.PublisherType), PublisherTenantID: item.PublisherTenantID,
		Title: item.Title, ContentMarkdown: item.ContentMarkdown, Category: string(item.Category), Severity: string(item.Severity),
		DisplayMode: string(item.DisplayMode), Status: string(item.Status), StartsAt: millisPtr(item.StartsAt), EndsAt: millisPtr(item.EndsAt),
		PublishedAt: millisPtr(item.PublishedAt), ArchivedAt: millisPtr(item.ArchivedAt), AudienceSizeAtPublish: item.AudienceSizeAtPublish,
		CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy, CreatedAt: item.CreatedAt.UnixMilli(), UpdatedAt: item.UpdatedAt.UnixMilli(), ReadAt: millisPtr(readAt),
		Audiences: make([]audienceRuleOutput, 0, len(item.Audiences)),
	}
	for _, rule := range item.Audiences {
		out.Audiences = append(out.Audiences, audienceRuleOutput{Kind: string(rule.Kind), Scope: string(rule.ScopeType), TenantID: rule.TenantID})
	}
	return out
}

func timeFromMillis(value *int64) *time.Time {
	if value == nil {
		return nil
	}
	t := time.UnixMilli(*value).UTC()
	return &t
}

func millisPtr(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	v := value.UTC().UnixMilli()
	return &v
}

func announcementHTTPError(err error) error {
	switch {
	case errors.Is(err, announcement.ErrNotFound):
		return httpx.ErrNotFound.WithDetail("公告不存在")
	case errors.Is(err, announcement.ErrForbidden):
		return httpx.ErrForbidden.WithDetail("无权操作该公告")
	case errors.Is(err, announcement.ErrInvalidTransition):
		return httpx.ErrConflict.WithDetail("公告当前状态不允许此操作")
	case errors.Is(err, announcement.ErrInvalidTitle), errors.Is(err, announcement.ErrInvalidContent),
		errors.Is(err, announcement.ErrInvalidAudience), errors.Is(err, announcement.ErrInvalidSchedule),
		errors.Is(err, announcement.ErrInvalidMetadata):
		return httpx.ErrBadRequest.WithDetail(strings.TrimPrefix(err.Error(), "announcement "))
	default:
		return httpx.ErrInternal.WithCause(err)
	}
}
